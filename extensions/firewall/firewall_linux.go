//go:build linux

package firewall

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/bits"
	"net"
	"strings"

	"github.com/TsekNet/converge/extensions"
	"github.com/TsekNet/converge/internal/version"
	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"
)

const (
	tableName = version.Name
	chainIn   = "input"
	chainOut  = "output"
)

// Check determines whether a matching nftables rule exists with the desired
// attributes. Matching by name (UserData) alone is insufficient: a rule that
// was flipped block->allow or re-pointed to another port/source still carries
// the same name, so Check decodes the rule's expressions and compares
// port/protocol/verdict/source/dest against the desired spec. Any mismatch is
// reported as drift so Apply (delete-by-name then re-add) corrects it.
func (f *Firewall) Check(_ context.Context) (*extensions.State, error) {
	if err := f.validErr(); err != nil {
		return nil, err
	}
	c, err := nftables.New()
	if err != nil {
		return nil, fmt.Errorf("check firewall rule %q: %w", f.Name, err)
	}
	matched, err := f.findMatchingRules(c)
	if err != nil {
		return nil, fmt.Errorf("check firewall rule %q: %w", f.Name, err)
	}

	if f.State == "absent" {
		// Want absent: in sync only if no name-matching rule exists.
		return checkResult(f.Name, len(matched) > 0, false)
	}

	// Want present: a name-matching rule must exist AND its decoded
	// attributes must match the desired spec.
	for _, rule := range matched {
		if f.attrsMatch(decodeRule(rule.Exprs)) {
			return &extensions.State{InSync: true}, nil
		}
	}
	action := "add"
	if len(matched) > 0 {
		action = "modify"
	}
	return &extensions.State{
		InSync: false,
		Changes: []extensions.Change{{
			Property: "rule",
			From:     boolToState(len(matched) > 0),
			To:       "present",
			Action:   action,
		}},
	}, nil
}

// Apply creates or removes the nftables rule.
func (f *Firewall) Apply(_ context.Context) (*extensions.Result, error) {
	if err := f.validErr(); err != nil {
		return nil, err
	}
	if f.State == "absent" {
		return f.removeRule()
	}
	return f.addRule()
}

func (f *Firewall) addRule() (*extensions.Result, error) {
	c, err := nftables.New()
	if err != nil {
		return nil, fmt.Errorf("nftables conn: %w", err)
	}

	// Delete existing rule first to prevent duplicates on repeated Apply.
	// Ignore "table/chain not found" errors (first run).
	if err := f.deleteExistingRules(c); err != nil {
		return nil, fmt.Errorf("cleanup existing rules: %w", err)
	}

	table := ensureTable(c)
	chain := f.ensureChain(c, table)
	f.buildRule(c, table, chain)

	if err := c.Flush(); err != nil {
		return nil, fmt.Errorf("nftables flush: %w", err)
	}

	return resultChanged("added")
}

func (f *Firewall) removeRule() (*extensions.Result, error) {
	c, err := nftables.New()
	if err != nil {
		return nil, fmt.Errorf("nftables conn: %w", err)
	}

	if err := f.deleteExistingRules(c); err != nil {
		return nil, err
	}

	if err := c.Flush(); err != nil {
		return nil, fmt.Errorf("nftables flush: %w", err)
	}

	return resultChanged("removed")
}

func (f *Firewall) findMatchingRules(c *nftables.Conn) ([]*nftables.Rule, error) {
	table := &nftables.Table{Name: tableName, Family: nftables.TableFamilyIPv4}
	chain := &nftables.Chain{Name: f.chainName(), Table: table}

	rules, err := c.GetRules(table, chain)
	if err != nil {
		// Table/chain doesn't exist yet: no matching rules.
		return nil, nil
	}

	var matched []*nftables.Rule
	for _, rule := range rules {
		if hasUserData(rule, f.Name) {
			matched = append(matched, rule)
		}
	}
	return matched, nil
}

func (f *Firewall) deleteExistingRules(c *nftables.Conn) error {
	matched, err := f.findMatchingRules(c)
	if err != nil {
		return err
	}
	for _, rule := range matched {
		if err := c.DelRule(rule); err != nil {
			return fmt.Errorf("delete rule: %w", err)
		}
	}
	return nil
}

func ensureTable(c *nftables.Conn) *nftables.Table {
	return c.AddTable(&nftables.Table{
		Name:   tableName,
		Family: nftables.TableFamilyIPv4,
	})
}

func (f *Firewall) ensureChain(c *nftables.Conn, table *nftables.Table) *nftables.Chain {
	hookType := nftables.ChainHookInput
	if f.Direction == "outbound" {
		hookType = nftables.ChainHookOutput
	}
	// Policy is ACCEPT: converge rules are additive filters on top of the
	// host's existing baseline. A missing rule falls through to the default
	// policy, not to DROP. This matches the Windows/macOS behavior.
	chainPolicy := nftables.ChainPolicyAccept
	return c.AddChain(&nftables.Chain{
		Name:     f.chainName(),
		Table:    table,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  hookType,
		Priority: nftables.ChainPriorityFilter,
		Policy:   &chainPolicy,
	})
}

func (f *Firewall) buildRule(c *nftables.Conn, table *nftables.Table, chain *nftables.Chain) {
	c.AddRule(&nftables.Rule{
		Table:    table,
		Chain:    chain,
		Exprs:    f.buildExprs(),
		UserData: []byte(f.Name),
	})
}

// buildExprs builds the nftables match/verdict expressions for this rule. It is
// the single source of truth for a rule's on-wire shape: Apply uses it to write
// the rule and the Check decoder (decodeRule) mirrors it to read one back.
func (f *Firewall) buildExprs() []expr.Any {
	var exprs []expr.Any

	exprs = append(exprs,
		&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     []byte{f.protoNum()},
		},
	)

	// Offset 2 = destination port in TCP/UDP header. For inbound traffic this
	// is the local port; for outbound it is the remote port. Port always means
	// "destination port of the packet" across all platforms.
	if f.Port > 0 {
		exprs = append(exprs,
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseTransportHeader,
				Offset:       2,
				Len:          2,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     binaryutil.BigEndian.PutUint16(uint16(f.Port)),
			},
		)
	}

	if f.Source != "" {
		exprs = append(exprs, matchAddr(f.Source, 12)...) // Source IP offset in IPv4.
	}

	if f.Dest != "" {
		exprs = append(exprs, matchAddr(f.Dest, 16)...) // Dest IP offset in IPv4.
	}

	exprs = append(exprs, &expr.Verdict{Kind: f.verdict()})

	return exprs
}

// protoNum returns the IP protocol number for this rule's protocol.
func (f *Firewall) protoNum() byte {
	if strings.EqualFold(f.Protocol, "udp") {
		return byte(unix.IPPROTO_UDP)
	}
	return byte(unix.IPPROTO_TCP)
}

// verdict returns the nftables verdict for this rule's action.
func (f *Firewall) verdict() expr.VerdictKind {
	if f.Action == "block" {
		return expr.VerdictDrop
	}
	return expr.VerdictAccept
}

// matchAddr builds nftables expressions to match an IP or CIDR address.
func matchAddr(addr string, offset uint32) []expr.Any {
	// Try CIDR first.
	if _, ipNet, err := net.ParseCIDR(addr); err == nil {
		ones, _ := ipNet.Mask.Size()
		return []expr.Any{
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       offset,
				Len:          4,
			},
			&expr.Bitwise{
				SourceRegister: 1,
				DestRegister:   1,
				Len:            4,
				Mask:           net.CIDRMask(ones, 32),
				Xor:            []byte{0, 0, 0, 0},
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     ipNet.IP.To4(),
			},
		}
	}

	// Bare IP.
	if ip := net.ParseIP(addr); ip != nil {
		return []expr.Any{
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       offset,
				Len:          4,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     ip.To4(),
			},
		}
	}

	return nil
}

func (f *Firewall) chainName() string {
	if f.Direction == "outbound" {
		return chainOut
	}
	return chainIn
}

func hasUserData(rule *nftables.Rule, name string) bool {
	return string(rule.UserData) == name
}

// ruleAttrs holds the decoded, comparable attributes of an nftables rule. It is
// the read-side mirror of buildExprs: only the fields converge sets are tracked.
type ruleAttrs struct {
	proto      byte
	hasProto   bool
	port       uint16
	hasPort    bool
	verdict    expr.VerdictKind
	hasVerdict bool
	src        addrMatch
	dst        addrMatch
}

// addrMatch is a decoded IPv4 address constraint. prefix is the CIDR prefix
// length (0-32), or -1 when no constraint is present (matches "any").
type addrMatch struct {
	ip     [4]byte
	prefix int
}

// noAddr is the zero constraint: no source/dest match (i.e. "any").
var noAddr = addrMatch{prefix: -1}

// decodeRule walks a rule's expressions and extracts the attributes converge
// cares about. It mirrors buildExprs: protocol via Meta+Cmp, port via a
// transport-header payload+Cmp, source/dest via network-header payloads
// (optionally masked by a Bitwise for CIDRs), and the trailing verdict.
func decodeRule(exprs []expr.Any) ruleAttrs {
	a := ruleAttrs{src: noAddr, dst: noAddr}
	for i := 0; i < len(exprs); i++ {
		switch e := exprs[i].(type) {
		case *expr.Meta:
			if e.Key == expr.MetaKeyL4PROTO {
				if cmp := cmpAt(exprs, i+1); cmp != nil && len(cmp.Data) == 1 {
					a.proto = cmp.Data[0]
					a.hasProto = true
				}
			}
		case *expr.Payload:
			switch {
			case e.Base == expr.PayloadBaseTransportHeader && e.Offset == 2 && e.Len == 2:
				if cmp := cmpAt(exprs, i+1); cmp != nil && len(cmp.Data) == 2 {
					a.port = binary.BigEndian.Uint16(cmp.Data)
					a.hasPort = true
				}
			case e.Base == expr.PayloadBaseNetworkHeader && e.Len == 4:
				m := decodeAddr(exprs, i)
				if e.Offset == 12 {
					a.src = m
				} else if e.Offset == 16 {
					a.dst = m
				}
			}
		case *expr.Verdict:
			a.verdict = e.Kind
			a.hasVerdict = true
		}
	}
	return a
}

// cmpAt returns the expression at index i as a *expr.Cmp, or nil.
func cmpAt(exprs []expr.Any, i int) *expr.Cmp {
	if i >= 0 && i < len(exprs) {
		if cmp, ok := exprs[i].(*expr.Cmp); ok {
			return cmp
		}
	}
	return nil
}

// decodeAddr reads an address constraint starting at the network-header payload
// at index i. A bare IP is payload+Cmp (prefix 32); a CIDR is payload+Bitwise+Cmp
// (prefix derived from the mask). Returns noAddr if the shape is unrecognized.
func decodeAddr(exprs []expr.Any, i int) addrMatch {
	m := addrMatch{prefix: 32}
	j := i + 1
	if j < len(exprs) {
		if bw, ok := exprs[j].(*expr.Bitwise); ok {
			m.prefix = maskToPrefix(bw.Mask)
			j++
		}
	}
	cmp := cmpAt(exprs, j)
	if cmp == nil || len(cmp.Data) != 4 {
		return noAddr
	}
	copy(m.ip[:], cmp.Data)
	return m
}

// maskToPrefix counts the set bits in a netmask to recover the CIDR prefix.
func maskToPrefix(mask []byte) int {
	ones := 0
	for _, b := range mask {
		ones += bits.OnesCount8(b)
	}
	return ones
}

// canonAddr converts a desired source/dest string into its addrMatch form so it
// can be compared against a decoded rule. Empty means "any" (noAddr).
func canonAddr(addr string) addrMatch {
	if addr == "" {
		return noAddr
	}
	if _, ipNet, err := net.ParseCIDR(addr); err == nil {
		m := addrMatch{}
		copy(m.ip[:], ipNet.IP.To4())
		m.prefix, _ = ipNet.Mask.Size()
		return m
	}
	if ip := net.ParseIP(addr); ip != nil {
		m := addrMatch{prefix: 32}
		copy(m.ip[:], ip.To4())
		return m
	}
	return noAddr
}

// attrsMatch reports whether a decoded rule matches this Firewall's desired
// spec across protocol, port, verdict, source, and dest.
func (f *Firewall) attrsMatch(a ruleAttrs) bool {
	if !a.hasProto || a.proto != f.protoNum() {
		return false
	}
	if f.Port > 0 {
		if !a.hasPort || a.port != uint16(f.Port) {
			return false
		}
	} else if a.hasPort {
		return false
	}
	if !a.hasVerdict || a.verdict != f.verdict() {
		return false
	}
	if a.src != canonAddr(f.Source) {
		return false
	}
	if a.dst != canonAddr(f.Dest) {
		return false
	}
	return true
}

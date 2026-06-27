//go:build linux

package firewall

import (
	"testing"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"
)

// TestChainName verifies the direction->chain mapping that selects which
// nftables hook chain a rule lands in.
func TestChainName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		direction string
		want      string
	}{
		{"inbound", "inbound", chainIn},
		{"outbound", "outbound", chainOut},
		{"empty defaults to input", "", chainIn},
		{"unknown defaults to input", "sideways", chainIn},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := Firewall{Direction: tt.direction}
			if got := f.chainName(); got != tt.want {
				t.Errorf("chainName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHasUserData verifies the name-based rule identifier match used to find
// converge-managed rules. UserData carries the rule name verbatim.
func TestHasUserData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		userData []byte
		match    string
		want     bool
	}{
		{"exact match", []byte("Allow SSH"), "Allow SSH", true},
		{"mismatch", []byte("Allow SSH"), "Block RDP", false},
		{"empty userdata vs name", nil, "Allow SSH", false},
		{"empty userdata vs empty name", nil, "", true},
		{"prefix is not a match", []byte("Allow"), "Allow SSH", false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rule := &nftables.Rule{UserData: tt.userData}
			if got := hasUserData(rule, tt.match); got != tt.want {
				t.Errorf("hasUserData(%q, %q) = %v, want %v", tt.userData, tt.match, got, tt.want)
			}
		})
	}
}

// TestMatchAddr verifies the address-matching expression builder for bare IPs,
// CIDRs, and unparseable input (which yields no constraint at all).
func TestMatchAddr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		addr     string
		wantLen  int // number of expressions emitted
		wantMask bool
	}{
		{"bare IP", "10.0.0.1", 2, false}, // payload + cmp
		{"CIDR", "10.0.0.0/8", 3, true},   // payload + bitwise + cmp
		{"CIDR /32", "192.168.1.5/32", 3, true},
		{"invalid yields nil", "not-an-ip", 0, false},
		{"empty yields nil", "", 0, false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := matchAddr(tt.addr, 12)
			if len(got) != tt.wantLen {
				t.Fatalf("matchAddr(%q) returned %d exprs, want %d", tt.addr, len(got), tt.wantLen)
			}
			hasMask := false
			for _, e := range got {
				if _, ok := e.(*expr.Bitwise); ok {
					hasMask = true
				}
			}
			if hasMask != tt.wantMask {
				t.Errorf("matchAddr(%q) bitwise mask present = %v, want %v", tt.addr, hasMask, tt.wantMask)
			}
		})
	}
}

// TestCmpAt covers the index-safe Cmp accessor: in-range Cmp, out-of-range, and
// an in-range expression that is not a Cmp.
func TestCmpAt(t *testing.T) {
	t.Parallel()

	exprs := []expr.Any{
		&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{6}},
	}

	if got := cmpAt(exprs, 1); got == nil {
		t.Error("cmpAt(1) = nil, want the Cmp expression")
	}
	if got := cmpAt(exprs, 0); got != nil {
		t.Error("cmpAt(0) should be nil: index 0 is a Meta, not a Cmp")
	}
	if got := cmpAt(exprs, 5); got != nil {
		t.Error("cmpAt(5) should be nil: index out of range")
	}
	if got := cmpAt(exprs, -1); got != nil {
		t.Error("cmpAt(-1) should be nil: negative index")
	}
}

// TestDecodeAddr_Malformed verifies decodeAddr falls back to noAddr when the
// trailing comparison is missing or the wrong width.
func TestDecodeAddr_Malformed(t *testing.T) {
	t.Parallel()

	// Network-header payload with no following Cmp: unrecognized shape.
	noCmp := []expr.Any{
		&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseNetworkHeader, Offset: 12, Len: 4},
	}
	if got := decodeAddr(noCmp, 0); got != noAddr {
		t.Errorf("decodeAddr(no cmp) = %+v, want noAddr", got)
	}

	// Payload + Cmp but with a 2-byte data (port-sized): not a 4-byte address.
	wrongWidth := []expr.Any{
		&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseNetworkHeader, Offset: 12, Len: 4},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{1, 2}},
	}
	if got := decodeAddr(wrongWidth, 0); got != noAddr {
		t.Errorf("decodeAddr(wrong width) = %+v, want noAddr", got)
	}
}

// TestCanonAddr covers the desired-spec address canonicalizer across bare IPs,
// CIDRs, the empty "any" case, and unparseable input.
func TestCanonAddr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		addr       string
		wantPrefix int
	}{
		{"empty is any", "", -1},
		{"bare IP is /32", "10.0.0.1", 32},
		{"CIDR keeps prefix", "10.0.0.0/8", 8},
		{"invalid is any", "garbage", -1},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := canonAddr(tt.addr); got.prefix != tt.wantPrefix {
				t.Errorf("canonAddr(%q).prefix = %d, want %d", tt.addr, got.prefix, tt.wantPrefix)
			}
		})
	}
}

// TestAttrsMatch_MissingAndExtraFields exercises the early-return drift paths in
// attrsMatch that the round-trip test cannot reach: a decoded rule missing
// protocol/verdict, and a stored rule carrying a port the desired spec omits.
func TestAttrsMatch_MissingAndExtraFields(t *testing.T) {
	t.Parallel()

	base := Firewall{Name: "rule", Port: 22, Protocol: "tcp", Direction: "inbound", Action: "allow"}

	full := ruleAttrs{
		proto: byte(unix.IPPROTO_TCP), hasProto: true,
		port: 22, hasPort: true,
		verdict: expr.VerdictAccept, hasVerdict: true,
		src: noAddr, dst: noAddr,
	}
	if !base.attrsMatch(full) {
		t.Fatal("attrsMatch should match a fully-populated decoded rule")
	}

	tests := []struct {
		name    string
		mutate  func(ruleAttrs) ruleAttrs
		desired Firewall
		want    bool
	}{
		{
			name:    "missing proto is drift",
			mutate:  func(a ruleAttrs) ruleAttrs { a.hasProto = false; return a },
			desired: base,
		},
		{
			name:    "wrong proto is drift",
			mutate:  func(a ruleAttrs) ruleAttrs { a.proto = byte(unix.IPPROTO_UDP); return a },
			desired: base,
		},
		{
			name:    "missing verdict is drift",
			mutate:  func(a ruleAttrs) ruleAttrs { a.hasVerdict = false; return a },
			desired: base,
		},
		{
			name:    "stored has port but desired omits it is drift",
			mutate:  func(a ruleAttrs) ruleAttrs { return a },
			desired: Firewall{Name: "rule", Port: 0, Protocol: "tcp", Direction: "inbound", Action: "allow"},
		},
		{
			name:    "desired wants port but stored lacks it is drift",
			mutate:  func(a ruleAttrs) ruleAttrs { a.hasPort = false; return a },
			desired: base,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.desired.attrsMatch(tt.mutate(full)); got != tt.want {
				t.Errorf("attrsMatch() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestBuildRule_Queues exercises ensureTable, ensureChain, and buildRule. These
// only enqueue netlink messages on the connection (no Flush, so no privileged
// socket write happens), letting the encode path run without root. It asserts
// the chain hook is selected by direction.
func TestBuildRule_Queues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		direction string
		wantChain string
		wantHook  *nftables.ChainHook
	}{
		{"inbound", "inbound", chainIn, nftables.ChainHookInput},
		{"outbound", "outbound", chainOut, nftables.ChainHookOutput},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, err := nftables.New()
			if err != nil {
				t.Skipf("cannot open nftables netlink socket: %v", err)
			}
			f := Firewall{Name: "rule", Port: 22, Protocol: "tcp", Direction: tt.direction, Action: "allow"}

			table := ensureTable(c)
			if table.Name != tableName {
				t.Errorf("ensureTable name = %q, want %q", table.Name, tableName)
			}
			if table.Family != nftables.TableFamilyIPv4 {
				t.Errorf("ensureTable family = %v, want IPv4", table.Family)
			}

			chain := f.ensureChain(c, table)
			if chain.Name != tt.wantChain {
				t.Errorf("ensureChain name = %q, want %q", chain.Name, tt.wantChain)
			}
			if chain.Hooknum == nil || *chain.Hooknum != *tt.wantHook {
				t.Errorf("ensureChain hook = %v, want %v", chain.Hooknum, tt.wantHook)
			}
			if chain.Policy == nil || *chain.Policy != nftables.ChainPolicyAccept {
				t.Error("ensureChain policy should be ACCEPT (additive filter)")
			}

			// Should not panic; rule is queued, not flushed.
			f.buildRule(c, table, chain)
		})
	}
}

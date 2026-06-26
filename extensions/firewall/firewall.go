// Package firewall manages host firewall rules across platforms.
// Windows uses the registry-based FirewallRules API, Linux uses nftables
// via netlink (inet family, IPv4 only), and macOS uses pf via anchors.
package firewall

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/TsekNet/converge/extensions"
)

// Opts configures a Firewall resource.
type Opts struct {
	Port      int
	Protocol  string // "tcp" or "udp".
	Direction string // "inbound" or "outbound".
	Action    string // "allow" or "block".
	Source    string // Optional source IP or CIDR. Empty = any.
	Dest      string // Optional destination IP or CIDR. Empty = any.
	State     string // "present" (default) or "absent".
	Critical  bool
}

// Firewall manages a single named firewall rule.
type Firewall struct {
	Name      string // Human-readable rule name (used as identifier).
	Port      int
	Protocol  string // "tcp" or "udp".
	Direction string // "inbound" or "outbound".
	Action    string // "allow" or "block".
	Source    string // Optional source IP or CIDR. Empty = any.
	Dest      string // Optional destination IP or CIDR. Empty = any.
	State     string // "present" (default) or "absent".
	Critical  bool

	// valErr holds a validation failure detected at construction. It is
	// surfaced as an error from Check/Apply rather than a panic, so one
	// malformed rule cannot crash the whole run (the engine accumulates and
	// reports it like any other resource failure).
	valErr error
}

// validName must never allow '|' or '=' to prevent Windows firewall rule injection.
var validName = regexp.MustCompile(`^[a-zA-Z0-9 _-]+$`)

var validProtocols = map[string]bool{"tcp": true, "udp": true}
var validDirections = map[string]bool{"inbound": true, "outbound": true}
var validActions = map[string]bool{"allow": true, "block": true}
var validStates = map[string]bool{"present": true, "absent": true}

func New(name string, opts Opts) *Firewall {
	state := opts.State
	if state == "" {
		state = "present"
	}
	f := &Firewall{
		Name:      name,
		Port:      opts.Port,
		Protocol:  opts.Protocol,
		Direction: opts.Direction,
		Action:    opts.Action,
		Source:    opts.Source,
		Dest:      opts.Dest,
		State:     state,
		Critical:  opts.Critical,
	}
	// Validation failures are recorded, not panicked: invalid input must be
	// reported through the normal Check/Apply error path so it cannot abort an
	// entire converge run (this mirrors the DSL's accumulate-errors contract).
	f.valErr = f.Validate()
	return f
}

// validErr returns the construction-time validation error, if any. Platform
// Check/Apply implementations must call this first so a malformed rule fails
// cleanly instead of attempting (and mis-reporting) enforcement.
func (f *Firewall) validErr() error { return f.valErr }

// Validate checks all fields for correctness and rejects values that
// could cause rule injection or produce silently broken rules.
func (f *Firewall) Validate() error {
	if f.Name == "" || !validName.MatchString(f.Name) {
		return fmt.Errorf("name must match %s, got %q", validName.String(), f.Name)
	}
	if f.Port < 1 || f.Port > 65535 {
		return fmt.Errorf("port must be 1-65535, got %d", f.Port)
	}
	if !validProtocols[f.Protocol] {
		return fmt.Errorf("protocol must be tcp or udp, got %q", f.Protocol)
	}
	if !validDirections[f.Direction] {
		return fmt.Errorf("direction must be inbound or outbound, got %q", f.Direction)
	}
	if !validActions[f.Action] {
		return fmt.Errorf("action must be allow or block, got %q", f.Action)
	}
	if !validStates[f.State] {
		return fmt.Errorf("state must be present or absent, got %q", f.State)
	}
	if f.Source != "" {
		if err := validateAddr(f.Source); err != nil {
			return fmt.Errorf("source: %w", err)
		}
	}
	if f.Dest != "" {
		if err := validateAddr(f.Dest); err != nil {
			return fmt.Errorf("dest: %w", err)
		}
	}
	return nil
}

// validateAddr accepts IPv4 addresses and CIDRs only. IPv6 is not yet
// supported by the nftables and pf rule builders.
func validateAddr(addr string) error {
	if ip := net.ParseIP(addr); ip != nil {
		if ip.To4() == nil {
			return fmt.Errorf("IPv6 not supported, got %q", addr)
		}
		return nil
	}
	if _, ipNet, err := net.ParseCIDR(addr); err == nil {
		if ipNet.IP.To4() == nil {
			return fmt.Errorf("IPv6 CIDR not supported, got %q", addr)
		}
		return nil
	}
	return fmt.Errorf("must be a valid IPv4 address or CIDR, got %q", addr)
}

// normalizeIPv4 canonicalizes an IPv4 address or CIDR into "ip/prefix" form so
// values can be compared regardless of notation. The Windows firewall API may
// return an address as a bare IP, "ip/prefixlen", or "ip/dotted-mask", while
// converge stores them as bare IPs or "ip/prefixlen". A bare IP normalizes to
// "/32". Inputs that are not a single IPv4 address/CIDR (lists, ranges,
// keywords like "*") are returned trimmed so callers fall back to exact compare.
func normalizeIPv4(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	ipPart, maskPart, hasMask := strings.Cut(addr, "/")
	ip := net.ParseIP(ipPart)
	if ip == nil || ip.To4() == nil {
		return addr
	}
	prefix := 32
	if hasMask {
		if pl, err := strconv.Atoi(maskPart); err == nil {
			prefix = pl
		} else if m := net.ParseIP(maskPart); m != nil && m.To4() != nil {
			prefix, _ = net.IPMask(m.To4()).Size()
		} else {
			return addr
		}
	}
	return fmt.Sprintf("%s/%d", ip.To4().String(), prefix)
}

// addrEqual reports whether two IPv4 address/CIDR specs are equivalent,
// tolerating notation differences (bare IP vs /32, prefixlen vs dotted mask).
func addrEqual(a, b string) bool {
	return normalizeIPv4(a) == normalizeIPv4(b)
}

// checkResult builds a State from whether the rule exists and whether we want it.
// Shared across all platform Check implementations.
func checkResult(name string, exists, wantPresent bool) (*extensions.State, error) {
	if exists == wantPresent {
		return &extensions.State{InSync: true}, nil
	}
	action := "add"
	if !wantPresent {
		action = "remove"
	}
	return &extensions.State{
		InSync: false,
		Changes: []extensions.Change{{
			Property: "rule",
			From:     boolToState(!wantPresent),
			To:       boolToState(wantPresent),
			Action:   action,
		}},
	}, nil
}

func (f *Firewall) ID() string { return fmt.Sprintf("firewall:%s", f.Name) }
func (f *Firewall) String() string {
	return fmt.Sprintf("Firewall %s (%s/%d %s)", f.Name, f.Protocol, f.Port, f.Action)
}
func (f *Firewall) IsCritical() bool { return f.Critical }

// resultChanged builds a success Result with the given message.
// Shared across all platform Apply implementations.
func resultChanged(message string) (*extensions.Result, error) {
	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: message}, nil
}

// boolToState converts a boolean to "present" or "absent".
func boolToState(b bool) string {
	if b {
		return "present"
	}
	return "absent"
}

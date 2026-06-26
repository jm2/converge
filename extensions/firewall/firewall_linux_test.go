//go:build linux

package firewall

import "testing"

// TestAttrsMatch_RoundTrip verifies the content-aware Check logic: a rule's
// expressions are built (buildExprs), read back (decodeRule), and compared
// (attrsMatch) against a desired spec. This is the seam that closes the
// fail-open bug where a name-matching rule flipped block->allow (or re-pointed
// to another port/source) was reported in-sync. No live nftables socket is
// needed because the builder and decoder are pure functions over []expr.Any.
func TestAttrsMatch_RoundTrip(t *testing.T) {
	t.Parallel()

	base := func() Firewall {
		return Firewall{Name: "rule", Port: 22, Protocol: "tcp", Direction: "inbound", Action: "allow", State: "present"}
	}

	tests := []struct {
		name    string
		stored  Firewall // the rule as written to nftables (its exprs are decoded)
		desired Firewall // the spec Check compares against
		want    bool
	}{
		{
			name:    "identical",
			stored:  base(),
			desired: base(),
			want:    true,
		},
		{
			name:    "action flipped allow->block is drift",
			stored:  func() Firewall { f := base(); f.Action = "block"; return f }(),
			desired: base(),
			want:    false,
		},
		{
			name:    "port re-pointed is drift",
			stored:  func() Firewall { f := base(); f.Port = 2222; return f }(),
			desired: base(),
			want:    false,
		},
		{
			name:    "protocol changed is drift",
			stored:  func() Firewall { f := base(); f.Protocol = "udp"; return f }(),
			desired: base(),
			want:    false,
		},
		{
			name:    "source added is drift",
			stored:  base(),
			desired: func() Firewall { f := base(); f.Source = "10.0.0.1"; return f }(),
			want:    false,
		},
		{
			name:    "source removed is drift",
			stored:  func() Firewall { f := base(); f.Source = "10.0.0.1"; return f }(),
			desired: base(),
			want:    false,
		},
		{
			name:    "source re-pointed is drift",
			stored:  func() Firewall { f := base(); f.Source = "10.0.0.1"; return f }(),
			desired: func() Firewall { f := base(); f.Source = "10.0.0.2"; return f }(),
			want:    false,
		},
		{
			name:    "matching bare source IP",
			stored:  func() Firewall { f := base(); f.Source = "10.0.0.1"; return f }(),
			desired: func() Firewall { f := base(); f.Source = "10.0.0.1"; return f }(),
			want:    true,
		},
		{
			name:    "bare IP equals /32 CIDR",
			stored:  func() Firewall { f := base(); f.Source = "10.0.0.1"; return f }(),
			desired: func() Firewall { f := base(); f.Source = "10.0.0.1/32"; return f }(),
			want:    true,
		},
		{
			name:    "matching source CIDR",
			stored:  func() Firewall { f := base(); f.Source = "10.0.0.0/8"; return f }(),
			desired: func() Firewall { f := base(); f.Source = "10.0.0.0/8"; return f }(),
			want:    true,
		},
		{
			name:    "differing CIDR prefix is drift",
			stored:  func() Firewall { f := base(); f.Source = "10.0.0.0/8"; return f }(),
			desired: func() Firewall { f := base(); f.Source = "10.0.0.0/16"; return f }(),
			want:    false,
		},
		{
			name:    "matching dest IP",
			stored:  func() Firewall { f := base(); f.Dest = "192.168.1.5"; return f }(),
			desired: func() Firewall { f := base(); f.Dest = "192.168.1.5"; return f }(),
			want:    true,
		},
		{
			name:    "dest re-pointed is drift",
			stored:  func() Firewall { f := base(); f.Dest = "192.168.1.5"; return f }(),
			desired: func() Firewall { f := base(); f.Dest = "192.168.1.6"; return f }(),
			want:    false,
		},
		{
			name:    "source and dest both match",
			stored:  func() Firewall { f := base(); f.Source = "10.0.0.0/24"; f.Dest = "192.168.1.5"; return f }(),
			desired: func() Firewall { f := base(); f.Source = "10.0.0.0/24"; f.Dest = "192.168.1.5"; return f }(),
			want:    true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			exprs := tt.stored.buildExprs()
			if got := tt.desired.attrsMatch(decodeRule(exprs)); got != tt.want {
				t.Errorf("attrsMatch() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDecodeRule_Attributes spot-checks the decoder against a known builder
// output so a future change to either side that breaks the round trip is caught.
func TestDecodeRule_Attributes(t *testing.T) {
	t.Parallel()

	f := Firewall{Name: "rule", Port: 443, Protocol: "udp", Direction: "inbound", Action: "block", Source: "10.1.2.0/24", Dest: "192.168.0.9"}
	a := decodeRule(f.buildExprs())

	if !a.hasProto || a.proto != f.protoNum() {
		t.Errorf("proto = %d (has=%v), want %d", a.proto, a.hasProto, f.protoNum())
	}
	if !a.hasPort || a.port != uint16(f.Port) {
		t.Errorf("port = %d (has=%v), want %d", a.port, a.hasPort, f.Port)
	}
	if !a.hasVerdict || a.verdict != f.verdict() {
		t.Errorf("verdict = %d (has=%v), want %d", a.verdict, a.hasVerdict, f.verdict())
	}
	if a.src != canonAddr(f.Source) {
		t.Errorf("src = %+v, want %+v", a.src, canonAddr(f.Source))
	}
	if a.dst != canonAddr(f.Dest) {
		t.Errorf("dst = %+v, want %+v", a.dst, canonAddr(f.Dest))
	}
}

// TestDecodeRule_NoAddrConstraints verifies a rule with no source/dest decodes
// to the "any" sentinel, so an unconstrained stored rule is not mistaken for a
// constrained one (and vice versa).
func TestDecodeRule_NoAddrConstraints(t *testing.T) {
	t.Parallel()

	f := Firewall{Name: "rule", Port: 22, Protocol: "tcp", Direction: "inbound", Action: "allow"}
	a := decodeRule(f.buildExprs())
	if a.src != noAddr {
		t.Errorf("src = %+v, want noAddr %+v", a.src, noAddr)
	}
	if a.dst != noAddr {
		t.Errorf("dst = %+v, want noAddr %+v", a.dst, noAddr)
	}
}

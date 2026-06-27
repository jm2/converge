package firewall

import (
	"testing"
	"time"
)

// TestPollInterval verifies firewall resources fall back to polling at a fixed
// cadence (no platform exposes reliable native firewall-change events).
func TestPollInterval(t *testing.T) {
	t.Parallel()

	f := New("Allow SSH", Opts{Port: 22, Protocol: "tcp", Direction: "inbound", Action: "allow"})
	if got := f.PollInterval(); got != 30*time.Second {
		t.Errorf("PollInterval() = %v, want %v", got, 30*time.Second)
	}
}

// TestNormalizeIPv4 covers the notation-canonicalizer used by addrEqual,
// including the fall-through cases for non-IP input and malformed masks.
func TestNormalizeIPv4(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"bare IP gets /32", "10.0.0.1", "10.0.0.1/32"},
		{"prefixlen preserved", "10.0.0.0/8", "10.0.0.0/8"},
		{"dotted mask to prefixlen", "10.0.0.0/255.255.255.0", "10.0.0.0/24"},
		{"non-ip returned as-is", "LocalSubnet", "LocalSubnet"},
		{"whitespace trimmed", "  10.0.0.1  ", "10.0.0.1/32"},
		{"malformed mask returned as-is", "10.0.0.1/notamask", "10.0.0.1/notamask"},
		{"ipv6 returned as-is", "fd00::1", "fd00::1"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeIPv4(tt.in); got != tt.want {
				t.Errorf("normalizeIPv4(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

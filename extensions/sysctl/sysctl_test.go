package sysctl

import "testing"

func TestSysctl_ID(t *testing.T) {
	s := New("net.ipv4.ip_forward", Opts{Value: "0"})
	if got := s.ID(); got != "sysctl:net.ipv4.ip_forward" {
		t.Errorf("ID() = %q, want %q", got, "sysctl:net.ipv4.ip_forward")
	}
}

func TestSysctl_String(t *testing.T) {
	s := New("net.ipv4.ip_forward", Opts{Value: "0"})
	if got := s.String(); got != "Sysctl net.ipv4.ip_forward = 0" {
		t.Errorf("String() = %q, want %q", got, "Sysctl net.ipv4.ip_forward = 0")
	}
}

func TestSysctl_IsCritical(t *testing.T) {
	s := New("net.ipv4.ip_forward", Opts{Value: "0"})
	if s.IsCritical() {
		t.Error("IsCritical() should be false by default")
	}
	s2 := New("net.ipv4.ip_forward", Opts{Value: "0", Critical: true})
	if !s2.IsCritical() {
		t.Error("IsCritical() should be true when set via Opts")
	}
}

func TestSysctl_New_Defaults(t *testing.T) {
	s := New("kernel.randomize_va_space", Opts{Value: "2", Persist: true})
	if s.Key != "kernel.randomize_va_space" {
		t.Errorf("Key = %q", s.Key)
	}
	if s.Value != "2" {
		t.Errorf("Value = %q", s.Value)
	}
	if !s.Persist {
		t.Error("Persist should be true when set via Opts")
	}
}

func TestSysctl_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid dotted key", "net.ipv4.ip_forward", false},
		{"valid with hyphen", "net.ipv4.conf.all.rp-filter", false},
		{"valid single segment", "hostname", false},
		{"path traversal double dot", "net..ipv4.ip_forward", true},
		{"leading double dot", "..secret", true},
		{"shell metachar semicolon", "net.ipv4;rm -rf /", true},
		{"space in key", "net.ipv4 .ip_forward", true},
		{"slash in key", "net/ipv4", true},
		{"empty key", "", true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := New(tt.key, Opts{Value: "1"})
			err := s.validate()
			if tt.wantErr && err == nil {
				t.Errorf("validate(%q) = nil, want error", tt.key)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validate(%q) = %v, want nil", tt.key, err)
			}
		})
	}
}

//go:build linux

package sysctl

import "testing"

// These tests exercise Check / keyToPath, which are defined only in
// sysctl_linux.go, so the file is Linux-tagged. Previously they lived in
// sysctl_test.go guarded by a runtime.GOOS check, but that does not prevent the
// compile error on macOS/Windows where the methods do not exist.

func TestSysctl_Check_ReadOnly(t *testing.T) {
	s := New("kernel.ostype", Opts{Value: "Linux"})
	state, err := s.Check(nil)
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if !state.InSync {
		t.Error("kernel.ostype should be 'Linux' on a Linux system")
	}
}

func TestKeyToPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
		want string
	}{
		{"ipv4_forward", "net.ipv4.ip_forward", "/proc/sys/net/ipv4/ip_forward"},
		{"kernel_ostype", "kernel.ostype", "/proc/sys/kernel/ostype"},
		{"nested_conf", "net.ipv4.conf.all.forwarding", "/proc/sys/net/ipv4/conf/all/forwarding"},
		{"randomize_va", "kernel.randomize_va_space", "/proc/sys/kernel/randomize_va_space"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := keyToPath(tt.key); got != tt.want {
				t.Errorf("keyToPath(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestSysctl_Check_Mismatch(t *testing.T) {
	t.Parallel()

	s := New("kernel.ostype", Opts{Value: "NotLinux"})
	state, err := s.Check(nil)
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if state.InSync {
		t.Fatal("expected InSync=false for mismatched value")
	}
	if len(state.Changes) == 0 {
		t.Fatal("expected at least one Change")
	}
	c := state.Changes[0]
	if c.From != "Linux" {
		t.Errorf("Changes[0].From = %q, want %q", c.From, "Linux")
	}
	if c.To != "NotLinux" {
		t.Errorf("Changes[0].To = %q, want %q", c.To, "NotLinux")
	}
}

func TestSysctl_Check_NonexistentKey(t *testing.T) {
	t.Parallel()

	s := New("nonexistent.fake.key", Opts{Value: "x"})
	_, err := s.Check(nil)
	if err == nil {
		t.Error("expected error for nonexistent sysctl key")
	}
}

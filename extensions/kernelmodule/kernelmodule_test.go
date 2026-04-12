package kernelmodule

import (
	"testing"
)

func TestKernelModule_ID(t *testing.T) {
	k := New("cramfs", Opts{State: Blacklisted})
	if got := k.ID(); got != "kernelmodule:cramfs" {
		t.Errorf("ID() = %q, want %q", got, "kernelmodule:cramfs")
	}
}

func TestKernelModule_String(t *testing.T) {
	tests := []struct {
		module string
		state  StateType
		want   string
	}{
		{"cramfs", Blacklisted, "KernelModule cramfs (blacklisted)"},
		{"vfat", Loaded, "KernelModule vfat (loaded)"},
	}
	for _, tt := range tests {
		t.Run(tt.module, func(t *testing.T) {
			k := New(tt.module, Opts{State: tt.state})
			if got := k.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestKernelModule_IsCritical(t *testing.T) {
	k := New("cramfs", Opts{State: Blacklisted})
	if k.IsCritical() {
		t.Error("IsCritical() should be false by default")
	}
	k2 := New("cramfs", Opts{State: Blacklisted, Critical: true})
	if !k2.IsCritical() {
		t.Error("IsCritical() should be true when set via Opts")
	}
}

func TestNew(t *testing.T) {
	k := New("ext4", Opts{State: Loaded})
	if k.Module != "ext4" {
		t.Errorf("Module = %q, want %q", k.Module, "ext4")
	}
	if k.State != Loaded {
		t.Errorf("State = %q, want %q", k.State, Loaded)
	}
}

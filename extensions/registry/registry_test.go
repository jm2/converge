package registry

import "testing"

func TestRegistry_IDAndString(t *testing.T) {
	tests := []struct {
		key     string
		value   string
		wantID  string
		wantStr string
	}{
		{`HKLM\SOFTWARE\Test`, "Foo", `registry:HKLM\SOFTWARE\Test\Foo`, `Registry HKLM\SOFTWARE\Test\Foo`},
		{`HKCU\Control Panel`, "Bar", `registry:HKCU\Control Panel\Bar`, `Registry HKCU\Control Panel\Bar`},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			r := New(tt.key, Opts{Value: tt.value})
			if r.ID() != tt.wantID {
				t.Errorf("ID() = %q, want %q", r.ID(), tt.wantID)
			}
			if r.String() != tt.wantStr {
				t.Errorf("String() = %q, want %q", r.String(), tt.wantStr)
			}
		})
	}
}

func TestRegistry_IsCritical(t *testing.T) {
	r := New(`HKLM\SOFTWARE\Test`, Opts{})
	if r.IsCritical() {
		t.Error("IsCritical() should be false by default")
	}
	r2 := New(`HKLM\SOFTWARE\Test`, Opts{Critical: true})
	if !r2.IsCritical() {
		t.Error("IsCritical() should be true when set")
	}
}

func TestRegistry_DefaultState(t *testing.T) {
	r := New(`HKLM\SOFTWARE\Test`, Opts{})
	if r.State != "present" {
		t.Errorf("default State = %q, want %q", r.State, "present")
	}
}

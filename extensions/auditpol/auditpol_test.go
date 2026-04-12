package auditpol

import "testing"

func TestAuditPolicy_IDAndString(t *testing.T) {
	tests := []struct {
		subcategory string
		wantID      string
		wantStr     string
	}{
		{"Credential Validation", "auditpol:Credential Validation", "AuditPolicy Credential Validation"},
		{"Logon", "auditpol:Logon", "AuditPolicy Logon"},
		{"Process Creation", "auditpol:Process Creation", "AuditPolicy Process Creation"},
	}
	for _, tt := range tests {
		t.Run(tt.subcategory, func(t *testing.T) {
			a := New(tt.subcategory, Opts{Subcategory: tt.subcategory, Success: true, Failure: true})
			if a.ID() != tt.wantID {
				t.Errorf("ID() = %q, want %q", a.ID(), tt.wantID)
			}
			if a.String() != tt.wantStr {
				t.Errorf("String() = %q, want %q", a.String(), tt.wantStr)
			}
		})
	}
}

func TestAuditPolicy_IsCritical(t *testing.T) {
	a := New("Logon", Opts{Subcategory: "Logon", Success: true})
	if a.IsCritical() {
		t.Error("IsCritical() should be false by default")
	}
	a2 := New("Logon", Opts{Subcategory: "Logon", Success: true, Critical: true})
	if !a2.IsCritical() {
		t.Error("IsCritical() should be true when set")
	}
}

func TestAuditPolicy_New(t *testing.T) {
	a := New("Credential Validation", Opts{Subcategory: "Credential Validation", Success: true})
	if a.Subcategory != "Credential Validation" {
		t.Errorf("Subcategory = %q, want %q", a.Subcategory, "Credential Validation")
	}
	if !a.Success {
		t.Error("Success should be true")
	}
	if a.Failure {
		t.Error("Failure should be false")
	}
}

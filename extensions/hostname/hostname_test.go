package hostname

import (
	"context"
	"os"
	"testing"
)

func TestHostname_ID(t *testing.T) {
	h := New("web01.example.com", Opts{})
	if got := h.ID(); got != "hostname:web01.example.com" {
		t.Errorf("ID() = %q, want %q", got, "hostname:web01.example.com")
	}
}

func TestHostname_String(t *testing.T) {
	h := New("web01.example.com", Opts{})
	if got := h.String(); got != "Hostname web01.example.com" {
		t.Errorf("String() = %q, want %q", got, "Hostname web01.example.com")
	}
}

func TestHostname_IsCritical(t *testing.T) {
	h := New("test", Opts{})
	if h.IsCritical() {
		t.Error("IsCritical() should be false by default")
	}
	h2 := New("test", Opts{Critical: true})
	if !h2.IsCritical() {
		t.Error("IsCritical() should be true when set via Opts")
	}
}

func TestHostname_Check_CurrentHostname(t *testing.T) {
	ctx := context.Background()
	current, err := os.Hostname()
	if err != nil {
		t.Fatalf("os.Hostname() error: %v", err)
	}

	h := New(current, Opts{})
	state, err := h.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !state.InSync {
		t.Errorf("should be in sync when desired matches current hostname %q", current)
	}
}

func TestHostname_Check_DifferentHostname(t *testing.T) {
	ctx := context.Background()

	h := New("definitely-not-the-current-hostname-xyz", Opts{})
	state, err := h.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if state.InSync {
		t.Error("should not be in sync when desired differs from current hostname")
	}
	if len(state.Changes) != 1 {
		t.Fatalf("want 1 change, got %d", len(state.Changes))
	}
	if state.Changes[0].Property != "hostname" {
		t.Errorf("Change.Property = %q, want %q", state.Changes[0].Property, "hostname")
	}
	if state.Changes[0].To != "definitely-not-the-current-hostname-xyz" {
		t.Errorf("Change.To = %q", state.Changes[0].To)
	}
}

func TestNew(t *testing.T) {
	h := New("server01", Opts{})
	if h.Name != "server01" {
		t.Errorf("Name = %q, want %q", h.Name, "server01")
	}
	if h.Critical {
		t.Error("Critical should default to false")
	}
}

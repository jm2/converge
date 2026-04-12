package testutil

import (
	"context"
	"testing"

	"github.com/TsekNet/converge/extensions"
)

// AssertConverges verifies the Check/Apply/Check contract:
//  1. Check returns not-in-sync
//  2. Apply succeeds and reports Changed
//  3. Check returns in-sync (convergence proof)
func AssertConverges(t *testing.T, ext extensions.Extension) {
	t.Helper()
	ctx := context.Background()

	state, err := ext.Check(ctx)
	if err != nil {
		t.Fatalf("Check() before Apply: %v", err)
	}
	if state.InSync {
		t.Fatal("Check() should report not-in-sync before Apply")
	}

	result, err := ext.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply(): %v", err)
	}
	if !result.Changed {
		t.Error("Apply() should report Changed=true")
	}

	state, err = ext.Check(ctx)
	if err != nil {
		t.Fatalf("Check() after Apply: %v", err)
	}
	if !state.InSync {
		t.Errorf("Check() should report in-sync after Apply, changes: %+v", state.Changes)
	}
}

// AssertInSync verifies that Check reports the resource is already in sync.
func AssertInSync(t *testing.T, ext extensions.Extension) {
	t.Helper()
	ctx := context.Background()

	state, err := ext.Check(ctx)
	if err != nil {
		t.Fatalf("Check(): %v", err)
	}
	if !state.InSync {
		t.Errorf("expected in-sync, got changes: %+v", state.Changes)
	}
}

// AssertDrifted verifies that Check reports drift with at least one change.
func AssertDrifted(t *testing.T, ext extensions.Extension) {
	t.Helper()
	ctx := context.Background()

	state, err := ext.Check(ctx)
	if err != nil {
		t.Fatalf("Check(): %v", err)
	}
	if state.InSync {
		t.Error("expected drift, got in-sync")
	}
	if len(state.Changes) == 0 {
		t.Error("expected at least one change")
	}
}

//go:build !windows

package file

import (
	"context"
	"testing"

	"github.com/TsekNet/converge/internal/testutil"
)

// TestFile_MapFS_OwnershipDrift verifies Owner/Group are checked and enforced:
// content and mode match, but ownership differs, so Check reports drift and
// Apply chowns to the desired ids. Numeric ids avoid os/user lookups.
func TestFile_MapFS_OwnershipDrift(t *testing.T) {
	ctx := context.Background()
	mfs := testutil.NewMapFS()
	mfs.Set("/etc/secret", []byte("data\n"), 0600)
	mfs.SetOwner("/etc/secret", 1000, 1000) // currently a non-root user

	f := New("/etc/secret", Opts{Content: "data\n", Mode: 0600, Owner: "4242", Group: "4343", FS: mfs})

	state, err := f.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if state.InSync {
		t.Fatal("should detect ownership drift (1000:1000 != 4242:4343)")
	}
	foundOwner := false
	for _, c := range state.Changes {
		if c.Property == "owner" {
			foundOwner = true
		}
	}
	if !foundOwner {
		t.Errorf("expected an owner change, got %+v", state.Changes)
	}

	if _, err := f.Apply(ctx); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	uid, gid, _ := mfs.Owner("/etc/secret")
	if uid != 4242 || gid != 4343 {
		t.Errorf("after Apply owner = %d:%d, want 4242:4343", uid, gid)
	}

	// Idempotent: ownership now matches.
	state, err = f.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !state.InSync {
		t.Errorf("should be in sync after chown, changes: %+v", state.Changes)
	}
}

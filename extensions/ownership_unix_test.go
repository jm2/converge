//go:build unix

package extensions

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
)

// TestOSFS_Owner reads back the uid/gid of a freshly created file, which must
// match the test process's own uid/gid.
func TestOSFS_Owner(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "owned.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	uid, gid, err := OSFS{}.Owner(file)
	if err != nil {
		t.Fatalf("Owner: %v", err)
	}
	if uid != os.Getuid() {
		t.Errorf("Owner uid = %d, want %d", uid, os.Getuid())
	}
	if gid != os.Getgid() {
		t.Errorf("Owner gid = %d, want %d", gid, os.Getgid())
	}
}

func TestOSFS_OwnerMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	if _, _, err := (OSFS{}).Owner(missing); err == nil {
		t.Error("Owner on missing path should error")
	}
}

// TestOSFS_ChownSelf chowns a file to the process's own uid/gid, which is
// permitted without root, then -1/-1 as a no-op.
func TestOSFS_ChownSelf(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	o := OSFS{}
	if err := o.Chown(file, os.Getuid(), os.Getgid()); err != nil {
		t.Fatalf("Chown to self: %v", err)
	}
	// -1/-1 leaves both fields unchanged and must succeed.
	if err := o.Chown(file, -1, -1); err != nil {
		t.Fatalf("Chown no-op: %v", err)
	}
}

func TestResolveOwnerGroup(t *testing.T) {
	cur, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current: %v", err)
	}
	curUID, _ := strconv.Atoi(cur.Uid)
	curGID, _ := strconv.Atoi(cur.Gid)

	grp, err := user.LookupGroupId(cur.Gid)
	if err != nil {
		t.Skipf("cannot resolve group name for gid %s: %v", cur.Gid, err)
	}

	tests := []struct {
		name      string
		owner     string
		group     string
		wantUID   int
		wantGID   int
		wantError bool
	}{
		{name: "both empty -> -1/-1", owner: "", group: "", wantUID: -1, wantGID: -1},
		{name: "owner by name", owner: cur.Username, group: "", wantUID: curUID, wantGID: -1},
		{name: "group by name", owner: "", group: grp.Name, wantUID: -1, wantGID: curGID},
		{name: "numeric uid", owner: cur.Uid, group: "", wantUID: curUID, wantGID: -1},
		{name: "numeric gid", owner: "", group: cur.Gid, wantUID: -1, wantGID: curGID},
		{name: "name + numeric", owner: cur.Username, group: cur.Gid, wantUID: curUID, wantGID: curGID},
		{name: "unknown user", owner: "definitely-no-such-user-xyz", group: "", wantError: true},
		{name: "unknown group", owner: "", group: "definitely-no-such-group-xyz", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uid, gid, err := resolveOwnerGroup(tt.owner, tt.group)
			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error, got uid=%d gid=%d", uid, gid)
				}
				if uid != -1 || gid != -1 {
					t.Errorf("on error want -1/-1, got %d/%d", uid, gid)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if uid != tt.wantUID || gid != tt.wantGID {
				t.Errorf("got %d/%d, want %d/%d", uid, gid, tt.wantUID, tt.wantGID)
			}
		})
	}
}

// TestApplyOwnership exercises the public entry point: empty owner/group is a
// no-op, a bad name returns the resolve error, and chowning to self succeeds.
func TestApplyOwnership(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	o := OSFS{}
	cur, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current: %v", err)
	}

	if err := ApplyOwnership(o, file, "", ""); err != nil {
		t.Errorf("ApplyOwnership no-op: %v", err)
	}
	if err := ApplyOwnership(o, file, "no-such-user-xyz", ""); err == nil {
		t.Error("ApplyOwnership with unknown user should error")
	}
	if err := ApplyOwnership(o, file, cur.Username, cur.Gid); err != nil {
		t.Errorf("ApplyOwnership to self: %v", err)
	}
}

// TestOwnershipChange covers no-op cases, in-sync detection, an unreadable
// owner (returns nil), a resolve error, and an actual drift Change.
func TestOwnershipChange(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	o := OSFS{}
	cur, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current: %v", err)
	}

	// No owner/group specified -> nil change.
	if ch, err := OwnershipChange(o, file, "", ""); err != nil || ch != nil {
		t.Errorf("empty spec: change=%v err=%v, want nil/nil", ch, err)
	}

	// Matches current owner -> in sync, nil change.
	if ch, err := OwnershipChange(o, file, cur.Username, cur.Gid); err != nil || ch != nil {
		t.Errorf("in-sync: change=%v err=%v, want nil/nil", ch, err)
	}

	// Unknown user -> resolve error propagates.
	if _, err := OwnershipChange(o, file, "no-such-user-xyz", ""); err == nil {
		t.Error("unknown user should produce a resolve error")
	}

	// Drift: request a uid that differs from the current owner via a numeric
	// id guaranteed not to equal it.
	wantUID := os.Getuid() + 1
	ch, err := OwnershipChange(o, file, strconv.Itoa(wantUID), "")
	if err != nil {
		t.Fatalf("drift: %v", err)
	}
	if ch == nil {
		t.Fatal("drift: expected a Change, got nil")
	}
	if ch.Property != "owner" || ch.Action != "modify" {
		t.Errorf("Change = %+v, want Property=owner Action=modify", ch)
	}
}

// ownerErrFS wraps OSFS but makes Owner fail, exercising the "ownership cannot
// be read -> nil change" branch of OwnershipChange.
type ownerErrFS struct{ OSFS }

func (ownerErrFS) Owner(string) (int, int, error) { return 0, 0, os.ErrPermission }

func TestOwnershipChange_OwnerUnreadable(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ch, err := OwnershipChange(ownerErrFS{}, file, "0", "")
	if err != nil || ch != nil {
		t.Errorf("unreadable owner: change=%v err=%v, want nil/nil", ch, err)
	}
}

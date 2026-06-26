package extensions

import "fmt"

// ApplyOwnership sets the owner/group of path when either is specified. It is a
// no-op when both are empty or on platforms without POSIX ownership (Windows).
// Owner/Group may be names or numeric IDs; an unspecified side is left unchanged.
func ApplyOwnership(fsys FS, path, owner, group string) error {
	if (owner == "" && group == "") || !ownershipSupported {
		return nil
	}
	uid, gid, err := resolveOwnerGroup(owner, group)
	if err != nil {
		return err
	}
	return fsys.Chown(path, uid, gid)
}

// OwnershipChange returns a Change describing owner/group drift, or nil if the
// file already matches (or ownership is unspecified/unsupported/unreadable).
// info is the already-stat'd file when available; when nil, fsys.Owner is used.
func OwnershipChange(fsys FS, path, owner, group string) (*Change, error) {
	if (owner == "" && group == "") || !ownershipSupported {
		return nil, nil
	}
	curUID, curGID, err := fsys.Owner(path)
	if err != nil {
		// Ownership cannot be read (e.g. mock FS without ownership); treat as
		// "no opinion" rather than a hard error so Check stays read-only-safe.
		return nil, nil
	}
	wantUID, wantGID, err := resolveOwnerGroup(owner, group)
	if err != nil {
		return nil, err
	}
	if (wantUID < 0 || wantUID == curUID) && (wantGID < 0 || wantGID == curGID) {
		return nil, nil
	}
	return &Change{
		Property: "owner",
		From:     fmt.Sprintf("%d:%d", curUID, curGID),
		To:       fmt.Sprintf("%d:%d", wantUID, wantGID),
		Action:   "modify",
	}, nil
}

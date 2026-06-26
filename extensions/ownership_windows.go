//go:build windows

package extensions

// Windows does not use POSIX uid/gid ownership; Owner/Group are no-ops there,
// as documented for the file and template resources.
const ownershipSupported = false

func resolveOwnerGroup(owner, group string) (uid, gid int, err error) { return -1, -1, nil }

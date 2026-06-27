//go:build !windows

package extensions

import (
	"fmt"
	"os/user"
	"strconv"
)

// ownershipSupported reports whether this platform uses POSIX uid/gid ownership.
const ownershipSupported = true

// resolveOwnerGroup resolves owner/group names (or numeric IDs) to uid/gid.
// An empty owner or group yields -1, meaning "leave unchanged" (os.Chown).
func resolveOwnerGroup(owner, group string) (uid, gid int, err error) {
	uid, gid = -1, -1
	if owner != "" {
		if u, e := user.Lookup(owner); e == nil {
			uid, _ = strconv.Atoi(u.Uid)
		} else if n, e2 := strconv.Atoi(owner); e2 == nil {
			uid = n
		} else {
			return -1, -1, fmt.Errorf("unknown user %q: %w", owner, e)
		}
	}
	if group != "" {
		if g, e := user.LookupGroup(group); e == nil {
			gid, _ = strconv.Atoi(g.Gid)
		} else if n, e2 := strconv.Atoi(group); e2 == nil {
			gid = n
		} else {
			return -1, -1, fmt.Errorf("unknown group %q: %w", group, e)
		}
	}
	return uid, gid, nil
}

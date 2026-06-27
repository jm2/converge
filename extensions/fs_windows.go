//go:build windows

package extensions

import "fmt"

// Owner is unsupported on Windows, which does not use POSIX uid/gid ownership.
// Callers gate on ownershipSupported and never reach this in practice.
func (OSFS) Owner(name string) (uid, gid int, err error) {
	return 0, 0, fmt.Errorf("file ownership is not supported on Windows")
}

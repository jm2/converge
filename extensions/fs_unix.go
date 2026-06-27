//go:build !windows

package extensions

import (
	"fmt"
	"os"
	"syscall"
)

// Owner returns the uid/gid of name from the underlying stat structure.
func (OSFS) Owner(name string) (uid, gid int, err error) {
	info, err := os.Stat(name)
	if err != nil {
		return 0, 0, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("ownership unavailable for %s", name)
	}
	return int(st.Uid), int(st.Gid), nil
}

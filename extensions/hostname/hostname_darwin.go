//go:build darwin

package hostname

import (
	"context"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/TsekNet/converge/extensions"
)

// sysSetHostname is the Darwin syscall number for sethostname(2).
// From XNU bsd/kern/syscalls.master: 88 = sethostname.
// golang.org/x/sys/unix does not expose Sethostname on darwin (only linux,
// aix, solaris), so a raw Syscall with unsafe.Pointer is required here.
const sysSetHostname = 88

// Apply sets the hostname via the sethostname(2) syscall on macOS.
func (h *Hostname) Apply(_ context.Context) (*extensions.Result, error) {
	name := []byte(h.Name)
	_, _, errno := unix.Syscall(
		sysSetHostname,
		uintptr(unsafe.Pointer(&name[0])),
		uintptr(len(name)),
		0,
	)
	if errno != 0 {
		return nil, fmt.Errorf("sethostname(%s): %w", h.Name, errno)
	}

	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "set"}, nil
}

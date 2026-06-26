//go:build darwin

package hostname

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/TsekNet/converge/extensions"
)

// sysSetHostname is the Darwin syscall number for sethostname(2).
// From XNU bsd/kern/syscalls.master: 88 = sethostname.
// golang.org/x/sys/unix does not expose Sethostname on darwin (only linux,
// aix, solaris), so a raw Syscall with unsafe.Pointer is required here.
const sysSetHostname = 88

// Apply sets the hostname on macOS. sethostname(2) updates the live kernel
// hostname for immediate effect, but that value is transient and is reset on
// reboot. To persist across reboots the name must be written to the
// SystemConfiguration preferences, which boot reads to repopulate the kernel
// hostname. There is no pure-Go binding for SCPreferences, so scutil is used
// (the same exec-a-platform-CLI pattern as systemd/launchctl elsewhere).
//
// macOS tracks three related names:
//   - HostName: the value returned by gethostname/os.Hostname (what Check reads)
//   - ComputerName: the user-visible name shown in the UI
//   - LocalHostName: the Bonjour/mDNS name (a single DNS label)
func (h *Hostname) Apply(ctx context.Context) (*extensions.Result, error) {
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

	// LocalHostName must be a single DNS label, so strip any domain suffix
	// from an FQDN (e.g. "web01.example.com" -> "web01").
	local := h.Name
	if i := strings.IndexByte(local, '.'); i >= 0 {
		local = local[:i]
	}

	for _, kv := range []struct{ key, value string }{
		{"HostName", h.Name},
		{"ComputerName", h.Name},
		{"LocalHostName", local},
	} {
		if out, err := exec.CommandContext(ctx, "scutil", "--set", kv.key, kv.value).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("scutil --set %s %q: %s: %w", kv.key, kv.value, strings.TrimSpace(string(out)), err)
		}
	}

	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "set"}, nil
}

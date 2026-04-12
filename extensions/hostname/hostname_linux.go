//go:build linux

package hostname

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/sys/unix"

	"github.com/TsekNet/converge/extensions"
)

// Apply sets the hostname via the sethostname syscall and persists it
// to /etc/hostname for survival across reboots.
func (h *Hostname) Apply(_ context.Context) (*extensions.Result, error) {
	if err := unix.Sethostname([]byte(h.Name)); err != nil {
		return nil, fmt.Errorf("sethostname(%s): %w", h.Name, err)
	}

	if err := os.WriteFile("/etc/hostname", []byte(h.Name+"\n"), 0644); err != nil {
		return nil, fmt.Errorf("write /etc/hostname: %w", err)
	}

	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "set"}, nil
}

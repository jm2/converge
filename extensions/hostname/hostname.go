// Package hostname manages the system hostname. Check reads the current
// hostname via os.Hostname(). Apply sets it via platform-specific syscalls.
package hostname

import (
	"context"
	"fmt"
	"os"

	"github.com/TsekNet/converge/extensions"
)

// Hostname ensures the system hostname matches the desired value.
type Hostname struct {
	Name     string
	Critical bool
	FS       extensions.FS // nil uses the real OS filesystem
}

// Opts holds configurable fields for a Hostname resource.
type Opts struct {
	Critical bool
	FS       extensions.FS // inject a mock for testing
}

// New creates a Hostname resource.
func New(name string, opts Opts) *Hostname {
	return &Hostname{Name: name, Critical: opts.Critical, FS: opts.FS}
}

func (h *Hostname) fsys() extensions.FS { return extensions.RealFS(h.FS) }

func (h *Hostname) ID() string       { return "hostname:" + h.Name }
func (h *Hostname) String() string   { return "Hostname " + h.Name }
func (h *Hostname) IsCritical() bool { return h.Critical }

// alreadySet returns true if the current hostname already matches.
// Also returns an error if the name is empty.
func (h *Hostname) alreadySet() (bool, error) {
	if h.Name == "" {
		return false, fmt.Errorf("hostname: name must not be empty")
	}
	current, err := os.Hostname()
	if err != nil {
		return false, fmt.Errorf("read hostname: %w", err)
	}
	return current == h.Name, nil
}

// Check compares the current hostname to the desired value.
func (h *Hostname) Check(_ context.Context) (*extensions.State, error) {
	current, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("read hostname: %w", err)
	}

	if current == h.Name {
		return &extensions.State{InSync: true}, nil
	}

	return &extensions.State{
		InSync: false,
		Changes: []extensions.Change{{
			Property: "hostname",
			From:     current,
			To:       h.Name,
			Action:   "modify",
		}},
	}, nil
}

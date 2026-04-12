// Package kernelmodule manages Linux kernel module loading and blacklisting.
// Check reads /proc/modules and /etc/modprobe.d/ for current state.
// Apply loads/unloads modules via finit_module/delete_module syscalls
// and persists blacklists to /etc/modprobe.d/.
package kernelmodule

import (
	"fmt"
	"regexp"
)

// StateType represents whether the module should be loaded or blacklisted.
type StateType string

const (
	Loaded      StateType = "loaded"
	Blacklisted StateType = "blacklisted"
)

// KernelModule ensures a kernel module is loaded or blacklisted.
type KernelModule struct {
	Module   string
	State    StateType
	Critical bool
}

// Opts holds configurable fields for a KernelModule resource.
type Opts struct {
	State    StateType
	Critical bool
}

// New creates a KernelModule resource.
func New(module string, opts Opts) *KernelModule {
	return &KernelModule{
		Module:   module,
		State:    opts.State,
		Critical: opts.Critical,
	}
}

var validModuleName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// validate returns an error if the module name contains invalid characters.
func (k *KernelModule) validate() error {
	if !validModuleName.MatchString(k.Module) {
		return fmt.Errorf("invalid module name %q: must match [a-zA-Z0-9_-]+", k.Module)
	}
	return nil
}

func (k *KernelModule) ID() string       { return fmt.Sprintf("kernelmodule:%s", k.Module) }
func (k *KernelModule) String() string   { return fmt.Sprintf("KernelModule %s (%s)", k.Module, k.State) }
func (k *KernelModule) IsCritical() bool { return k.Critical }

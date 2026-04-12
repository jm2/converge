//go:build linux

package kernelmodule

import "os/exec"

// newCommand wraps exec.Command for testability.
var newCommand = func(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

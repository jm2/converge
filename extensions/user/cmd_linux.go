//go:build linux

package user

import (
	"context"
	"os/exec"
)

// newCommand wraps exec.CommandContext for testability.
var newCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

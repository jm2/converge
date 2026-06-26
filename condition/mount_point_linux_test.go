//go:build linux

package condition_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/TsekNet/converge/condition"
)

// TestMountPoint_Wait_AlreadyMet verifies Wait returns immediately when the
// path is already a mount point (no poll(2) wait needed). /proc is a procfs
// mount on essentially every Linux system.
func TestMountPoint_Wait_AlreadyMet(t *testing.T) {
	t.Parallel()

	c := condition.MountPoint("/proc")
	met, err := c.Met(context.Background())
	if err != nil {
		t.Fatalf("Met() error = %v", err)
	}
	if !met {
		t.Skip("/proc is not a distinct mount point in this environment")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	if err := c.Wait(ctx); err != nil {
		t.Errorf("Wait() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 1*time.Second {
		t.Errorf("Wait() took %v, expected immediate return", elapsed)
	}
}

// TestMountPoint_Wait_CtxCancel verifies the poll(2)-based Wait honors ctx
// cancellation promptly for a path that never becomes a mount point.
func TestMountPoint_Wait_CtxCancel(t *testing.T) {
	t.Parallel()

	c := condition.MountPoint(filepath.Join(t.TempDir(), "never-mounted"))

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- c.Wait(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected non-nil error on ctx cancel")
		}
	case <-time.After(2 * time.Second):
		t.Error("Wait did not return after ctx cancel")
	}
}

//go:build linux

package condition_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/TsekNet/converge/condition"
)

// TestNetworkInterface_Wait_AlreadyMet verifies Wait returns immediately when
// the interface is already up (no netlink wait needed). The loopback "lo" is
// up on essentially every Linux system.
func TestNetworkInterface_Wait_AlreadyMet(t *testing.T) {
	t.Parallel()

	c := condition.NetworkInterface("lo")
	met, err := c.Met(context.Background())
	if err != nil {
		t.Fatalf("Met() error = %v", err)
	}
	if !met {
		t.Skip("loopback interface lo is not up in this environment")
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

// TestNetworkInterface_Wait_CtxCancel verifies the netlink-based Wait honors
// ctx cancellation for an interface that never appears. The 500ms recv timeout
// bounds how long each loop blocks, so cancellation is observed promptly.
func TestNetworkInterface_Wait_CtxCancel(t *testing.T) {
	t.Parallel()

	c := condition.NetworkInterface("converge-nope0")
	if met, _ := c.Met(context.Background()); met {
		t.Skip("unexpected interface converge-nope0 exists")
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- c.Wait(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		// A non-nil error is expected: either ctx.Err() or a netlink socket
		// setup failure in a restricted sandbox. Both are acceptable here; we
		// only assert Wait does not falsely report success.
		if err == nil {
			t.Error("expected non-nil error on ctx cancel")
		} else if !errors.Is(err, context.Canceled) {
			t.Logf("Wait returned non-ctx error (likely restricted netlink): %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("Wait did not return after ctx cancel")
	}
}

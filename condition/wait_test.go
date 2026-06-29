package condition

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/TsekNet/converge/internal/shell"
)

// waitErrCondition is a test helper whose Wait returns a fixed error. It lets
// us exercise the error-propagation path of combinators that delegate to a
// sub-condition's Wait (e.g. All).
type waitErrCondition struct {
	err error
}

func (w *waitErrCondition) Met(_ context.Context) (bool, error) { return false, nil }
func (w *waitErrCondition) Wait(_ context.Context) error        { return w.err }
func (w *waitErrCondition) String() string                      { return "waitErr" }

func TestAll_Wait(t *testing.T) {
	t.Run("all sub-waits succeed", func(t *testing.T) {
		// staticCondition.Wait returns nil, so All.Wait should return nil.
		c := All(met("a"), met("b"))
		if err := c.Wait(context.Background()); err != nil {
			t.Errorf("Wait() = %v, want nil", err)
		}
	})

	t.Run("empty returns nil", func(t *testing.T) {
		if err := All().Wait(context.Background()); err != nil {
			t.Errorf("Wait() = %v, want nil", err)
		}
	})

	t.Run("propagates sub-wait error", func(t *testing.T) {
		sentinel := errors.New("boom")
		c := All(met("a"), &waitErrCondition{err: sentinel})
		if err := c.Wait(context.Background()); !errors.Is(err, sentinel) {
			t.Errorf("Wait() = %v, want %v", err, sentinel)
		}
	})
}

func TestAny_Wait(t *testing.T) {
	t.Run("already met returns nil quickly", func(t *testing.T) {
		c := Any(unmet("a"), met("b"))
		start := time.Now()
		if err := c.Wait(context.Background()); err != nil {
			t.Fatalf("Wait() = %v, want nil", err)
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("Wait() took %v, expected immediate return", elapsed)
		}
	})

	t.Run("never met honors ctx cancel", func(t *testing.T) {
		c := Any(unmet("a"), unmet("b"))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		start := time.Now()
		err := c.Wait(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Wait() = %v, want context.Canceled", err)
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("Wait() took %v, expected prompt return on cancel", elapsed)
		}
	})

	t.Run("never met honors deadline", func(t *testing.T) {
		c := Any(unmet("a"))
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if err := c.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Wait() = %v, want context.DeadlineExceeded", err)
		}
	})
}

func TestNot_Wait(t *testing.T) {
	t.Run("inner unmet returns nil quickly", func(t *testing.T) {
		// Not is met when inner is not met, so Wait returns immediately.
		c := Not(unmet("a"))
		start := time.Now()
		if err := c.Wait(context.Background()); err != nil {
			t.Fatalf("Wait() = %v, want nil", err)
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("Wait() took %v, expected immediate return", elapsed)
		}
	})

	t.Run("inner always met honors ctx cancel", func(t *testing.T) {
		c := Not(met("a"))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		start := time.Now()
		err := c.Wait(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Wait() = %v, want context.Canceled", err)
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("Wait() took %v, expected prompt return on cancel", elapsed)
		}
	})
}

func TestResource_Wait(t *testing.T) {
	// resourceCondition.Wait is a no-op (the engine enforces ordering).
	if err := Resource("file:/etc/x").Wait(context.Background()); err != nil {
		t.Errorf("Wait() = %v, want nil", err)
	}
}

func TestShell_Wait(t *testing.T) {
	cmds := platformCmds()

	t.Run("already met returns nil quickly", func(t *testing.T) {
		// A command that exits 0 is immediately met. Use the platform default
		// shell (bash on Unix, PowerShell on Windows) since hardcoding bash
		// hangs on Windows, where /bin/bash is absent and the condition would
		// never become met. A generous safety-net deadline bounds any hang.
		c := Shell(cmds.exitZero).In(shell.Auto)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		start := time.Now()
		if err := c.Wait(ctx); err != nil {
			t.Fatalf("Wait() = %v, want nil", err)
		}
		if elapsed := time.Since(start); elapsed > 3*time.Second {
			t.Errorf("Wait() took %v, expected immediate return", elapsed)
		}
	})

	t.Run("never met honors deadline", func(t *testing.T) {
		// A command that exits 1 is never met; the short deadline must unblock
		// Wait via ctx.Err().
		c := Shell(cmds.exitOne).In(shell.Auto)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		start := time.Now()
		if err := c.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Wait() = %v, want context.DeadlineExceeded", err)
		}
		if elapsed := time.Since(start); elapsed > 3*time.Second {
			t.Errorf("Wait() took %v, expected prompt return on deadline", elapsed)
		}
	})
}

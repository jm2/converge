//go:build linux

package reboot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestBootTime_linux(t *testing.T) {
	t.Parallel()
	// /proc/uptime is readable as a normal user on Linux.
	bt, err := bootTime()
	if err != nil {
		t.Fatalf("bootTime() error: %v", err)
	}
	if bt.IsZero() {
		t.Fatal("bootTime() returned zero time")
	}
	// Boot must be in the past and not absurdly far back (well under 50 years).
	if bt.After(time.Now()) {
		t.Errorf("bootTime() %v is in the future", bt)
	}
	if time.Since(bt) > 50*365*24*time.Hour {
		t.Errorf("bootTime() %v implausibly far in the past", bt)
	}
}

func TestCheck_linux(t *testing.T) {
	t.Parallel()

	t.Run("no sentinel is drifted", func(t *testing.T) {
		t.Parallel()
		r := New("check-none", Opts{})
		r.sentinelOverride = filepath.Join(t.TempDir(), "reboot-check-none.sentinel")

		state, err := r.Check(context.Background())
		if err != nil {
			t.Fatalf("Check() error: %v", err)
		}
		if state.InSync {
			t.Error("expected drifted state with no sentinel")
		}
		if len(state.Changes) != 1 {
			t.Errorf("len(Changes) = %d, want 1", len(state.Changes))
		}
	})

	t.Run("sentinel before boot is compliant", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "reboot-check-old.sentinel")
		// Sentinel timestamp far in the past so the (real) boot time is after it.
		old := time.Now().Add(-100 * 365 * 24 * time.Hour).Unix()
		if err := os.WriteFile(path, []byte(formatUnix(old)), 0o644); err != nil {
			t.Fatal(err)
		}
		r := New("check-old", Opts{})
		r.sentinelOverride = path

		state, err := r.Check(context.Background())
		if err != nil {
			t.Fatalf("Check() error: %v", err)
		}
		if !state.InSync {
			t.Error("expected compliant state when boot is after sentinel")
		}
	})

	t.Run("sentinel in future is drifted", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "reboot-check-future.sentinel")
		future := time.Now().Add(100 * 365 * 24 * time.Hour).Unix()
		if err := os.WriteFile(path, []byte(formatUnix(future)), 0o644); err != nil {
			t.Fatal(err)
		}
		r := New("check-future", Opts{})
		r.sentinelOverride = path

		state, err := r.Check(context.Background())
		if err != nil {
			t.Fatalf("Check() error: %v", err)
		}
		if state.InSync {
			t.Error("expected drifted state when boot precedes sentinel")
		}
	})
}

// TestApply_linux exercises Apply's delay/cancellation path. A cancelled
// context with a non-zero delay forces Apply to return ctx.Err() *before* it
// writes the sentinel or calls unix.Reboot, so no reboot is ever attempted.
func TestApply_linux(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "reboot-apply.sentinel")
	r := New("apply", Opts{Delay: time.Hour})
	r.sentinelOverride = path

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so the delay select takes the ctx.Done() arm

	res, err := r.Apply(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if res != nil {
		t.Errorf("expected nil result, got %+v", res)
	}
	// Apply must bail out before writing the sentinel.
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("sentinel should not exist after cancelled Apply, stat err: %v", statErr)
	}
}

func TestSentinelDir_linux(t *testing.T) {
	t.Parallel()
	if got := sentinelDir(); got != "/var/lib/converge" {
		t.Errorf("sentinelDir() = %q, want %q", got, "/var/lib/converge")
	}
}

func TestSentinelPath_default_linux(t *testing.T) {
	t.Parallel()
	r := New("kpatch", Opts{})
	want := "/var/lib/converge/reboot-kpatch.sentinel"
	if got := r.sentinelPath(); got != want {
		t.Errorf("sentinelPath() = %q, want %q", got, want)
	}
}

func formatUnix(v int64) string {
	return strconv.FormatInt(v, 10)
}

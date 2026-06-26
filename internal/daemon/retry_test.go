package daemon

import (
	"errors"
	"testing"
	"time"

	"github.com/TsekNet/converge/extensions"
)

// TestShouldProcess_PollWakesConvergingAfterBackoff verifies that a resource
// stuck Converging is not suppressed from poll-driven re-detection forever.
// During the active backoff window a poll is suppressed (only a scheduled
// retry runs), but once the window elapses a poll is allowed through. This is
// the backstop that gives a poll-only resource a reliable wakeup even if its
// single scheduled retry event was dropped.
func TestShouldProcess_PollWakesConvergingAfterBackoff(t *testing.T) {
	rm := newRetryManager(5, 20*time.Millisecond)
	id := "package:git"
	rm.register(id)

	// One failure: resource enters Converging with a backoff window.
	delay := rm.recordFailure(id, errors.New("boom"))
	if delay <= 0 {
		t.Fatalf("recordFailure returned %v, want a positive backoff", delay)
	}
	if got := rm.status(id).Compliance; got != Converging {
		t.Fatalf("compliance = %v, want Converging", got)
	}

	// Within the backoff window, a poll must be suppressed.
	if rm.shouldProcess(id, extensions.EventPoll) {
		t.Error("poll processed during backoff window, want suppressed")
	}
	// A scheduled retry is always allowed, even within the window.
	if !rm.shouldProcess(id, extensions.EventRetry) {
		t.Error("retry suppressed during backoff window, want allowed")
	}

	// After the window elapses, a poll must wake the resource.
	time.Sleep(delay + 30*time.Millisecond)
	if !rm.shouldProcess(id, extensions.EventPoll) {
		t.Error("poll suppressed after backoff window, want allowed (reliable wakeup)")
	}
}

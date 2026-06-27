package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/TsekNet/converge/extensions"
)

func TestCoalescer_CollapsesBurstEvents(t *testing.T) {
	out := make(chan resourceEvent, 10)
	c := newCoalescer(100*time.Millisecond, out)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.run(ctx)

	// Send 50 events for the same resource in rapid succession.
	for i := 0; i < 50; i++ {
		c.submit(extensions.Event{ResourceID: "file:/etc/test", Kind: extensions.EventWatch, Detail: "modified"})
	}

	// Wait for the coalesce window to fire.
	time.Sleep(200 * time.Millisecond)
	cancel()

	var count int
	for range out {
		count++
		if len(out) == 0 {
			break
		}
	}

	// All 50 events should collapse into 1.
	if count != 1 {
		t.Errorf("got %d coalesced events, want 1", count)
	}
}

func TestCoalescer_DifferentResourcesNotCoalesced(t *testing.T) {
	out := make(chan resourceEvent, 10)
	c := newCoalescer(50*time.Millisecond, out)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.run(ctx)

	c.submit(extensions.Event{ResourceID: "file:/etc/a", Kind: extensions.EventWatch, Detail: "modified"})
	c.submit(extensions.Event{ResourceID: "file:/etc/b", Kind: extensions.EventWatch, Detail: "modified"})

	time.Sleep(150 * time.Millisecond)
	cancel()

	var ids []string
	for ev := range out {
		ids = append(ids, ev.id)
		if len(out) == 0 {
			break
		}
	}

	if len(ids) != 2 {
		t.Errorf("got %d events, want 2 (one per resource)", len(ids))
	}
}

// TestCoalescer_PreservesStrongestKind verifies that when a watch event and a
// poll event coalesce into one notification, the emitted kind is the stronger
// (watch) signal. This guards against a real watch event being downgraded to a
// poll, which shouldProcess would suppress during a backoff window.
func TestCoalescer_PreservesStrongestKind(t *testing.T) {
	out := make(chan resourceEvent, 10)
	c := newCoalescer(50*time.Millisecond, out)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.run(ctx)

	// Poll arrives first, then a watch event within the same window.
	c.submit(extensions.Event{ResourceID: "file:/etc/test", Kind: extensions.EventPoll})
	c.submit(extensions.Event{ResourceID: "file:/etc/test", Kind: extensions.EventWatch})

	select {
	case ev := <-out:
		if ev.id != "file:/etc/test" {
			t.Fatalf("got id %q, want file:/etc/test", ev.id)
		}
		if ev.kind != extensions.EventWatch {
			t.Errorf("coalesced kind = %v, want EventWatch (strongest signal)", ev.kind)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for coalesced event")
	}
}

func TestRateLimiter_ThrottlesEvents(t *testing.T) {
	// Rate limit: 2 per second, burst 1.
	rl := newResourceRateLimiter(2, 1)
	id := "file:/etc/test"

	allowed := 0
	for i := 0; i < 10; i++ {
		if ok, _ := rl.reserve(id); ok {
			allowed++
		}
	}

	// With burst 1 and a non-blocking reserve, only the burst token is granted
	// immediately; the rest are throttled.
	if allowed != 1 {
		t.Errorf("allowed %d events, want 1 (burst 1, non-blocking)", allowed)
	}
}

// TestRateLimiter_ReserveIsNonBlocking verifies reserve returns immediately
// with a positive delay when throttled, instead of blocking the caller. This
// is what keeps the single consumer loop from stalling on one throttled
// resource.
func TestRateLimiter_ReserveIsNonBlocking(t *testing.T) {
	rl := newResourceRateLimiter(0.1, 1) // 1 per 10s, burst 1
	id := "file:/etc/test"

	if ok, delay := rl.reserve(id); !ok || delay != 0 {
		t.Fatalf("first reserve = (%v, %v), want (true, 0)", ok, delay)
	}

	start := time.Now()
	ok, delay := rl.reserve(id)
	elapsed := time.Since(start)

	if ok {
		t.Error("second reserve returned ok, want throttled")
	}
	if delay <= 0 {
		t.Errorf("throttled delay = %v, want > 0", delay)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("reserve blocked for %v, want non-blocking", elapsed)
	}
}

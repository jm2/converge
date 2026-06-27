package daemon

import (
	"context"
	"sync"
	"time"

	"github.com/TsekNet/converge/extensions"
	"github.com/google/deck"
	"golang.org/x/time/rate"
)

// resourceEvent carries a resource ID together with the authoritative
// EventKind through the internal channels. Keeping the kind alongside the id
// means it is never reconstructed from shared last-write-wins state, so a
// retry can never be mislabeled as a poll (or vice versa).
type resourceEvent struct {
	id   string
	kind extensions.EventKind
}

// kindPriority ranks event kinds so that, when several events for the same
// resource coalesce, the strongest signal wins. External signals
// (condition/watch) outrank an internally detected poll, ensuring a coalesced
// burst that includes a real watch event is not downgraded to a poll (which
// shouldProcess may suppress during backoff).
func kindPriority(k extensions.EventKind) int {
	switch k {
	case extensions.EventCondition:
		return 3
	case extensions.EventWatch:
		return 2
	case extensions.EventPoll:
		return 1
	default:
		return 0
	}
}

// pendingCoalesce tracks a single in-flight coalesce window for a resource.
type pendingCoalesce struct {
	timer *time.Timer
	kind  extensions.EventKind
}

// coalescer collapses multiple rapid events for the same resource into
// a single notification after a configurable window.
type coalescer struct {
	window  time.Duration
	out     chan<- resourceEvent // resource events to process
	in      chan extensions.Event
	pending map[string]*pendingCoalesce
	mu      sync.Mutex
}

func newCoalescer(window time.Duration, out chan<- resourceEvent) *coalescer {
	return &coalescer{
		window:  window,
		out:     out,
		in:      make(chan extensions.Event, 128),
		pending: make(map[string]*pendingCoalesce),
	}
}

// submit queues an event for coalescing.
func (c *coalescer) submit(evt extensions.Event) {
	select {
	case c.in <- evt:
	default:
		deck.Warningf("coalescer: event dropped for %s (channel full)", evt.ResourceID)
	}
}

// run processes incoming events and fires coalesced outputs.
func (c *coalescer) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			c.mu.Lock()
			for _, p := range c.pending {
				p.timer.Stop()
			}
			c.mu.Unlock()
			return
		case evt := <-c.in:
			c.mu.Lock()
			if p, ok := c.pending[evt.ResourceID]; ok {
				// Already pending: coalesce (drop this event) but keep the
				// strongest kind seen during the window.
				if kindPriority(evt.Kind) > kindPriority(p.kind) {
					p.kind = evt.Kind
				}
				c.mu.Unlock()
				continue
			}
			id := evt.ResourceID
			p := &pendingCoalesce{kind: evt.Kind}
			p.timer = time.AfterFunc(c.window, func() {
				c.mu.Lock()
				kind := p.kind
				delete(c.pending, id)
				c.mu.Unlock()
				select {
				case c.out <- resourceEvent{id: id, kind: kind}:
				default:
				}
			})
			c.pending[id] = p
			c.mu.Unlock()
		}
	}
}

// resourceRateLimiter provides per-resource rate limiting.
type resourceRateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rateVal  rate.Limit
	burst    int
}

func newResourceRateLimiter(r float64, burst int) *resourceRateLimiter {
	return &resourceRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rateVal:  rate.Limit(r),
		burst:    burst,
	}
}

// limiterFor returns the per-resource limiter, creating it on first use.
func (rl *resourceRateLimiter) limiterFor(id string) *rate.Limiter {
	rl.mu.RLock()
	l, ok := rl.limiters[id]
	rl.mu.RUnlock()
	if ok {
		return l
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()
	if l, ok = rl.limiters[id]; ok {
		return l
	}
	l = rate.NewLimiter(rl.rateVal, rl.burst)
	rl.limiters[id] = l
	return l
}

// reserve is a non-blocking rate-limit check. It returns (true, 0) and
// consumes a token if the event may be processed now. If throttled it returns
// (false, delay) where delay is how long until a token becomes available, and
// leaves the limiter untouched (via Cancel) so a later re-check is accurate.
//
// Unlike a blocking Wait, reserve never stalls the single consumer loop: one
// heavily throttled resource cannot hold up convergence for every other
// resource. It is only safe to call from a single goroutine per id, because it
// relies on Cancel fully restoring the reservation.
func (rl *resourceRateLimiter) reserve(id string) (bool, time.Duration) {
	l := rl.limiterFor(id)
	r := l.Reserve()
	if !r.OK() {
		// Only happens when the action can never be satisfied (e.g. burst 0).
		// Treat as allowed rather than dropping drift forever.
		return true, 0
	}
	if delay := r.Delay(); delay > 0 {
		r.Cancel()
		return false, delay
	}
	return true, 0
}

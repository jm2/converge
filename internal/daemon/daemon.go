// Package daemon implements the persistent event-driven convergence loop.
// It watches resources for drift via OS-level events (Watcher interface)
// or polling (Poller interface / default interval), and re-converges
// only the affected resources.
package daemon

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TsekNet/converge/extensions"
	"github.com/TsekNet/converge/internal/engine"
	"github.com/TsekNet/converge/internal/graph"
	"github.com/TsekNet/converge/internal/output"
	"github.com/google/deck"
)

const (
	defaultPollInterval    = 30 * time.Second
	defaultMaxRetries      = 3
	defaultRetryBase       = 5 * time.Second
	defaultCoalesceWindow  = 500 * time.Millisecond
	defaultRatePerResource = 0.1 // 1 event per 10 seconds
	defaultRateBurst       = 3
)

// Options controls daemon behavior.
type Options struct {
	Timeout          time.Duration // per-resource timeout
	Parallel         int           // max concurrent resources during initial convergence
	DefaultPollFreq  time.Duration // poll interval for resources without Watcher or Poller
	MaxRetries       int           // max retries before marking noncompliant (0 = use default)
	RetryBaseDelay   time.Duration // base delay for exponential backoff (0 = use default)
	CoalesceWindow   time.Duration // event coalescing window (0 = use default)
	ConvergedTimeout time.Duration // exit after system is stable for this duration (0 = run forever)
}

// Daemon watches resources for drift and re-converges them.
type Daemon struct {
	graph           *graph.Graph
	printer         output.Printer
	opts            Options
	retries         *retryManager
	initErr         error        // error from initial convergence
	processing      sync.Map     // tracks in-progress resource IDs
	conditionsMet   sync.Map     // resourceID -> bool; true once condition is satisfied
	deferredPending sync.Map     // resourceID -> bool; a rate-limited re-enqueue is scheduled
	lastChange      atomic.Int64 // unix nano timestamp of last Apply that changed something
}

// New creates a daemon for the given resource graph.
func New(g *graph.Graph, printer output.Printer, opts Options) *Daemon {
	if opts.DefaultPollFreq == 0 {
		opts.DefaultPollFreq = defaultPollInterval
	}
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = defaultMaxRetries
	}
	if opts.RetryBaseDelay == 0 {
		opts.RetryBaseDelay = defaultRetryBase
	}
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Minute
	}

	rm := newRetryManager(opts.MaxRetries, opts.RetryBaseDelay)
	d := &Daemon{graph: g, printer: printer, opts: opts, retries: rm}

	for _, node := range g.Nodes() {
		id := node.Ext.ID()
		rm.register(id)
		if node.Meta.Retry > 0 {
			rm.setRetryOverride(id, node.Meta.Retry)
		}
		// Pre-populate conditionsMet true for resources with no condition gate.
		// Resources with a condition start as false; startWatchers sets them true.
		if node.Meta.Condition == nil {
			d.conditionsMet.Store(id, true)
		}
	}

	return d
}

// Status returns the current compliance state of a resource.
func (d *Daemon) Status(id string) ResourceStatus {
	return d.retries.status(id)
}

// Run performs initial convergence, then watches all resources until ctx
// is cancelled or --timeout stability window is reached.
func (d *Daemon) Run(ctx context.Context) error {
	// Phase 1: initial convergence pass.
	// In daemon mode (no timeout), suppress the summary since the daemon keeps running.
	engineOpts := engine.Options{
		Timeout:         d.opts.Timeout,
		Parallel:        d.opts.Parallel,
		SuppressSummary: d.opts.ConvergedTimeout == 0,
	}
	code, err := engine.RunApplyDAG(ctx, d.graph, d.printer, engineOpts)
	if err != nil {
		deck.Errorf("initial convergence failed (exit %d): %v", code, err)
		d.initErr = err
	}

	if d.opts.ConvergedTimeout == 0 {
		fmt.Printf("%s────────────────────────────────────────────%s\n", "\033[2m", "\033[0m")
		fmt.Printf("%s%s● WATCHING%s  %sdrift detection active%s\n\n",
			"\033[1m", "\033[36m", "\033[0m", "\033[2m", "\033[0m")
	}

	// Phase 2: start watchers/pollers feeding raw events.
	rawEvents := make(chan extensions.Event, 256)
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()

	wg := d.startWatchers(watchCtx, rawEvents)

	// Phase 3: split events into coalesced (watch/poll) and direct (retry).
	// Each channel carries the authoritative EventKind alongside the id, so the
	// kind is never reconstructed from shared last-write-wins state.
	coalescedEvents := make(chan resourceEvent, 256)
	retryEvents := make(chan resourceEvent, 64)
	window := d.opts.CoalesceWindow
	if window == 0 {
		window = defaultCoalesceWindow
	}
	coalescer := newCoalescer(window, coalescedEvents)
	go coalescer.run(watchCtx)

	// Bridge: raw events -> coalescer or direct retry channel. Retries use a
	// blocking (ctx-guarded) send so a scheduled retry is never silently
	// dropped; this is the reliable wakeup for resources stuck Converging.
	go func() {
		for {
			select {
			case <-watchCtx.Done():
				return
			case evt := <-rawEvents:
				if evt.Kind == extensions.EventRetry {
					select {
					case retryEvents <- resourceEvent{id: evt.ResourceID, kind: evt.Kind}:
					case <-watchCtx.Done():
						return
					}
				} else {
					coalescer.submit(evt)
				}
			}
		}
	}()

	// Converged timeout: exit after system is stable for the specified duration.
	loopCtx := ctx
	var convergedCancel context.CancelFunc
	if d.opts.ConvergedTimeout > 0 {
		loopCtx, convergedCancel = context.WithCancel(ctx)
		d.lastChange.Store(time.Now().UnixNano())
		go d.watchConvergence(loopCtx, convergedCancel)
	}

	// Phase 4: rate-limited event loop reads from both channels.
	rl := newResourceRateLimiter(defaultRatePerResource, defaultRateBurst)
	d.processLoop(loopCtx, coalescedEvents, retryEvents, rl, rawEvents)

	if convergedCancel != nil {
		convergedCancel()
	}
	watchCancel()
	wg.Wait()
	return d.initErr
}

// startWatchers launches a goroutine per resource for Watch or poll.
// Resources with a Condition gate get an additional goroutine that waits for
// the condition to be met before the resource is eligible for convergence.
func (d *Daemon) startWatchers(ctx context.Context, eventCh chan extensions.Event) *sync.WaitGroup {
	var wg sync.WaitGroup

	for _, node := range d.graph.Nodes() {
		ext := node.Ext
		cond := node.Meta.Condition

		// Condition watcher: blocks until condition is met, then marks the
		// resource eligible and injects an EventCondition to trigger convergence.
		if cond != nil {
			if met, _ := cond.Met(ctx); met {
				d.conditionsMet.Store(ext.ID(), true)
			} else {
				wg.Add(1)
				go func(ext extensions.Extension, cond extensions.Condition) {
					defer wg.Done()
					deck.Infof("waiting for condition: %s (%s)", ext.ID(), cond)
					if err := cond.Wait(ctx); err != nil {
						// ctx cancelled or fatal error; do not mark met.
						return
					}
					d.conditionsMet.Store(ext.ID(), true)
					deck.Infof("condition met: %s (%s)", ext.ID(), cond)
					// Blocking send: a dropped condition-met event means the resource
					// never converges. ctx guards against deadlock on shutdown.
					select {
					case eventCh <- extensions.Event{
						ResourceID: ext.ID(),
						Kind:       extensions.EventCondition,
						Detail:     "condition met: " + cond.String(),
						Time:       time.Now(),
					}:
					case <-ctx.Done():
					}
				}(ext, cond)
			}
		}

		wg.Add(1)

		if w, ok := ext.(extensions.Watcher); ok {
			go func(w extensions.Watcher, ext extensions.Extension) {
				defer wg.Done()
				backoff := time.Second
				maxBackoff := 5 * time.Minute
				for {
					err := w.Watch(ctx, eventCh)
					if ctx.Err() != nil {
						return
					}
					if err == nil {
						return
					}
					deck.Warningf("watcher %s failed, restarting in %v: %v", ext.ID(), backoff, err)
					select {
					case <-time.After(backoff):
						backoff *= 2
						if backoff > maxBackoff {
							backoff = maxBackoff
						}
					case <-ctx.Done():
						return
					}
				}
			}(w, ext)
		} else {
			interval := d.opts.DefaultPollFreq
			if p, ok := ext.(extensions.Poller); ok {
				interval = p.PollInterval()
			}
			go func(ext extensions.Extension, interval time.Duration) {
				defer wg.Done()
				d.poll(ctx, ext, interval, eventCh)
			}(ext, interval)
		}
	}

	return &wg
}

// poll periodically checks a resource and sends an event if it drifts.
func (d *Daemon) poll(ctx context.Context, ext extensions.Extension, interval time.Duration, events chan<- extensions.Event) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if d.retries.isNoncompliant(ext.ID()) {
				continue
			}

			checkCtx, cancel := context.WithTimeout(ctx, d.opts.Timeout)
			state, err := ext.Check(checkCtx)
			cancel()

			if err != nil {
				deck.Warningf("poll check %s: %v", ext.ID(), err)
				continue
			}
			if state != nil && !state.InSync {
				select {
				case events <- extensions.Event{
					ResourceID: ext.ID(),
					Kind:       extensions.EventPoll,
					Detail:     "poll detected drift",
					Time:       time.Now(),
				}:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// processLoop reads coalesced and retry resource events, applies rate
// limiting, and converges each resource in its own goroutine. The loop itself
// never blocks on rate limiting: throttled events are re-enqueued onto the
// internal deferred channel by timer goroutines, so one heavily throttled
// resource cannot stall convergence for every other resource.
func (d *Daemon) processLoop(ctx context.Context, coalescedEvents, retryEvents <-chan resourceEvent, rl *resourceRateLimiter, rawEvents chan extensions.Event) {
	// deferred carries rate-limited events re-enqueued after their limiter
	// delay. Timer goroutines block (ctx-guarded) on sending here, so a
	// throttled drift event is deferred rather than dropped.
	deferred := make(chan resourceEvent, 256)
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-coalescedEvents:
			d.handleResourceEvent(ctx, ev, rl, rawEvents, deferred)
		case ev := <-retryEvents:
			d.handleResourceEvent(ctx, ev, rl, rawEvents, deferred)
		case ev := <-deferred:
			// Clear the dedup marker before handling so a fresh throttle can
			// schedule another re-enqueue if needed.
			d.deferredPending.Delete(ev.id)
			d.handleResourceEvent(ctx, ev, rl, rawEvents, deferred)
		}
	}
}

func (d *Daemon) handleResourceEvent(ctx context.Context, ev resourceEvent, rl *resourceRateLimiter, rawEvents chan extensions.Event, deferred chan<- resourceEvent) {
	id, kind := ev.id, ev.kind

	node := d.graph.Node(id)
	if node == nil {
		return
	}

	// Gate: skip if the resource's condition has not yet been satisfied.
	if met, ok := d.conditionsMet.Load(id); !ok || !met.(bool) {
		return
	}

	if !d.retries.shouldProcess(id, kind) {
		return
	}

	// If a convergence for this resource is already in flight, it will pick up
	// the current drift, so drop this event. Only this single loop goroutine
	// stores into processing, so a Load peek here is race-free against the
	// LoadOrStore below.
	if _, busy := d.processing.Load(id); busy {
		return
	}

	// Rate limit watch/poll events, not retries (retries have their own
	// backoff). Never block the loop: if throttled, re-enqueue the event after
	// the limiter's delay so the drift is processed later instead of dropped.
	if kind != extensions.EventRetry {
		ok, delay := rl.reserve(id)
		if !ok {
			d.deferEvent(ctx, ev, delay, deferred)
			return
		}
	}

	if _, loaded := d.processing.LoadOrStore(id, true); loaded {
		return
	}

	deck.Infof("drift detected: %s (%s)", id, kind)
	go func(ext extensions.Extension, id string) {
		defer d.processing.Delete(id)
		d.convergeResource(ctx, ext, id, rawEvents)
	}(node.Ext, id)
}

// deferEvent re-enqueues a rate-limited event after delay. A single re-enqueue
// is kept pending per resource (deferredPending) so a steadily-throttled
// resource cannot spawn an unbounded number of timer goroutines, while still
// guaranteeing its drift is eventually processed rather than dropped. The
// timer goroutine performs a blocking, ctx-guarded send, so the consumer loop
// stays responsive and shutdown is clean.
func (d *Daemon) deferEvent(ctx context.Context, ev resourceEvent, delay time.Duration, deferred chan<- resourceEvent) {
	if _, pending := d.deferredPending.LoadOrStore(ev.id, true); pending {
		return // a re-enqueue is already scheduled for this resource
	}
	if delay <= 0 {
		delay = time.Millisecond
	}
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
			select {
			case deferred <- ev:
			case <-ctx.Done():
				d.deferredPending.Delete(ev.id)
			}
		case <-ctx.Done():
			d.deferredPending.Delete(ev.id)
		}
	}()
}

// convergeResource runs Check/Apply with retry/backoff logic.
func (d *Daemon) convergeResource(ctx context.Context, ext extensions.Extension, id string, events chan<- extensions.Event) {
	applyCtx, cancel := context.WithTimeout(ctx, d.opts.Timeout)
	defer cancel()

	state, err := ext.Check(applyCtx)
	if err != nil {
		d.scheduleRetry(ctx, id, err, events)
		return
	}
	if state == nil || state.InSync {
		d.retries.reset(id)
		return
	}

	result, err := ext.Apply(applyCtx)
	if err != nil {
		d.scheduleRetry(ctx, id, err, events)
		return
	}

	if result != nil {
		if result.Changed {
			d.lastChange.Store(time.Now().UnixNano())
		}
		d.printer.ApplyResult(ext, result)
	}
	d.retries.reset(id)

	// Schedule a re-check for dependent resources so they converge
	// after their dependency has been successfully applied.
	for _, childID := range d.graph.Children(id) {
		select {
		case events <- extensions.Event{
			ResourceID: childID,
			Kind:       extensions.EventWatch,
			Detail:     "dependency converged: " + id,
			Time:       time.Now(),
		}:
		default:
		}
	}
}

// watchConvergence monitors the last change timestamp and cancels the context
// once the system has been stable for ConvergedTimeout.
func (d *Daemon) watchConvergence(ctx context.Context, cancel context.CancelFunc) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			last := time.Unix(0, d.lastChange.Load())
			if time.Since(last) >= d.opts.ConvergedTimeout {
				deck.Infof("system stable for %v, shutting down", d.opts.ConvergedTimeout)
				cancel()
				return
			}
		}
	}
}

// scheduleRetry records a failure and schedules a retry event after backoff.
func (d *Daemon) scheduleRetry(ctx context.Context, id string, err error, events chan<- extensions.Event) {
	delay := d.retries.recordFailure(id, err)
	if delay == 0 {
		return // noncompliant, no more retries
	}

	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
			// Blocking (ctx-guarded) send: a dropped retry can leave a
			// resource stuck Converging with no further wakeup, so deliver it
			// rather than dropping on a full channel.
			select {
			case events <- extensions.Event{
				ResourceID: id,
				Kind:       extensions.EventRetry,
				Detail:     "scheduled retry",
				Time:       time.Now(),
			}:
			case <-ctx.Done():
			}
		case <-ctx.Done():
			return
		}
	}()
}

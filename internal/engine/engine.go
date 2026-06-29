package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TsekNet/converge/extensions"
	"github.com/TsekNet/converge/internal/exit"
	"github.com/TsekNet/converge/internal/graph"
	"github.com/TsekNet/converge/internal/output"
	"github.com/google/deck"
)

// Options controls engine execution behaviour.
type Options struct {
	Timeout         time.Duration // per-resource timeout (0 = no timeout)
	Parallel        int           // max concurrent resources (<=1 = sequential)
	SuppressSummary bool          // skip the summary line (daemon mode prints its own)
}

func DefaultOptions() Options {
	return Options{Timeout: 5 * time.Minute, Parallel: 1}
}

func withTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d > 0 {
		return context.WithTimeout(parent, d)
	}
	return parent, func() {}
}

func isCritical(r extensions.Extension) bool {
	cr, ok := r.(extensions.CriticalResource)
	return ok && cr.IsCritical()
}

// RunPlan checks all resources without applying changes.
func RunPlan(resources []extensions.Extension, printer output.Printer, opts Options) (int, error) {
	ctx := context.Background()
	pending, ok := 0, 0

	for i, r := range resources {
		printer.ResourceChecking(r, i+1, len(resources))

		rctx, cancel := withTimeout(ctx, opts.Timeout)
		state, err := r.Check(rctx)
		cancel()

		if err != nil {
			printer.Error(r, err)
			return exit.Error, fmt.Errorf("check failed for %s: %w", r.ID(), err)
		}
		if state == nil {
			state = &extensions.State{}
		}

		printer.PlanResult(r, state)
		if state.InSync {
			ok++
		} else {
			pending++
		}
	}

	printer.PlanSummary(pending, ok, len(resources))
	if pending > 0 {
		return exit.Pending, nil
	}
	return exit.OK, nil
}

type applyResult struct {
	ext    extensions.Extension
	result *extensions.Result
}

func (ar applyResult) failed() bool {
	return ar.result.Status == extensions.StatusFailed
}

func applyOne(ctx context.Context, r extensions.Extension, timeout time.Duration, noop bool) applyResult {
	start := time.Now()

	rctx, cancel := withTimeout(ctx, timeout)
	state, err := r.Check(rctx)
	cancel()

	if err != nil {
		return applyResult{r, &extensions.Result{
			Status: extensions.StatusFailed, Err: err, Duration: time.Since(start),
		}}
	}
	if state == nil {
		state = &extensions.State{}
	}
	if state.InSync {
		return applyResult{r, &extensions.Result{Status: extensions.StatusOK}}
	}

	// Per-resource noop: report drift but skip Apply.
	if noop {
		return applyResult{r, &extensions.Result{
			Status:   extensions.StatusOK,
			Message:  "noop: drift detected, apply skipped",
			Duration: time.Since(start),
		}}
	}

	changes := state.Changes

	rctx, cancel = withTimeout(ctx, timeout)
	result, err := r.Apply(rctx)
	cancel()

	if err != nil {
		return applyResult{r, &extensions.Result{
			Status: extensions.StatusFailed, Err: err, Duration: time.Since(start), Changes: changes,
		}}
	}
	if result == nil {
		return applyResult{r, &extensions.Result{
			Status: extensions.StatusFailed, Err: fmt.Errorf("Apply returned nil"), Duration: time.Since(start),
		}}
	}
	result.Duration = time.Since(start)
	result.Changes = changes

	// Verify convergence: re-Check after a successful Apply. A resource that
	// reports success but is still out of sync did not actually converge, so
	// report it as a failure rather than masking the drift as success.
	//
	// Resources that intentionally always apply (e.g. a guardless Exec that runs
	// every convergence) never report in-sync on their own, so skip the re-Check
	// for them — "still drifted" is their contract, not a failure.
	if aa, ok := r.(extensions.AlwaysApplies); ok && aa.AlwaysApplies() {
		return applyResult{r, result}
	}
	rctx, cancel = withTimeout(ctx, timeout)
	newState, checkErr := r.Check(rctx)
	cancel()
	if checkErr == nil && newState != nil && !newState.InSync {
		return applyResult{r, &extensions.Result{
			Status:   extensions.StatusFailed,
			Err:      fmt.Errorf("%s still out of sync after apply", r.ID()),
			Duration: time.Since(start),
			Changes:  changes,
		}}
	}

	return applyResult{r, result}
}

// gateMet reports whether a resource's condition gate currently allows it to
// run. A nil condition is always met. Errors are treated as not-met, so a
// resource is skipped rather than applied against an unknown precondition.
func gateMet(ctx context.Context, cond extensions.Condition, timeout time.Duration) bool {
	if cond == nil {
		return true
	}
	cctx, cancel := withTimeout(ctx, timeout)
	defer cancel()
	met, err := cond.Met(cctx)
	return err == nil && met
}

// applyNode gates a resource on its condition, then applies it. A gated resource
// whose condition is not yet met is reported as a skipped no-op (StatusOK), not
// a failure, mirroring the documented "skip until met" semantics (the daemon
// converges it later via its EventCondition path).
func applyNode(ctx context.Context, r extensions.Extension, timeout time.Duration, noop bool, cond extensions.Condition) applyResult {
	if !gateMet(ctx, cond, timeout) {
		return applyResult{r, &extensions.Result{Status: extensions.StatusOK, Message: "skipped: condition not met"}}
	}
	return applyOne(ctx, r, timeout, noop)
}

type nameAware interface {
	SetMaxNameLen(int)
}

func setMaxNameLen(resources []extensions.Extension, printer output.Printer) {
	if p, ok := printer.(nameAware); ok {
		maxLen := 0
		for _, r := range resources {
			if l := len(r.String()); l > maxLen {
				maxLen = l
			}
		}
		p.SetMaxNameLen(maxLen)
	}
}

// RunPlanDAG checks all resources in topological order without applying changes.
func RunPlanDAG(g *graph.Graph, printer output.Printer, opts Options) (int, error) {
	all, err := g.Flatten()
	if err != nil {
		return exit.Error, fmt.Errorf("building execution order: %w", err)
	}
	return RunPlan(all, printer, opts)
}

// RunApplyDAG checks and applies changes in topological layer order.
// Resources within the same layer run concurrently up to opts.Parallel.
// Dependencies in earlier layers complete before later layers start.
// Package resources in the same layer with the same manager and state
// are auto-grouped into a single batch install/remove invocation.
func RunApplyDAG(ctx context.Context, g *graph.Graph, printer output.Printer, opts Options) (int, error) {
	nodeLayers, err := g.TopologicalNodeLayers()
	if err != nil {
		return exit.Error, fmt.Errorf("building execution order: %w", err)
	}

	all, _ := g.Flatten()

	// Build a noop lookup from node meta.
	noopSet := make(map[string]bool)
	// Build a condition-gate lookup. Gated resources are skipped until their
	// precondition holds; applying them unconditionally during the initial pass
	// would contradict the documented "skip until met" gate.
	gated := make(map[string]extensions.Condition)
	for _, node := range g.Nodes() {
		if node.Meta.Noop {
			noopSet[node.Ext.ID()] = true
		}
		if node.Meta.Condition != nil {
			gated[node.Ext.ID()] = node.Meta.Condition
		}
	}

	start := time.Now()
	changed, ok, failed := 0, 0, 0
	idx := 0

	setMaxNameLen(all, printer)

	for _, nodeLayer := range nodeLayers {
		layer := autoGroupLayer(nodeLayer)
		results := make([]applyResult, len(layer))

		if opts.Parallel > 1 && len(layer) > 1 {
			// Parallel: run all, print after layer completes.
			sem := make(chan struct{}, opts.Parallel)
			var wg sync.WaitGroup
			for i, r := range layer {
				wg.Add(1)
				sem <- struct{}{}
				go func(j int, res extensions.Extension) {
					defer wg.Done()
					defer func() { <-sem }()
					results[j] = applyNode(ctx, res, opts.Timeout, noopSet[res.ID()], gated[res.ID()])
				}(i, r)
			}
			wg.Wait()
			for _, ar := range results {
				idx++
				printer.ApplyStart(ar.ext, idx, len(all))
				printer.ApplyResult(ar.ext, ar.result)
			}
		} else {
			// Sequential: stream output as each resource completes.
			for i, r := range layer {
				idx++
				printer.ApplyStart(r, idx, len(all))
				results[i] = applyNode(ctx, r, opts.Timeout, noopSet[r.ID()], gated[r.ID()])
				printer.ApplyResult(r, results[i].result)
			}
		}

		for _, ar := range results {
			switch ar.result.Status {
			case extensions.StatusOK:
				ok++
			case extensions.StatusChanged:
				changed++
			default:
				failed++
				if isCritical(ar.ext) {
					deck.Errorf("critical resource failed: %s", ar.ext.ID())
					if !opts.SuppressSummary {
						printer.Summary(changed, ok, failed, changed+ok+failed, time.Since(start).Milliseconds())
					}
					return exit.PartialFail, fmt.Errorf("critical resource %s failed", ar.ext.ID())
				}
			}
		}
	}

	total := changed + ok + failed
	if !opts.SuppressSummary {
		printer.Summary(changed, ok, failed, total, time.Since(start).Milliseconds())
	}

	switch {
	case total == 0:
		return exit.OK, nil
	case failed == total:
		return exit.AllFailed, nil
	case failed > 0:
		return exit.PartialFail, nil
	case changed > 0:
		return exit.Changed, nil
	default:
		return exit.OK, nil
	}
}

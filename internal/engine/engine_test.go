package engine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/TsekNet/converge/extensions"
	"github.com/TsekNet/converge/internal/graph"
	"github.com/TsekNet/converge/internal/output"
)

type mockExtension struct {
	id          string
	name        string
	inSync      bool
	applyErr    error
	checkErr    error
	applyCalled bool
}

func (m *mockExtension) ID() string     { return m.id }
func (m *mockExtension) String() string { return m.name }

func (m *mockExtension) Check(_ context.Context) (*extensions.State, error) {
	if m.checkErr != nil {
		return nil, m.checkErr
	}
	return &extensions.State{InSync: m.inSync}, nil
}

func (m *mockExtension) Apply(_ context.Context) (*extensions.Result, error) {
	m.applyCalled = true
	if m.applyErr != nil {
		return nil, m.applyErr
	}
	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "applied"}, nil
}

// mockCondition is a fixed-result gate for testing condition handling.
type mockCondition struct{ met bool }

func (c mockCondition) Met(_ context.Context) (bool, error) { return c.met, nil }
func (c mockCondition) Wait(_ context.Context) error        { return nil }
func (c mockCondition) String() string                      { return "mock" }

type criticalMock struct {
	mockExtension
	critical bool
}

func (c *criticalMock) IsCritical() bool { return c.critical }

type discardPrinter struct{}

func (d *discardPrinter) Banner(_ string)                                          {}
func (d *discardPrinter) BlueprintHeader(_ string)                                 {}
func (d *discardPrinter) ResourceChecking(_ extensions.Extension, _, _ int)        {}
func (d *discardPrinter) PlanResult(_ extensions.Extension, _ *extensions.State)   {}
func (d *discardPrinter) ApplyStart(_ extensions.Extension, _, _ int)              {}
func (d *discardPrinter) ApplyResult(_ extensions.Extension, _ *extensions.Result) {}
func (d *discardPrinter) Summary(_, _, _, _ int, _ int64)                          {}
func (d *discardPrinter) PlanSummary(_, _, _ int)                                  {}
func (d *discardPrinter) Error(_ extensions.Extension, _ error)                    {}

var _ output.Printer = (*discardPrinter)(nil)

func makeGraph(exts ...extensions.Extension) *graph.Graph {
	g := graph.New()
	for _, e := range exts {
		g.AddNode(e)
	}
	return g
}

func TestRunPlanDAG(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		exts     []extensions.Extension
		wantCode int
		wantErr  bool
	}{
		{"all converged", []extensions.Extension{
			&mockExtension{id: "file:/a", name: "File /a", inSync: true},
		}, 0, false},
		{"pending changes", []extensions.Extension{
			&mockExtension{id: "file:/a", name: "File /a", inSync: false},
		}, 5, false},
		{"check error", []extensions.Extension{
			&mockExtension{id: "file:/a", name: "File /a", checkErr: fmt.Errorf("denied")},
		}, 1, true},
		{"empty", nil, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := makeGraph(tt.exts...)
			code, err := RunPlanDAG(g, &discardPrinter{}, DefaultOptions())
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
		})
	}
}

func TestRunApplyDAG(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		exts     []extensions.Extension
		wantCode int
	}{
		{"all converged", []extensions.Extension{
			&mockExtension{id: "file:/a", name: "File /a", inSync: true},
		}, 0},
		{"changes applied", []extensions.Extension{
			&mockExtension{id: "file:/a", name: "File /a", inSync: false},
		}, 2},
		{"partial failure", []extensions.Extension{
			&mockExtension{id: "file:/a", name: "File /a", inSync: false},
			&mockExtension{id: "pkg:git", name: "Package git", inSync: false, applyErr: fmt.Errorf("fail")},
		}, 3},
		{"all failed", []extensions.Extension{
			&mockExtension{id: "file:/a", name: "File /a", inSync: false, applyErr: fmt.Errorf("fail")},
		}, 4},
		{"check error", []extensions.Extension{
			&mockExtension{id: "file:/a", name: "File /a", checkErr: fmt.Errorf("denied")},
		}, 4},
		{"empty", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := makeGraph(tt.exts...)
			code, err := RunApplyDAG(context.Background(), g, &discardPrinter{}, DefaultOptions())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
		})
	}
}

// TestRunApplyDAG_ConditionGate verifies the engine honors Meta.Condition during
// apply: a drifted resource whose gate is unmet is skipped (not applied), while
// one whose gate is met is applied.
func TestRunApplyDAG_ConditionGate(t *testing.T) {
	t.Parallel()

	// Unmet gate: the drifted resource must be skipped, not applied.
	unmet := &mockExtension{id: "file:/gated", name: "File /gated", inSync: false}
	g := graph.New()
	if err := g.AddNode(unmet); err != nil {
		t.Fatal(err)
	}
	g.SetMeta(unmet.ID(), graph.NodeMeta{Condition: mockCondition{met: false}})
	code, err := RunApplyDAG(context.Background(), g, &discardPrinter{}, DefaultOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if unmet.applyCalled {
		t.Error("resource with an unmet condition gate must not be applied")
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0 (gated resource skipped, nothing changed)", code)
	}

	// Met gate: the drifted resource is applied.
	met := &mockExtension{id: "file:/active", name: "File /active", inSync: false}
	g2 := graph.New()
	if err := g2.AddNode(met); err != nil {
		t.Fatal(err)
	}
	g2.SetMeta(met.ID(), graph.NodeMeta{Condition: mockCondition{met: true}})
	code, err = RunApplyDAG(context.Background(), g2, &discardPrinter{}, DefaultOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !met.applyCalled {
		t.Error("resource with a met condition gate must be applied")
	}
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (changed)", code)
	}
}

func TestRunApplyDAG_Parallel(t *testing.T) {
	t.Parallel()

	opts := DefaultOptions()
	opts.Parallel = 2

	g := makeGraph(
		&mockExtension{id: "file:/a", name: "File /a", inSync: false},
		&mockExtension{id: "file:/b", name: "File /b", inSync: true},
		&mockExtension{id: "pkg:git", name: "Package git", inSync: false},
	)
	code, err := RunApplyDAG(context.Background(), g, &discardPrinter{}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestRunApplyDAG_CriticalFailure(t *testing.T) {
	t.Parallel()

	g := makeGraph(
		&criticalMock{
			mockExtension: mockExtension{id: "file:/a", name: "File /a", inSync: false, applyErr: fmt.Errorf("fail")},
			critical:      true,
		},
		&mockExtension{id: "file:/b", name: "File /b", inSync: false},
	)
	code, err := RunApplyDAG(context.Background(), g, &discardPrinter{}, DefaultOptions())
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
	if err == nil {
		t.Error("expected error for critical failure")
	}
}

func TestRunApplyDAG_WithDependencies(t *testing.T) {
	t.Parallel()

	g := graph.New()
	pkg := &mockExtension{id: "package:nginx", name: "Package nginx", inSync: false}
	svc := &mockExtension{id: "service:nginx", name: "Service nginx", inSync: false}
	g.AddNode(pkg)
	g.AddNode(svc)
	g.AddEdge("service:nginx", "package:nginx")

	code, err := RunApplyDAG(context.Background(), g, &discardPrinter{}, DefaultOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (changed)", code)
	}
}

type trackingPrinter struct {
	discardPrinter
	maxNameLen int
}

func (tp *trackingPrinter) SetMaxNameLen(n int) { tp.maxNameLen = n }

var _ nameAware = (*trackingPrinter)(nil)

func TestSetMaxNameLen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		exts    []extensions.Extension
		wantLen int
	}{
		{"single resource", []extensions.Extension{
			&mockExtension{id: "file:/a", name: "File /a"},
		}, len("File /a")},
		{"picks longest", []extensions.Extension{
			&mockExtension{id: "file:/a", name: "File /a"},
			&mockExtension{id: "pkg:nginx", name: "Package nginx"},
		}, len("Package nginx")},
		{"empty list", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tp := &trackingPrinter{}
			setMaxNameLen(tt.exts, tp)
			if tp.maxNameLen != tt.wantLen {
				t.Errorf("maxNameLen = %d, want %d", tp.maxNameLen, tt.wantLen)
			}
		})
	}
}

func TestSetMaxNameLen_NonAware(t *testing.T) {
	t.Parallel()

	// discardPrinter does not implement nameAware: setMaxNameLen should be a no-op.
	dp := &discardPrinter{}
	setMaxNameLen([]extensions.Extension{
		&mockExtension{id: "file:/a", name: "File /a"},
	}, dp)
	// No panic, no error: the function silently skips non-nameAware printers.
}

func TestWithTimeout_Zero(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	got, cancel := withTimeout(ctx, 0)
	defer cancel()

	if _, ok := got.Deadline(); ok {
		t.Error("withTimeout(0) should return a context with no deadline")
	}
}

func TestWithTimeout_NonZero(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	got, cancel := withTimeout(ctx, time.Second)
	defer cancel()

	deadline, ok := got.Deadline()
	if !ok {
		t.Fatal("withTimeout(1s) should return a context with a deadline")
	}
	if time.Until(deadline) <= 0 || time.Until(deadline) > time.Second {
		t.Errorf("deadline should be ~1s from now, got %v", time.Until(deadline))
	}
}

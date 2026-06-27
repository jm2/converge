package pkg

import (
	"context"
	"fmt"
	"os/exec"
	"testing"

	"github.com/TsekNet/converge/internal/testutil"
)

// withMockSnap swaps the snapCommand seam for a MockCmd-backed implementation
// for the duration of the test, returning the mock for assertions.
func withMockSnap(t *testing.T) *testutil.MockCmd {
	t.Helper()
	mock := testutil.NewMockCmd()
	old := snapCommand
	snapCommand = func(_ context.Context, name string, args ...string) *exec.Cmd {
		return mock.Command(name, args...)
	}
	t.Cleanup(func() { snapCommand = old })
	return mock
}

func TestSnapManager_IsInstalled(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		runErr    error
		wantFound bool
	}{
		{"installed", nil, true},
		{"not installed", fmt.Errorf("exit"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := withMockSnap(t)
			mock.SetOutput("snap", "", tt.runErr)
			mgr := &snapManager{}

			found, err := mgr.IsInstalled(ctx, "hello-world")
			if err != nil {
				t.Fatalf("IsInstalled() error = %v", err)
			}
			if found != tt.wantFound {
				t.Errorf("IsInstalled() = %v, want %v", found, tt.wantFound)
			}
			if !mock.Called("snap") {
				t.Error("expected snap to be invoked")
			}
		})
	}
}

func TestSnapManager_Install(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		runErr  error
		wantErr bool
	}{
		{"success", nil, false},
		{"failure", fmt.Errorf("exit"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := withMockSnap(t)
			mock.SetOutput("snap", "", tt.runErr)
			mgr := &snapManager{}

			err := mgr.Install(ctx, "hello-world")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Install() error = %v, wantErr %v", err, tt.wantErr)
			}
			calls := mock.CallsFor("snap")
			if len(calls) != 1 {
				t.Fatalf("want 1 snap call, got %d", len(calls))
			}
			if calls[0].Args[0] != "install" {
				t.Errorf("args = %v, want install subcommand", calls[0].Args)
			}
		})
	}
}

func TestSnapManager_Remove(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		runErr  error
		wantErr bool
	}{
		{"success", nil, false},
		{"failure", fmt.Errorf("exit"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := withMockSnap(t)
			mock.SetOutput("snap", "", tt.runErr)
			mgr := &snapManager{}

			err := mgr.Remove(ctx, "hello-world")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Remove() error = %v, wantErr %v", err, tt.wantErr)
			}
			calls := mock.CallsFor("snap")
			if len(calls) != 1 {
				t.Fatalf("want 1 snap call, got %d", len(calls))
			}
			if calls[0].Args[0] != "remove" {
				t.Errorf("args = %v, want remove subcommand", calls[0].Args)
			}
		})
	}
}

func TestSnapManager_Name(t *testing.T) {
	if got := (&snapManager{}).Name(); got != "snap" {
		t.Errorf("Name() = %q, want %q", got, "snap")
	}
}

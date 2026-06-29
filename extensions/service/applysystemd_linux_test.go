//go:build linux

package service

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/TsekNet/converge/extensions"
)

// fakeCmd builds a *exec.Cmd that succeeds or fails without invoking systemctl.
// "true"/"false" are tiny coreutils binaries present on any Linux test host;
// if they are somehow absent the caller skips.
func fakeCmd(t *testing.T, fail bool) *exec.Cmd {
	t.Helper()
	bin := "true"
	if fail {
		bin = "false"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		t.Skipf("%q not found in PATH: %v", bin, err)
	}
	return exec.Command(path)
}

// recordingSeam swaps the package commandContext seam for the duration of a
// test, recording the systemctl subcommand of each call and failing the call
// whose subcommand matches failOn ("" means all succeed).
func recordingSeam(t *testing.T, failOn string) *[]string {
	t.Helper()
	var calls []string
	orig := commandContext
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		sub := ""
		if len(args) > 0 {
			sub = args[0]
		}
		calls = append(calls, sub)
		return fakeCmd(t, sub == failOn)
	}
	t.Cleanup(func() { commandContext = orig })
	return &calls
}

func TestApplySystemd_Success(t *testing.T) {
	tests := []struct {
		name      string
		state     string
		enable    bool
		wantCalls []string
		wantMsg   string
	}{
		{
			name:      "running_enabled",
			state:     "running",
			enable:    true,
			wantCalls: []string{"start", "enable"},
			wantMsg:   "started",
		},
		{
			name:      "stopped_disabled",
			state:     "stopped",
			enable:    false,
			wantCalls: []string{"stop", "disable"},
			wantMsg:   "stopped",
		},
		{
			name:      "running_disabled",
			state:     "running",
			enable:    false,
			wantCalls: []string{"start", "disable"},
			wantMsg:   "started",
		},
		{
			name:      "stopped_enabled",
			state:     "stopped",
			enable:    true,
			wantCalls: []string{"stop", "enable"},
			wantMsg:   "stopped",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := recordingSeam(t, "")
			s := New("unit", Opts{State: tt.state, Enable: tt.enable, InitSystem: "systemd"})

			res, err := s.Apply(context.Background())
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if !res.Changed || res.Status != extensions.StatusChanged {
				t.Errorf("Apply() = %+v, want Changed with StatusChanged", res)
			}
			if res.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", res.Message, tt.wantMsg)
			}
			if strings.Join(*calls, ",") != strings.Join(tt.wantCalls, ",") {
				t.Errorf("systemctl subcommands = %v, want %v", *calls, tt.wantCalls)
			}
		})
	}
}

func TestApplySystemd_Errors(t *testing.T) {
	tests := []struct {
		name    string
		state   string
		enable  bool
		failOn  string
		wantSub string // substring expected in error message
	}{
		{name: "start_fails", state: "running", enable: true, failOn: "start", wantSub: "systemctl start"},
		{name: "stop_fails", state: "stopped", enable: false, failOn: "stop", wantSub: "systemctl stop"},
		{name: "enable_fails", state: "running", enable: true, failOn: "enable", wantSub: "systemctl enable"},
		{name: "disable_fails", state: "running", enable: false, failOn: "disable", wantSub: "systemctl disable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recordingSeam(t, tt.failOn)
			s := New("unit", Opts{State: tt.state, Enable: tt.enable, InitSystem: "systemd"})

			res, err := s.Apply(context.Background())
			if err == nil {
				t.Fatalf("Apply() error = nil, want failure on %q", tt.failOn)
			}
			if res != nil {
				t.Errorf("Apply() result = %+v, want nil on error", res)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error = %q, want substring %q", err.Error(), tt.wantSub)
			}
		})
	}
}

// TestWatch_ContextCancelled exercises the Linux D-Bus Watch path. It requires a
// live system bus; without one SharedDbus fails and the test skips rather than
// hard-failing in the non-root/no-bus CI sandbox.
func TestWatch_ContextCancelled(t *testing.T) {
	s := New("cron", Opts{State: "running", Enable: true, InitSystem: "systemd"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already done: Watch should return promptly after subscribing

	events := make(chan extensions.Event, 1)
	done := make(chan error, 1)
	go func() { done <- s.Watch(ctx, events) }()

	select {
	case err := <-done:
		// err may be nil (returned via ctx.Done) or a bus error (no system bus).
		if err != nil {
			t.Skipf("Watch returned (no usable system bus): %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return after context cancellation")
	}
}

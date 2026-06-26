//go:build darwin

package service

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/TsekNet/converge/extensions"
)

// Check dispatches to the detected init system (launchd on macOS).
func (s *Service) Check(ctx context.Context) (*extensions.State, error) {
	switch s.InitSystem {
	case "launchd":
		return s.checkLaunchd(ctx)
	default:
		return nil, fmt.Errorf("unsupported init system: %q", s.InitSystem)
	}
}

func (s *Service) Apply(ctx context.Context) (*extensions.Result, error) {
	switch s.InitSystem {
	case "launchd":
		return s.applyLaunchd(ctx)
	default:
		return nil, fmt.Errorf("apply not implemented for init system: %q", s.InitSystem)
	}
}

// checkLaunchd inspects launchd state via "launchctl list <label>". A loaded
// job prints a plist-style dictionary on success; a running job additionally
// has a "PID" entry. An error (non-zero exit) means the job is not loaded.
//
// This deliberately never assumes compliance: an unmanaged or absent service
// reports drift instead of falsely returning InSync.
func (s *Service) checkLaunchd(ctx context.Context) (*extensions.State, error) {
	var changes []extensions.Change

	out, err := exec.CommandContext(ctx, "launchctl", "list", s.Name).Output()
	loaded := err == nil
	isRunning := loaded && strings.Contains(string(out), `"PID"`)

	wantRunning := s.State == "running"
	if isRunning != wantRunning {
		from, to := "running", "stopped"
		if wantRunning {
			from, to = "stopped", "running"
		}
		changes = append(changes, extensions.Change{
			Property: "state", From: from, To: to, Action: "modify",
		})
	}

	if s.Enable && !loaded {
		changes = append(changes, extensions.Change{
			Property: "enabled", From: "false", To: "true", Action: "modify",
		})
	} else if !s.Enable && loaded {
		changes = append(changes, extensions.Change{
			Property: "enabled", From: "true", To: "false", Action: "modify",
		})
	}

	return &extensions.State{InSync: len(changes) == 0, Changes: changes}, nil
}

// applyLaunchd toggles the disabled flag and starts/stops the job. enable and
// disable operate on the system domain service target (system/<label>); start
// and stop use the legacy label form, which works for an already-loaded job.
func (s *Service) applyLaunchd(ctx context.Context) (*extensions.Result, error) {
	target := "system/" + s.Name

	if s.Enable {
		if out, err := exec.CommandContext(ctx, "launchctl", "enable", target).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("launchctl enable %s: %s: %w", target, strings.TrimSpace(string(out)), err)
		}
	} else {
		if out, err := exec.CommandContext(ctx, "launchctl", "disable", target).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("launchctl disable %s: %s: %w", target, strings.TrimSpace(string(out)), err)
		}
	}

	if s.State == "running" {
		if out, err := exec.CommandContext(ctx, "launchctl", "start", s.Name).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("launchctl start %s: %s: %w", s.Name, strings.TrimSpace(string(out)), err)
		}
	} else {
		if out, err := exec.CommandContext(ctx, "launchctl", "stop", s.Name).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("launchctl stop %s: %s: %w", s.Name, strings.TrimSpace(string(out)), err)
		}
	}

	msg := "started"
	if s.State == "stopped" {
		msg = "stopped"
	}
	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: msg}, nil
}

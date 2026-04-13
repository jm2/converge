// Package cron manages scheduled tasks: cron jobs on Linux/macOS
// and Task Scheduler entries on Windows.
package cron

import (
	"fmt"
	"strings"

	"github.com/TsekNet/converge/extensions"
)

// Cron ensures a cron job or Windows scheduled task exists
// with the specified schedule and command.
type Cron struct {
	Name     string // unique name (cron comment tag on Linux/macOS, task name on Windows)
	Schedule string // cron expression (Linux/macOS) or trigger spec (Windows)
	Command  string // command to execute
	User     string // user to run as (Linux/macOS: crontab owner, Windows: SYSTEM or username)
	State    string // "present" or "absent"
	Critical bool
	FS       extensions.FS // nil uses the real OS filesystem
}

// Opts holds configurable fields for a Cron resource.
type Opts struct {
	Schedule string // cron expression (Linux/macOS) or trigger spec (Windows)
	Command  string // command to execute
	User     string // user to run as (Linux/macOS: crontab owner, Windows: SYSTEM or username)
	State    string // "present" or "absent"
	Critical bool
	FS       extensions.FS // inject a mock for testing
}

// New creates a Cron resource. State defaults to "present" when the Opts
// field is zero-valued.
func New(name string, opts Opts) *Cron {
	state := opts.State
	if state == "" {
		state = "present"
	}
	return &Cron{
		Name:     name,
		Schedule: opts.Schedule,
		Command:  opts.Command,
		User:     opts.User,
		State:    state,
		Critical: opts.Critical,
		FS:       opts.FS,
	}
}

func (c *Cron) fsys() extensions.FS { return extensions.RealFS(c.FS) }

func (c *Cron) ID() string       { return fmt.Sprintf("cron:%s", c.Name) }
func (c *Cron) String() string   { return fmt.Sprintf("Cron %s", c.Name) }
func (c *Cron) IsCritical() bool { return c.Critical }

// validate rejects fields containing newline, carriage return, or null bytes.
// These characters could inject additional cron lines or corrupt Task Scheduler
// XML on Windows.
func (c *Cron) validate() error {
	const bad = "\n\r\x00"
	for _, pair := range []struct {
		name, value string
	}{
		{"Schedule", c.Schedule},
		{"Command", c.Command},
		{"User", c.User},
		{"Name", c.Name},
	} {
		if strings.ContainsAny(pair.value, bad) {
			return fmt.Errorf("cron %s: %s contains invalid character (newline, carriage return, or null)", c.Name, pair.name)
		}
	}
	if strings.Contains(c.Name, "..") {
		return fmt.Errorf("cron %s: Name contains path traversal sequence", c.Name)
	}
	return nil
}

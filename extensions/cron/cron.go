// Package cron manages scheduled tasks: cron jobs on Linux/macOS
// and Task Scheduler entries on Windows.
package cron

import (
	"fmt"
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
}

// Opts holds configurable fields for a Cron resource.
type Opts struct {
	Schedule string // cron expression (Linux/macOS) or trigger spec (Windows)
	Command  string // command to execute
	User     string // user to run as (Linux/macOS: crontab owner, Windows: SYSTEM or username)
	State    string // "present" or "absent"
	Critical bool
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
	}
}

func (c *Cron) ID() string       { return fmt.Sprintf("cron:%s", c.Name) }
func (c *Cron) String() string   { return fmt.Sprintf("Cron %s", c.Name) }
func (c *Cron) IsCritical() bool { return c.Critical }

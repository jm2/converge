package cron

import (
	"testing"
)

func TestCron_ID(t *testing.T) {
	c := New("backup", Opts{Schedule: "0 2 * * *", Command: "/usr/bin/backup.sh"})
	if got := c.ID(); got != "cron:backup" {
		t.Errorf("ID() = %q, want %q", got, "cron:backup")
	}
}

func TestCron_String(t *testing.T) {
	c := New("backup", Opts{Schedule: "0 2 * * *", Command: "/usr/bin/backup.sh"})
	if got := c.String(); got != "Cron backup" {
		t.Errorf("String() = %q, want %q", got, "Cron backup")
	}
}

func TestCron_IsCritical(t *testing.T) {
	c := New("test", Opts{Schedule: "* * * * *", Command: "echo"})
	if c.IsCritical() {
		t.Error("IsCritical() should be false by default")
	}
	c2 := New("test", Opts{Schedule: "* * * * *", Command: "echo", Critical: true})
	if !c2.IsCritical() {
		t.Error("IsCritical() should be true when set via Opts")
	}
}

func TestNew(t *testing.T) {
	c := New("cleanup", Opts{Schedule: "30 3 * * 0", Command: "/opt/cleanup.sh"})
	if c.Name != "cleanup" {
		t.Errorf("Name = %q, want %q", c.Name, "cleanup")
	}
	if c.Schedule != "30 3 * * 0" {
		t.Errorf("Schedule = %q", c.Schedule)
	}
	if c.Command != "/opt/cleanup.sh" {
		t.Errorf("Command = %q", c.Command)
	}
	if c.State != "present" {
		t.Errorf("State = %q, want %q", c.State, "present")
	}
}

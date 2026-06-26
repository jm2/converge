//go:build linux

package cron

import (
	"context"
	"testing"

	"github.com/TsekNet/converge/internal/testutil"
)

func TestCron_MapFS_CheckMissing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mfs := testutil.NewMapFS()
	c := New("backup", Opts{
		Schedule: "0 2 * * *",
		Command:  "/usr/bin/backup.sh",
		FS:       mfs,
	})

	state, err := c.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if state.InSync {
		t.Error("Check() should report drift when cron file is missing")
	}
	if len(state.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(state.Changes))
	}
	if state.Changes[0].Action != "add" {
		t.Errorf("Changes[0].Action = %q, want %q", state.Changes[0].Action, "add")
	}
}

func TestCron_MapFS_CheckInSync(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mfs := testutil.NewMapFS()
	c := New("backup", Opts{
		Schedule: "0 2 * * *",
		Command:  "/usr/bin/backup.sh",
		FS:       mfs,
	})

	// Seed the expected content.
	mfs.Set(c.cronFilePath(), []byte(c.cronLine()+"\n"), 0644)

	state, err := c.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !state.InSync {
		t.Errorf("Check() should report in-sync, got changes: %+v", state.Changes)
	}
}

func TestCron_MapFS_Converges(t *testing.T) {
	t.Parallel()

	mfs := testutil.NewMapFS()
	c := New("backup", Opts{
		Schedule: "0 2 * * *",
		Command:  "/usr/bin/backup.sh",
		FS:       mfs,
	})

	testutil.AssertConverges(t, c)

	data, ok := mfs.Get(c.cronFilePath())
	if !ok {
		t.Fatal("cron file should exist in MapFS after Apply")
	}
	want := c.cronLine() + "\n"
	if string(data) != want {
		t.Errorf("content = %q, want %q", data, want)
	}
}

func TestCron_MapFS_AbsentConverges(t *testing.T) {
	t.Parallel()

	mfs := testutil.NewMapFS()
	c := New("cleanup", Opts{
		Schedule: "30 3 * * 0",
		Command:  "/opt/cleanup.sh",
		State:    "absent",
		FS:       mfs,
	})

	// Seed an existing cron file that should be removed.
	mfs.Set(c.cronFilePath(), []byte(c.cronLine()+"\n"), 0644)

	testutil.AssertConverges(t, c)

	if mfs.Has(c.cronFilePath()) {
		t.Error("cron file should be removed after Apply with state=absent")
	}
}

func TestCron_MapFS_AbsentAlreadyGone(t *testing.T) {
	t.Parallel()

	mfs := testutil.NewMapFS()
	c := New("cleanup", Opts{
		Schedule: "30 3 * * 0",
		Command:  "/opt/cleanup.sh",
		State:    "absent",
		FS:       mfs,
	})

	testutil.AssertInSync(t, c)
}

func TestCron_MapFS_ContentDrift(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mfs := testutil.NewMapFS()
	c := New("backup", Opts{
		Schedule: "0 2 * * *",
		Command:  "/usr/bin/backup.sh",
		FS:       mfs,
	})

	// Seed with wrong content.
	mfs.Set(c.cronFilePath(), []byte("0 3 * * * root /usr/bin/old.sh # converge:backup\n"), 0644)

	state, err := c.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if state.InSync {
		t.Error("Check() should detect content drift")
	}
	if len(state.Changes) == 0 {
		t.Error("expected at least one change")
	}
}

//go:build darwin

package cron

import (
	"context"
	"strings"
	"testing"

	"github.com/TsekNet/converge/internal/testutil"
)

func TestCron_Darwin_CheckMissing(t *testing.T) {
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
		t.Error("Check() should report drift when the crontab is missing")
	}
	if len(state.Changes) != 1 || state.Changes[0].Action != "add" {
		t.Fatalf("expected one add change, got %+v", state.Changes)
	}
}

func TestCron_Darwin_WritesSystemCrontab(t *testing.T) {
	t.Parallel()

	mfs := testutil.NewMapFS()
	c := New("backup", Opts{
		Schedule: "0 2 * * *",
		Command:  "/usr/bin/backup.sh",
		FS:       mfs,
	})

	testutil.AssertConverges(t, c)

	data, ok := mfs.Get(systemCrontab)
	if !ok {
		t.Fatalf("%s should exist after Apply", systemCrontab)
	}
	if !containsLine(string(data), c.cronLine()) {
		t.Errorf("crontab %q missing desired line %q", data, c.cronLine())
	}
}

func TestCron_Darwin_PreservesOtherLines(t *testing.T) {
	t.Parallel()

	mfs := testutil.NewMapFS()
	existing := "# system crontab\n0 5 * * * root /usr/sbin/periodic daily\n"
	mfs.Set(systemCrontab, []byte(existing), 0644)

	c := New("backup", Opts{
		Schedule: "0 2 * * *",
		Command:  "/usr/bin/backup.sh",
		FS:       mfs,
	})

	testutil.AssertConverges(t, c)

	data, _ := mfs.Get(systemCrontab)
	got := string(data)
	if !strings.Contains(got, "0 5 * * * root /usr/sbin/periodic daily") {
		t.Errorf("Apply dropped pre-existing crontab lines: %q", got)
	}
	if !containsLine(got, c.cronLine()) {
		t.Errorf("Apply did not add managed line: %q", got)
	}
}

func TestCron_Darwin_AbsentRemovesOnlyTaggedLine(t *testing.T) {
	t.Parallel()

	mfs := testutil.NewMapFS()
	c := New("cleanup", Opts{
		Schedule: "30 3 * * 0",
		Command:  "/opt/cleanup.sh",
		State:    "absent",
		FS:       mfs,
	})

	other := "0 5 * * * root /usr/sbin/periodic daily\n"
	mfs.Set(systemCrontab, []byte(other+c.cronLine()+"\n"), 0644)

	testutil.AssertConverges(t, c)

	data, _ := mfs.Get(systemCrontab)
	got := string(data)
	if containsLine(got, c.cronLine()) {
		t.Errorf("managed line should be removed, got %q", got)
	}
	if !strings.Contains(got, "0 5 * * * root /usr/sbin/periodic daily") {
		t.Errorf("unrelated line should be preserved, got %q", got)
	}
}

func TestCron_Darwin_TagDoesNotMatchPrefix(t *testing.T) {
	t.Parallel()

	mfs := testutil.NewMapFS()
	// A different task whose name has ours as a prefix must not be touched.
	other := New("backup-extra", Opts{Schedule: "0 4 * * *", Command: "/usr/bin/extra.sh"})
	mfs.Set(systemCrontab, []byte(other.cronLine()+"\n"), 0644)

	c := New("backup", Opts{
		Schedule: "0 2 * * *",
		Command:  "/usr/bin/backup.sh",
		FS:       mfs,
	})

	testutil.AssertConverges(t, c)

	data, _ := mfs.Get(systemCrontab)
	got := string(data)
	if !containsLine(got, other.cronLine()) {
		t.Errorf("prefix-sharing task line was wrongly removed: %q", got)
	}
	if !containsLine(got, c.cronLine()) {
		t.Errorf("managed line missing: %q", got)
	}
}

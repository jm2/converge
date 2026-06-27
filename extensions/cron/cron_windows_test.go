//go:build windows

package cron

import "testing"

// sig is a test helper that computes the canonical signature for a cron spec.
func sig(t *testing.T, spec string) string {
	t.Helper()
	ps, err := parseCronSchedule(spec)
	if err != nil {
		t.Fatalf("parseCronSchedule(%q): %v", spec, err)
	}
	return ps.signature()
}

func TestCheckState(t *testing.T) {
	daily := "0 2 * * *" // -> "daily 02:00"

	tests := []struct {
		name        string
		cron        *Cron
		info        taskInfo
		wantSync    bool
		wantChanges int
	}{
		{
			"task fully matches",
			New("backup", Opts{Schedule: daily, Command: "/usr/bin/backup.sh"}),
			taskInfo{exists: true, command: "/usr/bin/backup.sh", user: "SYSTEM", runLevelHighest: true, schedule: "daily 02:00"},
			true, 0,
		},
		{
			"command differs",
			New("backup", Opts{Schedule: daily, Command: "/usr/bin/backup-v2.sh"}),
			taskInfo{exists: true, command: "/usr/bin/backup.sh", user: "SYSTEM", runLevelHighest: true, schedule: "daily 02:00"},
			false, 1,
		},
		{
			"schedule drift (triggerless task reports empty schedule)",
			New("backup", Opts{Schedule: daily, Command: "/usr/bin/backup.sh"}),
			taskInfo{exists: true, command: "/usr/bin/backup.sh", user: "SYSTEM", runLevelHighest: true, schedule: ""},
			false, 1,
		},
		{
			"run level drift",
			New("backup", Opts{Schedule: daily, Command: "/usr/bin/backup.sh"}),
			taskInfo{exists: true, command: "/usr/bin/backup.sh", user: "SYSTEM", runLevelHighest: false, schedule: "daily 02:00"},
			false, 1,
		},
		{
			"user drift",
			New("backup", Opts{Schedule: daily, Command: "/usr/bin/backup.sh", User: "alice"}),
			taskInfo{exists: true, command: "/usr/bin/backup.sh", user: "bob", runLevelHighest: true, schedule: "daily 02:00"},
			false, 1,
		},
		{
			"SYSTEM aliases do not drift",
			New("backup", Opts{Schedule: daily, Command: "/usr/bin/backup.sh"}),
			taskInfo{exists: true, command: "/usr/bin/backup.sh", user: `NT AUTHORITY\SYSTEM`, runLevelHighest: true, schedule: "daily 02:00"},
			true, 0,
		},
		{
			"args drift",
			New("backup", Opts{Schedule: daily, Command: "/usr/bin/backup.sh"}),
			taskInfo{exists: true, command: "/usr/bin/backup.sh", args: "--unexpected", user: "SYSTEM", runLevelHighest: true, schedule: "daily 02:00"},
			false, 1,
		},
		{
			"task does not exist",
			New("backup", Opts{Schedule: daily, Command: "/usr/bin/backup.sh"}),
			taskInfo{exists: false},
			false, 1,
		},
		{
			"want absent, task exists",
			New("cleanup", Opts{Schedule: daily, Command: "echo", State: "absent"}),
			taskInfo{exists: true, command: "echo"},
			false, 1,
		},
		{
			"want absent, task already gone",
			New("cleanup", Opts{Schedule: daily, Command: "echo", State: "absent"}),
			taskInfo{exists: false},
			true, 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := ""
			if tt.cron.State != "absent" {
				want = sig(t, tt.cron.Schedule)
			}
			state := tt.cron.checkState(tt.info, want)
			if state.InSync != tt.wantSync {
				t.Errorf("InSync = %v, want %v", state.InSync, tt.wantSync)
			}
			if len(state.Changes) != tt.wantChanges {
				t.Errorf("len(Changes) = %d, want %d: %+v", len(state.Changes), tt.wantChanges, state.Changes)
			}
		})
	}
}

func TestBuildTrigger(t *testing.T) {
	tests := []struct {
		spec string
		want string // expected triggerSignature round-trip
	}{
		{"0 2 * * *", "daily 02:00"},
		{"30 3 * * 1,3,5", "weekly 03:30 dow=1,3,5"},
		{"0 0 1,15 * *", "monthly 00:00 dom=1,15 months=ALL"},
		{"0 0 1 6 *", "monthly 00:00 dom=1 months=6"},
	}
	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			ps, err := parseCronSchedule(tt.spec)
			if err != nil {
				t.Fatalf("parseCronSchedule: %v", err)
			}
			trig, err := buildTrigger(ps)
			if err != nil {
				t.Fatalf("buildTrigger: %v", err)
			}
			if got := triggerSignature(trig); got != tt.want {
				t.Errorf("round-trip signature = %q, want %q", got, tt.want)
			}
		})
	}
}

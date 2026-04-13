//go:build windows

package cron

import "testing"

func TestCheckState(t *testing.T) {
	tests := []struct {
		name        string
		cron        *Cron
		info        taskInfo
		wantSync    bool
		wantChanges int
	}{
		{
			"task exists and command matches",
			New("backup", Opts{Schedule: "daily", Command: "/usr/bin/backup.sh"}),
			taskInfo{exists: true, command: "/usr/bin/backup.sh"},
			true, 0,
		},
		{
			"task exists but command differs",
			New("backup", Opts{Schedule: "daily", Command: "/usr/bin/backup-v2.sh"}),
			taskInfo{exists: true, command: "/usr/bin/backup.sh"},
			false, 1,
		},
		{
			"task does not exist",
			New("backup", Opts{Schedule: "daily", Command: "/usr/bin/backup.sh"}),
			taskInfo{exists: false},
			false, 1,
		},
		{
			"want absent, task exists",
			New("cleanup", Opts{Schedule: "daily", Command: "echo", State: "absent"}),
			taskInfo{exists: true, command: "echo"},
			false, 1,
		},
		{
			"want absent, task already gone",
			New("cleanup", Opts{Schedule: "daily", Command: "echo", State: "absent"}),
			taskInfo{exists: false},
			true, 0,
		},
		{
			"task exists, command empty in info (COM read failed gracefully)",
			New("backup", Opts{Schedule: "daily", Command: "/usr/bin/backup.sh"}),
			taskInfo{exists: true, command: ""},
			false, 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := tt.cron.checkState(tt.info)
			if state.InSync != tt.wantSync {
				t.Errorf("InSync = %v, want %v", state.InSync, tt.wantSync)
			}
			if len(state.Changes) != tt.wantChanges {
				t.Errorf("len(Changes) = %d, want %d: %+v", len(state.Changes), tt.wantChanges, state.Changes)
			}
		})
	}
}

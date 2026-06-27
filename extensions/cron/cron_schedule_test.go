package cron

import "testing"

func TestParseCronSchedule_Signatures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		spec string
		want string
	}{
		{"0 2 * * *", "daily 02:00"},
		{"30 3 * * *", "daily 03:30"},
		{"5 0 * * *", "daily 00:05"},
		{"30 3 * * 0", "weekly 03:30 dow=0"},
		{"30 3 * * 7", "weekly 03:30 dow=0"}, // 7 normalizes to Sunday(0)
		{"0 9 * * 1,3,5", "weekly 09:00 dow=1,3,5"},
		{"0 9 * * 5,1,3", "weekly 09:00 dow=1,3,5"}, // sorted
		{"0 9 * * 1,1,2", "weekly 09:00 dow=1,2"},   // de-duplicated
		{"0 0 1 * *", "monthly 00:00 dom=1 months=ALL"},
		{"0 0 1,15 * *", "monthly 00:00 dom=1,15 months=ALL"},
		{"0 0 1,15 6 *", "monthly 00:00 dom=1,15 months=6"},
		{"0 0 1 3,6,9,12 *", "monthly 00:00 dom=1 months=3,6,9,12"},
		// An explicit list of all 12 months collapses to ALL.
		{"0 0 1 1,2,3,4,5,6,7,8,9,10,11,12 *", "monthly 00:00 dom=1 months=ALL"},
	}
	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			ps, err := parseCronSchedule(tt.spec)
			if err != nil {
				t.Fatalf("parseCronSchedule(%q) error = %v", tt.spec, err)
			}
			if got := ps.signature(); got != tt.want {
				t.Errorf("signature() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseCronSchedule_Errors(t *testing.T) {
	t.Parallel()
	specs := []string{
		"",            // empty
		"0 2 * *",     // too few fields
		"0 2 * * * *", // too many fields
		"* 2 * * *",   // wildcard minute (unrepresentable as a single trigger)
		"0 * * * *",   // wildcard hour
		"*/5 * * * *", // step minute
		"0 2 1 * 1",   // both day-of-month and day-of-week
		"0 2 1-5 * *", // range day-of-month
		"0 2 * * 1-5", // range day-of-week
		"0 2 * 6 *",   // month restriction with no specific day (daily)
		"0 2 * 6 1",   // weekly schedule cannot restrict months
		"60 2 * * *",  // minute out of range
		"0 24 * * *",  // hour out of range
		"0 2 32 * *",  // day-of-month out of range
		"0 2 * 13 *",  // month out of range
		"0 2 * * 8",   // day-of-week out of range
		"x 2 * * *",   // non-numeric minute
	}
	for _, spec := range specs {
		t.Run(spec, func(t *testing.T) {
			if _, err := parseCronSchedule(spec); err == nil {
				t.Errorf("parseCronSchedule(%q) expected error, got nil", spec)
			}
		})
	}
}

func TestNormalizeUser(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want string
	}{
		{"", "system"},
		{"SYSTEM", "system"},
		{"System", "system"},
		{`NT AUTHORITY\SYSTEM`, "system"},
		{"S-1-5-18", "system"},
		{"LocalService", "localservice"},
		{"S-1-5-19", "localservice"},
		{"Network Service", "networkservice"},
		{"S-1-5-20", "networkservice"},
		{`DOMAIN\alice`, `domain\alice`},
		{"alice", "alice"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := normalizeUser(tt.in); got != tt.want {
				t.Errorf("normalizeUser(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestJoinInts(t *testing.T) {
	t.Parallel()
	if got := joinInts(nil); got != "" {
		t.Errorf("joinInts(nil) = %q, want empty", got)
	}
	if got := joinInts([]int{1}); got != "1" {
		t.Errorf("joinInts([1]) = %q, want %q", got, "1")
	}
	if got := joinInts([]int{1, 2, 3}); got != "1,2,3" {
		t.Errorf("joinInts([1,2,3]) = %q, want %q", got, "1,2,3")
	}
}

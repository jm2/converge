package cron

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// scheduleKind identifies which Windows Task Scheduler trigger a cron
// expression maps onto.
type scheduleKind int

const (
	scheduleDaily scheduleKind = iota
	scheduleWeekly
	scheduleMonthly
)

// parsedSchedule is an OS-independent representation of a cron expression. It is
// consumed on Windows to build a Task Scheduler trigger and to detect schedule
// drift. Keeping the parsing here (rather than in the Windows-only file) lets it
// be unit-tested on any platform.
type parsedSchedule struct {
	kind        scheduleKind
	hour        int   // 0-23
	minute      int   // 0-59
	daysOfWeek  []int // 0-6 (Sunday=0), sorted; set when kind == scheduleWeekly
	daysOfMonth []int // 1-31, sorted; set when kind == scheduleMonthly
	months      []int // 1-12, sorted; empty means every month (scheduleMonthly only)
}

// parseCronSchedule translates a standard 5-field cron expression
// (minute hour day-of-month month day-of-week) into a parsedSchedule.
//
// It supports the subset of cron that maps cleanly onto a single Windows Task
// Scheduler trigger: a concrete minute and hour, plus optionally a list of
// days-of-week (weekly) or a list of days-of-month with an optional list of
// months (monthly). Expressions that cannot be represented by a single trigger
// (ranges or steps in any field, both day-of-month and day-of-week set, etc.)
// return an error so the caller can refuse to register a triggerless task.
func parseCronSchedule(spec string) (parsedSchedule, error) {
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return parsedSchedule{}, fmt.Errorf("schedule %q: expected 5 cron fields (minute hour day-of-month month day-of-week)", spec)
	}

	minute, err := parseCronSingle(fields[0], 0, 59)
	if err != nil {
		return parsedSchedule{}, fmt.Errorf("schedule %q: minute: %w", spec, err)
	}
	hour, err := parseCronSingle(fields[1], 0, 23)
	if err != nil {
		return parsedSchedule{}, fmt.Errorf("schedule %q: hour: %w", spec, err)
	}
	dom, domAll, err := parseCronList(fields[2], 1, 31)
	if err != nil {
		return parsedSchedule{}, fmt.Errorf("schedule %q: day-of-month: %w", spec, err)
	}
	months, monthsAll, err := parseCronList(fields[3], 1, 12)
	if err != nil {
		return parsedSchedule{}, fmt.Errorf("schedule %q: month: %w", spec, err)
	}
	dow, dowAll, err := parseCronList(fields[4], 0, 7)
	if err != nil {
		return parsedSchedule{}, fmt.Errorf("schedule %q: day-of-week: %w", spec, err)
	}
	// cron treats both 0 and 7 as Sunday; normalize 7 to 0.
	dow = normalizeWeekdays(dow)

	ps := parsedSchedule{hour: hour, minute: minute}

	switch {
	case domAll && dowAll:
		if !monthsAll {
			return parsedSchedule{}, fmt.Errorf("schedule %q: a month restriction requires specific days", spec)
		}
		ps.kind = scheduleDaily
	case !dowAll && domAll:
		if !monthsAll {
			return parsedSchedule{}, fmt.Errorf("schedule %q: a weekly schedule cannot restrict months", spec)
		}
		ps.kind = scheduleWeekly
		ps.daysOfWeek = dow
	case !domAll && dowAll:
		ps.kind = scheduleMonthly
		ps.daysOfMonth = dom
		// An explicit list covering every month is equivalent to "*".
		if !monthsAll && len(months) < 12 {
			ps.months = months
		}
	default:
		return parsedSchedule{}, fmt.Errorf("schedule %q: cannot set both day-of-month and day-of-week", spec)
	}

	return ps, nil
}

// parseCronSingle parses a cron field that must be a single concrete integer
// within [min, max]. Wildcards, lists, ranges, and steps are rejected.
func parseCronSingle(field string, min, max int) (int, error) {
	if strings.ContainsAny(field, "*,-/") {
		return 0, fmt.Errorf("%q must be a single value (wildcards, ranges, lists, and steps are unsupported)", field)
	}
	n, err := strconv.Atoi(field)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number", field)
	}
	if n < min || n > max {
		return 0, fmt.Errorf("%d out of range [%d,%d]", n, min, max)
	}
	return n, nil
}

// parseCronList parses a cron field that is either "*" (all, returns all=true)
// or a comma-separated list of concrete integers within [min, max]. Ranges and
// steps are rejected. The returned slice is sorted and de-duplicated.
func parseCronList(field string, min, max int) (vals []int, all bool, err error) {
	if field == "*" {
		return nil, true, nil
	}
	if strings.ContainsAny(field, "*-/") {
		return nil, false, fmt.Errorf("%q: ranges, steps, and partial wildcards are unsupported", field)
	}
	seen := make(map[int]bool)
	for _, part := range strings.Split(field, ",") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, false, fmt.Errorf("%q is not a number", part)
		}
		if n < min || n > max {
			return nil, false, fmt.Errorf("%d out of range [%d,%d]", n, min, max)
		}
		if !seen[n] {
			seen[n] = true
			vals = append(vals, n)
		}
	}
	sort.Ints(vals)
	return vals, false, nil
}

// normalizeWeekdays maps cron's Sunday=7 to 0, then sorts and de-duplicates.
func normalizeWeekdays(dow []int) []int {
	if len(dow) == 0 {
		return dow
	}
	seen := make(map[int]bool)
	var out []int
	for _, d := range dow {
		if d == 7 {
			d = 0
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	sort.Ints(out)
	return out
}

// signature returns a stable, canonical string describing the schedule. It is
// used to compare the desired schedule against the trigger read back from an
// existing task, so the Windows decode path must produce the identical format.
func (p parsedSchedule) signature() string {
	clock := fmt.Sprintf("%02d:%02d", p.hour, p.minute)
	switch p.kind {
	case scheduleWeekly:
		return fmt.Sprintf("weekly %s dow=%s", clock, joinInts(p.daysOfWeek))
	case scheduleMonthly:
		months := "ALL"
		if len(p.months) > 0 {
			months = joinInts(p.months)
		}
		return fmt.Sprintf("monthly %s dom=%s months=%s", clock, joinInts(p.daysOfMonth), months)
	default:
		return fmt.Sprintf("daily %s", clock)
	}
}

// joinInts renders a slice of ints as a comma-separated string.
func joinInts(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return strings.Join(parts, ",")
}

// normalizeUser collapses the well-known Windows service-account aliases to a
// canonical form so that a task registered as "SYSTEM" does not read back as
// drift when Task Scheduler reports it as "S-1-5-18" or "NT AUTHORITY\SYSTEM".
// An empty desired user is treated as SYSTEM (the createTask default).
func normalizeUser(u string) string {
	s := strings.ToLower(strings.TrimSpace(u))
	switch s {
	case "", "system", "localsystem", "local system", `nt authority\system`, "s-1-5-18":
		return "system"
	case "localservice", "local service", `nt authority\local service`, "s-1-5-19":
		return "localservice"
	case "networkservice", "network service", `nt authority\network service`, "s-1-5-20":
		return "networkservice"
	}
	return s
}

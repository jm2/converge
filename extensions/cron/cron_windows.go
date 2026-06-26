//go:build windows

package cron

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/capnspacehook/taskmaster"

	"github.com/TsekNet/converge/extensions"
)

// Check queries the Windows Task Scheduler for the named task and compares its
// configured action, schedule (trigger), run-as user, and privilege level
// against the desired state.
func (c *Cron) Check(_ context.Context) (*extensions.State, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	wantSchedule := ""
	if c.State != "absent" {
		ps, err := parseCronSchedule(c.Schedule)
		if err != nil {
			return nil, fmt.Errorf("cron %s: %w", c.Name, err)
		}
		wantSchedule = ps.signature()
	}

	info, err := getTaskInfo(c.Name)
	if err != nil {
		return nil, fmt.Errorf("query task %s: %w", c.Name, err)
	}

	return c.checkState(info, wantSchedule), nil
}

// Apply creates or removes a Windows scheduled task via the Task Scheduler API.
func (c *Cron) Apply(_ context.Context) (*extensions.Result, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	if c.State == "absent" {
		if err := deleteTask(c.Name); err != nil {
			return nil, err
		}
		return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "removed"}, nil
	}

	if err := createTask(c.Name, c.Schedule, c.Command, c.User); err != nil {
		return nil, err
	}
	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "created"}, nil
}

// taskInfo holds the properties of an existing scheduled task.
type taskInfo struct {
	exists          bool
	command         string
	args            string
	user            string
	runLevelHighest bool
	schedule        string // canonical trigger signature, "" if absent/unrepresentable
}

// checkState compares the desired state against the queried task info.
// wantSchedule is the canonical signature of the desired schedule (empty when
// the desired state is "absent"). Extracted for testability (taskmaster
// requires Windows).
func (c *Cron) checkState(info taskInfo, wantSchedule string) *extensions.State {
	if c.State == "absent" {
		if !info.exists {
			return &extensions.State{InSync: true}
		}
		return &extensions.State{
			InSync: false,
			Changes: []extensions.Change{
				{Property: "task", From: c.Name, To: "", Action: "remove"},
			},
		}
	}

	if !info.exists {
		return &extensions.State{
			InSync: false,
			Changes: []extensions.Change{
				{Property: "task", To: c.Name, Action: "add"},
			},
		}
	}

	var changes []extensions.Change
	if info.command != c.Command {
		changes = append(changes, extensions.Change{
			Property: "command",
			From:     info.command,
			To:       c.Command,
			Action:   "modify",
		})
	}
	if info.schedule != wantSchedule {
		changes = append(changes, extensions.Change{
			Property: "schedule",
			From:     info.schedule,
			To:       wantSchedule,
			Action:   "modify",
		})
	}
	wantUser := c.User
	if wantUser == "" {
		wantUser = "SYSTEM"
	}
	if normalizeUser(info.user) != normalizeUser(wantUser) {
		changes = append(changes, extensions.Change{
			Property: "user",
			From:     info.user,
			To:       wantUser,
			Action:   "modify",
		})
	}
	if !info.runLevelHighest {
		changes = append(changes, extensions.Change{
			Property: "runlevel",
			From:     "limited",
			To:       "highest",
			Action:   "modify",
		})
	}
	if info.args != "" {
		changes = append(changes, extensions.Change{
			Property: "args",
			From:     info.args,
			To:       "",
			Action:   "modify",
		})
	}

	return &extensions.State{InSync: len(changes) == 0, Changes: changes}
}

// getTaskInfo retrieves the task's existence, configured command, run-as
// principal, and schedule.
func getTaskInfo(name string) (taskInfo, error) {
	svc, err := taskmaster.Connect()
	if err != nil {
		return taskInfo{}, fmt.Errorf("taskmaster.Connect: %w", err)
	}
	defer svc.Disconnect()

	task, err := svc.GetRegisteredTask(`\` + name)
	if err != nil {
		return taskInfo{exists: false}, nil
	}
	defer task.Release()

	info := taskInfo{exists: true}

	for _, action := range task.Definition.Actions {
		if ea, ok := action.(taskmaster.ExecAction); ok {
			info.command = ea.Path
			info.args = ea.Args
			break
		}
	}

	info.user = task.Definition.Principal.UserID
	info.runLevelHighest = task.Definition.Principal.RunLevel == taskmaster.TASK_RUNLEVEL_HIGHEST

	for _, trig := range task.Definition.Triggers {
		if sig := triggerSignature(trig); sig != "" {
			info.schedule = sig
			break
		}
	}

	return info, nil
}

// createTask registers a new task using the taskmaster API. The schedule is
// translated into a Task Scheduler trigger before any mutation occurs; an
// unrepresentable schedule returns an error rather than registering a
// triggerless (never-firing) task.
func createTask(name, schedule, command, user string) error {
	ps, err := parseCronSchedule(schedule)
	if err != nil {
		return fmt.Errorf("cron %s: %w", name, err)
	}
	trigger, err := buildTrigger(ps)
	if err != nil {
		return fmt.Errorf("cron %s: %w", name, err)
	}

	svc, err := taskmaster.Connect()
	if err != nil {
		return fmt.Errorf("taskmaster.Connect: %w", err)
	}
	defer svc.Disconnect()

	// Delete existing task for idempotent recreate
	tasks, err := svc.GetRegisteredTasks()
	if err == nil {
		for _, t := range tasks {
			if strings.EqualFold(t.Name, name) {
				svc.DeleteTask(t.Path)
				break
			}
		}
		tasks.Release()
	}

	def := svc.NewTaskDefinition()
	def.AddAction(taskmaster.ExecAction{
		ID:   name,
		Path: command,
	})
	def.AddTrigger(trigger)
	def.RegistrationInfo.Description = "Managed by Converge: " + name
	def.Principal.RunLevel = taskmaster.TASK_RUNLEVEL_HIGHEST
	if user == "" {
		user = "SYSTEM"
	}
	def.Principal.UserID = user

	task, ok, err := svc.CreateTask(`\`+name, def, true)
	if err != nil {
		return fmt.Errorf("CreateTask(%s): %w", name, err)
	}
	if !ok {
		return fmt.Errorf("CreateTask(%s): registration failed", name)
	}
	task.Release()
	return nil
}

// buildTrigger constructs the Task Scheduler trigger for a parsed cron schedule.
func buildTrigger(ps parsedSchedule) (taskmaster.Trigger, error) {
	// StartBoundary needs a concrete date; use a fixed past date so the trigger
	// is active immediately. Only the time-of-day is significant for recurrence.
	start := time.Date(2000, 1, 1, ps.hour, ps.minute, 0, 0, time.Local)
	base := taskmaster.TaskTrigger{Enabled: true, StartBoundary: start}

	switch ps.kind {
	case scheduleDaily:
		return taskmaster.DailyTrigger{
			TaskTrigger: base,
			DayInterval: taskmaster.EveryDay,
		}, nil
	case scheduleWeekly:
		return taskmaster.WeeklyTrigger{
			TaskTrigger:  base,
			DaysOfWeek:   weekdaysToMask(ps.daysOfWeek),
			WeekInterval: taskmaster.EveryWeek,
		}, nil
	case scheduleMonthly:
		return taskmaster.MonthlyTrigger{
			TaskTrigger:  base,
			DaysOfMonth:  daysOfMonthToMask(ps.daysOfMonth),
			MonthsOfYear: monthsToMask(ps.months),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported schedule kind")
	}
}

// triggerSignature renders a Task Scheduler trigger into the same canonical
// form as parsedSchedule.signature so the two can be compared for drift. An
// unexpected or unsupported trigger type yields "", which never matches a
// desired schedule and therefore reports drift.
func triggerSignature(t taskmaster.Trigger) string {
	start := t.GetStartBoundary()
	clock := fmt.Sprintf("%02d:%02d", start.Hour(), start.Minute())
	switch tr := t.(type) {
	case taskmaster.DailyTrigger:
		return fmt.Sprintf("daily %s", clock)
	case taskmaster.WeeklyTrigger:
		return fmt.Sprintf("weekly %s dow=%s", clock, joinInts(weekdaysFromMask(tr.DaysOfWeek)))
	case taskmaster.MonthlyTrigger:
		months := "ALL"
		if tr.MonthsOfYear != taskmaster.AllMonths {
			months = joinInts(monthsFromMask(tr.MonthsOfYear))
		}
		return fmt.Sprintf("monthly %s dom=%s months=%s", clock, joinInts(daysOfMonthFromMask(tr.DaysOfMonth)), months)
	default:
		return ""
	}
}

// weekdaysToMask converts cron weekday numbers (Sunday=0) into the bitmask used
// by the Task Scheduler (Sunday is bit 0).
func weekdaysToMask(days []int) taskmaster.DayOfWeek {
	var m taskmaster.DayOfWeek
	for _, d := range days {
		m |= taskmaster.DayOfWeek(1) << uint(d)
	}
	return m
}

// weekdaysFromMask is the inverse of weekdaysToMask.
func weekdaysFromMask(m taskmaster.DayOfWeek) []int {
	var days []int
	for i := 0; i < 7; i++ {
		if m&(taskmaster.DayOfWeek(1)<<uint(i)) != 0 {
			days = append(days, i)
		}
	}
	return days
}

// daysOfMonthToMask converts day-of-month numbers (1-31) into the bitmask used
// by the Task Scheduler (day 1 is bit 0).
func daysOfMonthToMask(days []int) taskmaster.DayOfMonth {
	var m taskmaster.DayOfMonth
	for _, d := range days {
		m |= taskmaster.DayOfMonth(1) << uint(d-1)
	}
	return m
}

// daysOfMonthFromMask is the inverse of daysOfMonthToMask.
func daysOfMonthFromMask(m taskmaster.DayOfMonth) []int {
	var days []int
	for i := 0; i < 31; i++ {
		if m&(taskmaster.DayOfMonth(1)<<uint(i)) != 0 {
			days = append(days, i+1)
		}
	}
	return days
}

// monthsToMask converts month numbers (1-12) into the bitmask used by the Task
// Scheduler (January is bit 0). An empty slice means every month.
func monthsToMask(months []int) taskmaster.Month {
	if len(months) == 0 {
		return taskmaster.AllMonths
	}
	var m taskmaster.Month
	for _, mo := range months {
		m |= taskmaster.Month(1) << uint(mo-1)
	}
	return m
}

// monthsFromMask is the inverse of monthsToMask for a specific (non-all) mask.
func monthsFromMask(m taskmaster.Month) []int {
	var months []int
	for i := 0; i < 12; i++ {
		if m&(taskmaster.Month(1)<<uint(i)) != 0 {
			months = append(months, i+1)
		}
	}
	return months
}

// deleteTask removes a task by name.
func deleteTask(name string) error {
	svc, err := taskmaster.Connect()
	if err != nil {
		return fmt.Errorf("taskmaster.Connect: %w", err)
	}
	defer svc.Disconnect()

	tasks, err := svc.GetRegisteredTasks()
	if err != nil {
		return fmt.Errorf("GetRegisteredTasks: %w", err)
	}
	defer tasks.Release()

	for _, t := range tasks {
		if strings.EqualFold(t.Name, name) {
			return svc.DeleteTask(t.Path)
		}
	}
	return nil // task already absent
}

//go:build windows

package cron

import (
	"context"
	"fmt"
	"strings"

	"github.com/capnspacehook/taskmaster"

	"github.com/TsekNet/converge/extensions"
)

// Check queries the Windows Task Scheduler for the named task and compares
// its configured action against the desired command.
func (c *Cron) Check(_ context.Context) (*extensions.State, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	info, err := getTaskInfo(c.Name)
	if err != nil {
		return nil, fmt.Errorf("query task %s: %w", c.Name, err)
	}

	return c.checkState(info), nil
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

	if err := createTask(c.Name, c.Command, c.User); err != nil {
		return nil, err
	}
	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "created"}, nil
}

// taskInfo holds the properties of an existing scheduled task.
type taskInfo struct {
	exists  bool
	command string
}

// checkState compares the desired state against the queried task info.
// Extracted for testability (taskmaster requires Windows).
func (c *Cron) checkState(info taskInfo) *extensions.State {
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

	return &extensions.State{InSync: len(changes) == 0, Changes: changes}
}

// getTaskInfo retrieves the task's existence and configured command.
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

	var command string
	for _, action := range task.Definition.Actions {
		if ea, ok := action.(taskmaster.ExecAction); ok {
			command = ea.Path
			break
		}
	}

	return taskInfo{exists: true, command: command}, nil
}

// createTask registers a new task using the taskmaster API.
func createTask(name, command, user string) error {
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

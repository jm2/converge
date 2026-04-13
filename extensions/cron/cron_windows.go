//go:build windows

package cron

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"

	"github.com/TsekNet/converge/extensions"
)

// Check queries the Windows Task Scheduler COM API for the named task.
func (c *Cron) Check(_ context.Context) (*extensions.State, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	info, err := getTaskInfo(c.Name)
	if err != nil {
		return nil, fmt.Errorf("query task %s: %w", c.Name, err)
	}

	if c.State == "absent" {
		if !info.exists {
			return &extensions.State{InSync: true}, nil
		}
		return &extensions.State{
			InSync: false,
			Changes: []extensions.Change{
				{Property: "task", From: c.Name, To: "", Action: "remove"},
			},
		}, nil
	}

	if !info.exists {
		return &extensions.State{
			InSync: false,
			Changes: []extensions.Change{
				{Property: "task", To: c.Name, Action: "add"},
			},
		}, nil
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

	return &extensions.State{InSync: len(changes) == 0, Changes: changes}, nil
}

// Apply creates or removes a Windows scheduled task via the Task Scheduler COM API.
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

// withTaskService initializes COM, creates an ITaskService, connects,
// and passes it to fn. Cleanup is handled on return.
func withTaskService(fn func(service *ole.IDispatch) error) error {
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		// S_FALSE (0x00000001) means COM was already initialized on this thread.
		var oleErr *ole.OleError
		if errors.As(err, &oleErr) && oleErr.Code() == 0x00000001 {
			// Safe to continue: COM already initialized.
		} else {
			return fmt.Errorf("CoInitializeEx: %w", err)
		}
	}
	defer ole.CoUninitialize()

	unknown, err := oleutil.CreateObject("Schedule.Service")
	if err != nil {
		return fmt.Errorf("create Schedule.Service: %w", err)
	}
	defer unknown.Release()

	service, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return fmt.Errorf("QueryInterface: %w", err)
	}
	defer service.Release()

	if _, err := oleutil.CallMethod(service, "Connect"); err != nil {
		return fmt.Errorf("ITaskService.Connect: %w", err)
	}

	return fn(service)
}

// taskInfo holds the properties of an existing scheduled task.
type taskInfo struct {
	exists  bool
	command string // Action Path from the first exec action
}

// getTaskInfo retrieves the task's existence and configured command.
func getTaskInfo(name string) (taskInfo, error) {
	var info taskInfo
	err := withTaskService(func(service *ole.IDispatch) error {
		folder, err := oleutil.CallMethod(service, "GetFolder", `\`)
		if err != nil {
			return fmt.Errorf("GetFolder: %w", err)
		}
		folderDisp := folder.ToIDispatch()
		defer folderDisp.Release()

		taskResult, err := oleutil.CallMethod(folderDisp, "GetTask", name)
		if err != nil {
			return nil // task does not exist
		}
		info.exists = true
		taskDisp := taskResult.ToIDispatch()
		defer taskDisp.Release()

		// task.Definition.Actions.Item(1).Path
		defResult, err := oleutil.GetProperty(taskDisp, "Definition")
		if err != nil {
			return nil // can't read definition, treat as exists-only
		}
		defDisp := defResult.ToIDispatch()
		defer defDisp.Release()

		actionsResult, err := oleutil.GetProperty(defDisp, "Actions")
		if err != nil {
			return nil
		}
		actionsDisp := actionsResult.ToIDispatch()
		defer actionsDisp.Release()

		actionResult, err := oleutil.GetProperty(actionsDisp, "Item", 1)
		if err != nil {
			return nil
		}
		actionDisp := actionResult.ToIDispatch()
		defer actionDisp.Release()

		pathResult, err := oleutil.GetProperty(actionDisp, "Path")
		if err != nil {
			return nil
		}
		info.command = pathResult.ToString()
		return nil
	})
	return info, err
}

// createTask registers a new task using the Task Scheduler 2.0 COM API.
func createTask(name, schedule, command, user string) error {
	return withTaskService(func(service *ole.IDispatch) error {
		// Get root folder
		folder, err := oleutil.CallMethod(service, "GetFolder", `\`)
		if err != nil {
			return fmt.Errorf("GetFolder: %w", err)
		}
		folderDisp := folder.ToIDispatch()
		defer folderDisp.Release()

		// Delete existing task if present (idempotent recreate)
		oleutil.CallMethod(folderDisp, "DeleteTask", name, 0)

		// Create new task definition
		taskDef, err := oleutil.CallMethod(service, "NewTask", 0)
		if err != nil {
			return fmt.Errorf("NewTask: %w", err)
		}
		taskDefDisp := taskDef.ToIDispatch()
		defer taskDefDisp.Release()

		// Set registration info
		regInfo, err := oleutil.GetProperty(taskDefDisp, "RegistrationInfo")
		if err != nil {
			return fmt.Errorf("RegistrationInfo: %w", err)
		}
		regInfoDisp := regInfo.ToIDispatch()
		defer regInfoDisp.Release()
		oleutil.PutProperty(regInfoDisp, "Description", "Managed by Converge: "+name)

		// Configure action (exec)
		actions, err := oleutil.GetProperty(taskDefDisp, "Actions")
		if err != nil {
			return fmt.Errorf("Actions: %w", err)
		}
		actionsDisp := actions.ToIDispatch()
		defer actionsDisp.Release()

		// TASK_ACTION_EXEC = 0
		action, err := oleutil.CallMethod(actionsDisp, "Create", 0)
		if err != nil {
			return fmt.Errorf("Actions.Create: %w", err)
		}
		actionDisp := action.ToIDispatch()
		defer actionDisp.Release()
		oleutil.PutProperty(actionDisp, "Path", command)

		// Configure principal (run-as user)
		principal, err := oleutil.GetProperty(taskDefDisp, "Principal")
		if err != nil {
			return fmt.Errorf("Principal: %w", err)
		}
		principalDisp := principal.ToIDispatch()
		defer principalDisp.Release()

		if user != "" {
			oleutil.PutProperty(principalDisp, "UserId", user)
		}
		// TASK_LOGON_SERVICE_ACCOUNT = 5
		oleutil.PutProperty(principalDisp, "LogonType", 5)

		// Register task
		// TASK_CREATE_OR_UPDATE = 6, TASK_LOGON_SERVICE_ACCOUNT = 5
		_, err = oleutil.CallMethod(folderDisp, "RegisterTaskDefinition",
			name,        // path
			taskDefDisp, // definition
			6,           // TASK_CREATE_OR_UPDATE
			nil,         // userId (from principal)
			nil,         // password
			5,           // logonType
		)
		if err != nil {
			return fmt.Errorf("RegisterTaskDefinition(%s): %w", name, err)
		}

		return nil
	})
}

// deleteTask removes a task from the root folder.
func deleteTask(name string) error {
	return withTaskService(func(service *ole.IDispatch) error {
		folder, err := oleutil.CallMethod(service, "GetFolder", `\`)
		if err != nil {
			return fmt.Errorf("GetFolder: %w", err)
		}
		folderDisp := folder.ToIDispatch()
		defer folderDisp.Release()

		if _, err := oleutil.CallMethod(folderDisp, "DeleteTask", name, 0); err != nil {
			return fmt.Errorf("DeleteTask(%s): %w", name, err)
		}
		return nil
	})
}

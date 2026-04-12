//go:build windows

package cron

import (
	"context"
	"fmt"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"

	"github.com/TsekNet/converge/extensions"
)

// Check queries the Windows Task Scheduler COM API for the named task.
func (c *Cron) Check(_ context.Context) (*extensions.State, error) {
	exists, err := taskExists(c.Name)
	if err != nil {
		return nil, fmt.Errorf("query task %s: %w", c.Name, err)
	}

	if c.State == "absent" {
		if !exists {
			return &extensions.State{InSync: true}, nil
		}
		return &extensions.State{
			InSync: false,
			Changes: []extensions.Change{
				{Property: "task", From: c.Name, To: "", Action: "remove"},
			},
		}, nil
	}

	if !exists {
		return &extensions.State{
			InSync: false,
			Changes: []extensions.Change{
				{Property: "task", To: c.Name, Action: "add"},
			},
		}, nil
	}

	return &extensions.State{InSync: true}, nil
}

// Apply creates or removes a Windows scheduled task via the Task Scheduler COM API.
func (c *Cron) Apply(_ context.Context) (*extensions.Result, error) {
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
		if err.(*ole.OleError).Code() != 0x00000001 {
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

// taskExists checks whether a task with the given name exists in the root folder.
func taskExists(name string) (bool, error) {
	var exists bool
	err := withTaskService(func(service *ole.IDispatch) error {
		folder, err := oleutil.CallMethod(service, "GetFolder", `\`)
		if err != nil {
			return fmt.Errorf("GetFolder: %w", err)
		}
		folderDisp := folder.ToIDispatch()
		defer folderDisp.Release()

		_, err = oleutil.CallMethod(folderDisp, "GetTask", name)
		exists = err == nil
		return nil
	})
	return exists, err
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

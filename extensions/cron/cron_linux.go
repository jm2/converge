//go:build linux

package cron

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/TsekNet/converge/extensions"
)

// cronDir is the drop-in directory scanned by Linux cron. Each managed task
// lives in its own file there.
const cronDir = "/etc/cron.d"

// cronFilePath returns the path to the cron.d file for this task.
func (c *Cron) cronFilePath() string {
	return filepath.Join(cronDir, "converge-"+sanitizeName(c.Name))
}

// Check reads the cron.d file to determine if the task exists with correct content.
func (c *Cron) Check(_ context.Context) (*extensions.State, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	path := c.cronFilePath()
	wantLine := c.cronLine()

	data, err := c.fsys().ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if c.State == "absent" {
			return &extensions.State{InSync: true}, nil
		}
		return &extensions.State{
			InSync: false,
			Changes: []extensions.Change{
				{Property: "cron", To: wantLine, Action: "add"},
			},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	found := containsLine(string(data), wantLine)

	if c.State == "absent" {
		if !found {
			return &extensions.State{InSync: true}, nil
		}
		return &extensions.State{
			InSync: false,
			Changes: []extensions.Change{
				{Property: "cron", From: wantLine, To: "", Action: "remove"},
			},
		}, nil
	}

	if found {
		return &extensions.State{InSync: true}, nil
	}

	return &extensions.State{
		InSync: false,
		Changes: []extensions.Change{
			{Property: "cron", To: wantLine, Action: "modify"},
		},
	}, nil
}

// Apply creates, updates, or removes the cron.d file.
func (c *Cron) Apply(_ context.Context) (*extensions.Result, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	path := c.cronFilePath()

	if c.State == "absent" {
		if err := c.fsys().Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("remove %s: %w", path, err)
		}
		return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "removed"}, nil
	}

	if err := c.fsys().MkdirAll(cronDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", cronDir, err)
	}

	content := c.cronLine() + "\n"
	if err := c.fsys().WriteFile(path, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("write %s: %w", path, err)
	}

	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "created"}, nil
}

// sanitizeName replaces non-filename-safe characters with dashes.
func sanitizeName(name string) string {
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-")
	return replacer.Replace(name)
}

//go:build linux || darwin

package cron

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TsekNet/converge/extensions"
)

const cronDir = "/etc/cron.d"

// cronLine returns the formatted cron line with a tag comment.
func (c *Cron) cronLine() string {
	user := c.User
	if user == "" {
		user = "root"
	}
	return fmt.Sprintf("%s %s %s # converge:%s", c.Schedule, user, c.Command, c.Name)
}

// cronFilePath returns the path to the cron.d file for this task.
func (c *Cron) cronFilePath() string {
	return filepath.Join(cronDir, "converge-"+sanitizeName(c.Name))
}

// Check reads the cron.d file to determine if the task exists with correct content.
func (c *Cron) Check(_ context.Context) (*extensions.State, error) {
	path := c.cronFilePath()
	wantLine := c.cronLine()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
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
	path := c.cronFilePath()

	if c.State == "absent" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("remove %s: %w", path, err)
		}
		return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "removed"}, nil
	}

	if err := os.MkdirAll(cronDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", cronDir, err)
	}

	content := c.cronLine() + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("write %s: %w", path, err)
	}

	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "created"}, nil
}

// containsLine checks if data contains the exact line.
func containsLine(data, line string) bool {
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == strings.TrimSpace(line) {
			return true
		}
	}
	return false
}

// sanitizeName replaces non-filename-safe characters with dashes.
func sanitizeName(name string) string {
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-")
	return replacer.Replace(name)
}

//go:build darwin

package cron

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/TsekNet/converge/extensions"
)

// systemCrontab is the system-wide crontab table that macOS cron reads. Unlike
// Linux, macOS cron does not scan /etc/cron.d, so converge manages individual
// tagged rows inside this shared file instead of writing one file per task.
// Each managed row ends with a "# converge:<name>" tag so it can be located,
// replaced, or removed without disturbing other entries.
const systemCrontab = "/etc/crontab"

// tag returns the comment marker that identifies this task's row.
func (c *Cron) tag() string {
	return "# converge:" + c.Name
}

// Check reads the system crontab and reports whether this task's row is present
// with the desired content.
func (c *Cron) Check(_ context.Context) (*extensions.State, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	wantLine := c.cronLine()

	data, err := c.fsys().ReadFile(systemCrontab)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", systemCrontab, err)
	}
	content := string(data)
	found := containsLine(content, wantLine)
	tagged := containsTag(content, c.tag())

	if c.State == "absent" {
		if !tagged {
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

	action := "add"
	if tagged {
		action = "modify"
	}
	return &extensions.State{
		InSync: false,
		Changes: []extensions.Change{
			{Property: "cron", To: wantLine, Action: action},
		},
	}, nil
}

// Apply inserts, updates, or removes this task's row in the system crontab,
// preserving all other lines in the file.
func (c *Cron) Apply(_ context.Context) (*extensions.Result, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	data, err := c.fsys().ReadFile(systemCrontab)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", systemCrontab, err)
	}

	// Drop any existing row for this task so updates are idempotent.
	lines := removeTaggedLines(string(data), c.tag())

	if c.State == "absent" {
		if err := c.fsys().WriteFile(systemCrontab, []byte(joinLines(lines)), 0644); err != nil {
			return nil, fmt.Errorf("write %s: %w", systemCrontab, err)
		}
		return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "removed"}, nil
	}

	lines = append(lines, c.cronLine())
	if err := c.fsys().WriteFile(systemCrontab, []byte(joinLines(lines)), 0644); err != nil {
		return nil, fmt.Errorf("write %s: %w", systemCrontab, err)
	}
	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "created"}, nil
}

// containsTag reports whether any line in data is tagged with tag (matched as a
// trailing marker so that "converge:foo" does not match "converge:foobar").
func containsTag(data, tag string) bool {
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		if strings.HasSuffix(strings.TrimSpace(scanner.Text()), tag) {
			return true
		}
	}
	return false
}

// removeTaggedLines returns data's lines with any line tagged by tag removed.
func removeTaggedLines(data, tag string) []string {
	var out []string
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasSuffix(strings.TrimSpace(line), tag) {
			continue
		}
		out = append(out, line)
	}
	return out
}

// joinLines renders lines back into file content with a trailing newline.
func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

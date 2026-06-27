//go:build linux || darwin

package cron

import (
	"bufio"
	"fmt"
	"strings"
)

// cronLine returns the formatted cron line with a tag comment. The line carries
// the user field so it is valid both as an /etc/cron.d entry (Linux) and as a
// row in the system crontab table /etc/crontab (macOS).
func (c *Cron) cronLine() string {
	user := c.User
	if user == "" {
		user = "root"
	}
	return fmt.Sprintf("%s %s %s # converge:%s", c.Schedule, user, c.Command, c.Name)
}

// containsLine reports whether data contains the exact line (ignoring
// surrounding whitespace).
func containsLine(data, line string) bool {
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == strings.TrimSpace(line) {
			return true
		}
	}
	return false
}

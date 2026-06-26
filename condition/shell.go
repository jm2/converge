package condition

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/TsekNet/converge/internal/shell"
)

// Shell returns a Condition that runs a command in the platform-default shell
// (bash on Linux/macOS, powershell on Windows). Exit code 0 = met.
//
// Chain .Match(expected) for output-based matching, .In(shell) for an explicit
// shell override.
//
//	condition.Shell("pgrep nginx")                                    // exit 0 = met
//	condition.Shell("cat /etc/hostname").Match("web01")               // output match
//	condition.Shell("(Get-WindowsOptionalFeature ...).State").Match("Enabled")
//	condition.Shell("Get-Feature ...").In("pwsh").Match("Enabled")    // explicit shell
func Shell(command string) *shellCondition {
	return &shellCondition{
		shellName: shell.Auto,
		command:   command,
	}
}

// shellTimeout bounds a single Met() evaluation so a slow or blocking command
// cannot stall the caller indefinitely. The daemon evaluates conditions
// synchronously while starting watchers, so an unbounded command would hang the
// whole daemon. It is a package var so tests can shorten it.
var shellTimeout = 30 * time.Second

type shellCondition struct {
	shellName      string
	command        string
	expectedOutput string
	matchSet       bool // true once Match was called (so Match("") is meaningful)
}

// In sets an explicit shell. Accepts "powershell", "pwsh", "cmd", "bash",
// "sh", or a custom binary path.
func (c *shellCondition) In(shellName string) *shellCondition {
	c.shellName = shellName
	return c
}

// Match sets the expected trimmed stdout. Once called, the condition is met
// when the command's trimmed output equals the trimmed expected value (instead
// of exit code 0). Calling Match("") is meaningful: it asserts empty output.
func (c *shellCondition) Match(expected string) *shellCondition {
	c.expectedOutput = expected
	c.matchSet = true
	return c
}

func (c *shellCondition) Met(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, shellTimeout)
	defer cancel()
	stdout, err := shell.Run(ctx, c.shellName, c.command, nil)

	if c.matchSet {
		if err != nil {
			return false, err
		}
		return strings.TrimSpace(stdout) == strings.TrimSpace(c.expectedOutput), nil
	}

	return err == nil, nil
}

func (c *shellCondition) Wait(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if met, _ := c.Met(ctx); met {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *shellCondition) String() string {
	if c.matchSet {
		return fmt.Sprintf("shell %s: %s == %q", c.shellName, shell.Truncate(c.command, 40), c.expectedOutput)
	}
	return fmt.Sprintf("shell %s: %s", c.shellName, shell.Truncate(c.command, 40))
}

package user

import (
	"context"
	"fmt"
	"os/user"
	"strings"

	"github.com/TsekNet/converge/extensions"
)

// Opts configures a User resource.
type Opts struct {
	Groups   []string
	Shell    string
	Home     string
	System   bool
	Critical bool
}

// User ensures a local user account exists with the specified groups and shell.
// Apply is platform-specific (useradd on Linux, dscl on macOS, net user on Windows).
type User struct {
	Name     string
	Groups   []string
	Shell    string
	Home     string
	System   bool
	Critical bool
}

func New(name string, opts Opts) *User {
	return &User{
		Name:     name,
		Groups:   opts.Groups,
		Shell:    opts.Shell,
		Home:     opts.Home,
		System:   opts.System,
		Critical: opts.Critical,
	}
}

func (u *User) ID() string       { return fmt.Sprintf("user:%s", u.Name) }
func (u *User) String() string   { return fmt.Sprintf("User %s", u.Name) }
func (u *User) IsCritical() bool { return u.Critical }

func (u *User) Check(_ context.Context) (*extensions.State, error) {
	existing, err := lookupUser(u.Name)
	if err != nil {
		var changes []extensions.Change
		changes = append(changes, extensions.Change{
			Property: "user", To: u.Name, Action: "add",
		})
		if len(u.Groups) > 0 {
			changes = append(changes, extensions.Change{
				Property: "groups", To: strings.Join(u.Groups, ","), Action: "add",
			})
		}
		if u.Shell != "" {
			changes = append(changes, extensions.Change{
				Property: "shell", To: u.Shell, Action: "add",
			})
		}
		return &extensions.State{InSync: false, Changes: changes}, nil
	}

	var changes []extensions.Change

	if u.Shell != "" {
		currentShell := shellForUser(existing)
		if currentShell != "" && currentShell != u.Shell {
			changes = append(changes, extensions.Change{
				Property: "shell", From: currentShell, To: u.Shell, Action: "modify",
			})
		}
	}

	// Diff desired group membership against current membership. Membership is
	// read via os/user (which works on Linux, macOS, and Windows); if it cannot
	// be read we skip the diff rather than report a false positive.
	if len(u.Groups) > 0 {
		if current, err := userGroups(existing); err == nil {
			if missing := missingGroups(u.Groups, current); len(missing) > 0 {
				changes = append(changes, extensions.Change{
					Property: "groups", To: strings.Join(missing, ","), Action: "add",
				})
			}
		}
	}

	return &extensions.State{InSync: len(changes) == 0, Changes: changes}, nil
}

func lookupUser(name string) (*user.User, error) {
	return user.Lookup(name)
}

// userGroups returns the names of the groups the user is a member of. It
// resolves group IDs to names where possible, falling back to the raw ID.
func userGroups(existing *user.User) ([]string, error) {
	ids, err := existing.GroupIds()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		if g, err := user.LookupGroupId(id); err == nil {
			names = append(names, g.Name)
		} else {
			names = append(names, id)
		}
	}
	return names, nil
}

// missingGroups returns the desired groups that are not present in current.
func missingGroups(desired, current []string) []string {
	if len(desired) == 0 {
		return nil
	}
	have := make(map[string]bool, len(current))
	for _, g := range current {
		have[g] = true
	}
	var missing []string
	for _, g := range desired {
		if !have[g] {
			missing = append(missing, g)
		}
	}
	return missing
}

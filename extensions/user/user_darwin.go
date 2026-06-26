//go:build darwin

package user

import (
	"context"
	"fmt"
	"os/exec"
	osuser "os/user"
	"strconv"
	"strings"

	"github.com/TsekNet/converge/extensions"
)

// Apply creates or modifies the user via macOS Directory Services (dscl).
func (u *User) Apply(ctx context.Context) (*extensions.Result, error) {
	_, err := lookupUser(u.Name)
	if err != nil {
		return u.createUser(ctx)
	}
	return u.modifyUser(ctx)
}

func (u *User) createUser(ctx context.Context) (*extensions.Result, error) {
	userPath := fmt.Sprintf("/Users/%s", u.Name)

	if out, err := exec.CommandContext(ctx, "dscl", ".", "-create", userPath).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("dscl create %s: %s: %w", u.Name, strings.TrimSpace(string(out)), err)
	}

	uid, err := u.nextUID(ctx)
	if err != nil {
		return nil, fmt.Errorf("dscl find uid %s: %w", u.Name, err)
	}

	shell := u.Shell
	if shell == "" {
		shell = "/bin/bash"
	}
	home := u.Home
	if home == "" {
		home = userPath
	}

	// Set the standard attributes a usable account requires. Without these the
	// record is incomplete and login/lookup misbehaves.
	attrs := [][2]string{
		{"UserShell", shell},
		{"RealName", u.Name},
		{"UniqueID", strconv.Itoa(uid)},
		{"PrimaryGroupID", "20"}, // staff
		{"NFSHomeDirectory", home},
	}
	if u.System {
		// System accounts are hidden from the login window and use a low UID
		// range (selected by nextUID).
		attrs = append(attrs, [2]string{"IsHidden", "1"})
	}
	for _, a := range attrs {
		if out, err := exec.CommandContext(ctx, "dscl", ".", "-create", userPath, a[0], a[1]).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("dscl create %s %s: %s: %w", u.Name, a[0], strings.TrimSpace(string(out)), err)
		}
	}

	if err := u.addToGroups(ctx); err != nil {
		return nil, err
	}

	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "Created"}, nil
}

func (u *User) modifyUser(ctx context.Context) (*extensions.Result, error) {
	changed := false
	if u.Shell != "" {
		cmd := exec.CommandContext(ctx, "dscl", ".", "-create", fmt.Sprintf("/Users/%s", u.Name), "UserShell", u.Shell)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("dscl modify shell %s: %s: %w", u.Name, strings.TrimSpace(string(out)), err)
		}
		changed = true
	}
	if len(u.Groups) > 0 {
		if err := u.addToGroups(ctx); err != nil {
			return nil, err
		}
		changed = true
	}
	if changed {
		return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "Modified"}, nil
	}
	return &extensions.Result{Changed: false, Status: extensions.StatusOK, Message: "OK"}, nil
}

// addToGroups appends the user to each desired group's membership. dscl -append
// does not create duplicate entries, so this is safe to re-run.
func (u *User) addToGroups(ctx context.Context) error {
	for _, group := range u.Groups {
		out, err := exec.CommandContext(ctx, "dscl", ".", "-append", fmt.Sprintf("/Groups/%s", group), "GroupMembership", u.Name).CombinedOutput()
		if err != nil {
			return fmt.Errorf("dscl append group %s %s: %s: %w", group, u.Name, strings.TrimSpace(string(out)), err)
		}
	}
	return nil
}

// nextUID picks the next free UID in the range appropriate for the account
// class. System accounts use IDs below 500; regular accounts use 501+.
func (u *User) nextUID(ctx context.Context) (int, error) {
	out, err := exec.CommandContext(ctx, "dscl", ".", "-list", "/Users", "UniqueID").Output()
	if err != nil {
		return 0, err
	}
	max := 500
	if u.System {
		max = 200
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		id, err := strconv.Atoi(fields[len(fields)-1])
		if err != nil {
			continue
		}
		// Only consider IDs within the target class to avoid collisions.
		if u.System && id >= 500 {
			continue
		}
		if !u.System && id < 500 {
			continue
		}
		if id > max {
			max = id
		}
	}
	return max + 1, nil
}

// shellForUser reads the login shell from the local directory via dscl.
func shellForUser(u *osuser.User) string {
	cmd := exec.Command("dscl", ".", "-read", fmt.Sprintf("/Users/%s", u.Name), "UserShell")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), " ", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

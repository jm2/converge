//go:build linux

package sysctl

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/TsekNet/converge/extensions"
)

func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

// procSysBase is the root of the sysctl pseudo-filesystem. It is a var (not a
// const) solely so tests can redirect it at a real temp directory to exercise
// the inotify-based Watch without writing to /proc/sys (which requires root).
var procSysBase = "/proc/sys"

// Check reads the live kernel value from /proc/sys/<key> and compares it.
func (s *Sysctl) Check(_ context.Context) (*extensions.State, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}

	current, err := s.read()
	if err != nil {
		return nil, fmt.Errorf("read sysctl %s: %w", s.Key, err)
	}

	if current == s.Value {
		return &extensions.State{InSync: true}, nil
	}

	return &extensions.State{
		InSync: false,
		Changes: []extensions.Change{{
			Property: s.Key,
			From:     current,
			To:       s.Value,
			Action:   "modify",
		}},
	}, nil
}

// Apply writes the value to /proc/sys/ for immediate effect, then optionally
// persists it to /etc/sysctl.d/99-converge.conf so it survives reboots.
func (s *Sysctl) Apply(_ context.Context) (*extensions.Result, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}

	p := keyToPath(s.Key)
	if err := s.fsys().WriteFile(p, []byte(s.Value+"\n"), 0644); err != nil {
		return nil, fmt.Errorf("write sysctl %s: %w", s.Key, err)
	}

	if s.Persist {
		if err := s.writePersist(); err != nil {
			return nil, fmt.Errorf("persist sysctl %s: %w", s.Key, err)
		}
	}

	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "set"}, nil
}

func (s *Sysctl) read() (string, error) {
	data, err := s.fsys().ReadFile(keyToPath(s.Key))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (s *Sysctl) writePersist() error {
	dir := "/etc/sysctl.d"
	if err := s.fsys().MkdirAll(dir, 0755); err != nil {
		return err
	}

	confPath := filepath.Join(dir, "99-converge.conf")
	prefix := s.Key + " = "
	newLine := fmt.Sprintf("%s = %s", s.Key, s.Value)

	existing, err := s.fsys().ReadFile(confPath)
	if err != nil && !isNotExist(err) {
		return fmt.Errorf("read %s: %w", confPath, err)
	}

	var lines []string
	if len(existing) > 0 {
		lines = strings.Split(strings.TrimRight(string(existing), "\n"), "\n")
	}

	found := false
	for i, l := range lines {
		if strings.HasPrefix(l, prefix) {
			lines[i] = newLine
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, newLine)
	}

	content := strings.Join(lines, "\n") + "\n"
	return s.fsys().WriteFile(confPath, []byte(content), 0644)
}

// keyToPath converts "net.ipv4.ip_forward" to "/proc/sys/net/ipv4/ip_forward".
func keyToPath(key string) string {
	return filepath.Join(procSysBase, strings.ReplaceAll(key, ".", "/"))
}

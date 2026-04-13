//go:build linux

package kernelmodule

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/TsekNet/converge/extensions"
)

const (
	procModules   = "/proc/modules"
	modprobeDir   = "/etc/modprobe.d"
	blacklistFile = "converge-blacklist.conf"
)

// Check reads /proc/modules for loaded state and /etc/modprobe.d/ for blacklist state.
func (k *KernelModule) Check(_ context.Context) (*extensions.State, error) {
	if err := k.validate(); err != nil {
		return nil, err
	}

	loaded, err := isModuleLoaded(k.Module)
	if err != nil {
		return nil, fmt.Errorf("check module %s: %w", k.Module, err)
	}

	blacklisted, err := isModuleBlacklisted(k.Module)
	if err != nil {
		return nil, fmt.Errorf("check blacklist %s: %w", k.Module, err)
	}

	switch k.State {
	case Loaded:
		if loaded && !blacklisted {
			return &extensions.State{InSync: true}, nil
		}
		var changes []extensions.Change
		if !loaded {
			changes = append(changes, extensions.Change{
				Property: "state", From: "unloaded", To: "loaded", Action: "add",
			})
		}
		if blacklisted {
			changes = append(changes, extensions.Change{
				Property: "blacklist", From: "blacklisted", To: "allowed", Action: "remove",
			})
		}
		return &extensions.State{InSync: false, Changes: changes}, nil

	case Blacklisted:
		if !loaded && blacklisted {
			return &extensions.State{InSync: true}, nil
		}
		var changes []extensions.Change
		if loaded {
			changes = append(changes, extensions.Change{
				Property: "state", From: "loaded", To: "unloaded", Action: "remove",
			})
		}
		if !blacklisted {
			changes = append(changes, extensions.Change{
				Property: "blacklist", From: "allowed", To: "blacklisted", Action: "add",
			})
		}
		return &extensions.State{InSync: false, Changes: changes}, nil

	default:
		return nil, fmt.Errorf("unknown state %q for module %s", k.State, k.Module)
	}
}

// Apply loads or blacklists the module.
func (k *KernelModule) Apply(_ context.Context) (*extensions.Result, error) {
	if err := k.validate(); err != nil {
		return nil, err
	}

	switch k.State {
	case Loaded:
		if err := removeFromBlacklist(k.Module); err != nil {
			return nil, fmt.Errorf("remove blacklist %s: %w", k.Module, err)
		}
		loaded, err := isModuleLoaded(k.Module)
		if err != nil {
			return nil, fmt.Errorf("check module %s: %w", k.Module, err)
		}
		if !loaded {
			if err := loadModule(k.Module); err != nil {
				return nil, fmt.Errorf("load module %s: %w", k.Module, err)
			}
		}
		return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "loaded"}, nil

	case Blacklisted:
		loaded, err := isModuleLoaded(k.Module)
		if err != nil {
			return nil, fmt.Errorf("check module %s: %w", k.Module, err)
		}
		if loaded {
			if err := unloadModule(k.Module); err != nil {
				return nil, fmt.Errorf("unload module %s: %w", k.Module, err)
			}
		}
		if err := addToBlacklist(k.Module); err != nil {
			return nil, fmt.Errorf("blacklist module %s: %w", k.Module, err)
		}
		return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "blacklisted"}, nil

	default:
		return nil, fmt.Errorf("unknown state %q for module %s", k.State, k.Module)
	}
}

// isModuleLoaded checks /proc/modules for the module name.
func isModuleLoaded(module string) (bool, error) {
	f, err := os.Open(procModules)
	if err != nil {
		return false, fmt.Errorf("open %s: %w", procModules, err)
	}
	defer f.Close()

	// Normalize hyphens to underscores: the kernel stores module names with
	// underscores in /proc/modules, but users may specify either form.
	normalized := strings.ReplaceAll(module, "-", "_")

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) > 0 && fields[0] == normalized {
			return true, nil
		}
	}
	return false, scanner.Err()
}

// isModuleBlacklisted checks for a blacklist entry in /etc/modprobe.d/converge-blacklist.conf.
func isModuleBlacklisted(module string) (bool, error) {
	path := filepath.Join(modprobeDir, blacklistFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	line := fmt.Sprintf("blacklist %s", module)
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) == line {
			return true, nil
		}
	}
	return false, nil
}

// loadModule loads a kernel module using /sbin/modprobe via finit_module.
// We use modprobe here because it handles module dependencies, which
// raw finit_module does not.
func loadModule(module string) error {
	// Write to /sys/module/<module> is not possible for loading.
	// modprobe is the standard mechanism and handles dependencies.
	return modprobe(module, false)
}

// unloadModule removes a loaded kernel module.
func unloadModule(module string) error {
	return modprobe(module, true)
}

// modprobe runs modprobe to load or remove a module.
func modprobe(module string, remove bool) error {
	args := []string{module}
	if remove {
		args = []string{"-r", module}
	}

	cmd := newCommand("/sbin/modprobe", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("modprobe %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}

// addToBlacklist adds a blacklist entry to /etc/modprobe.d/converge-blacklist.conf.
func addToBlacklist(module string) error {
	if err := os.MkdirAll(modprobeDir, 0755); err != nil {
		return err
	}

	path := filepath.Join(modprobeDir, blacklistFile)
	line := fmt.Sprintf("blacklist %s", module)

	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	for _, l := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(l) == line {
			return nil // already blacklisted
		}
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	_, writeErr := fmt.Fprintf(f, "%s\n", line)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

// removeFromBlacklist removes a module's blacklist entry from converge-blacklist.conf.
func removeFromBlacklist(module string) error {
	path := filepath.Join(modprobeDir, blacklistFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	line := fmt.Sprintf("blacklist %s", module)
	var filtered []string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) != line {
			filtered = append(filtered, l)
		}
	}

	return os.WriteFile(path, []byte(strings.Join(filtered, "\n")), 0644)
}

package pkg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TsekNet/converge/extensions"
)

// Repository manages a package repository source. For apt, it writes to
// /etc/apt/sources.list.d/. For dnf/yum, it writes to /etc/yum.repos.d/.
type Repository struct {
	Name         string // repo identifier (becomes filename)
	URI          string // repository URL
	Distribution string // apt: distribution (e.g. "jammy"), dnf/yum: unused
	Components   string // apt: components (e.g. "main"), dnf/yum: unused
	GPGKey       string // GPG key URL (apt: written to sources, dnf/yum: gpgkey= line)
	Enabled      bool
	State        string // "present" or "absent"
	ManagerName  string // "apt", "dnf", or "yum"
	Critical     bool
}

// RepositoryOpts holds configurable fields for a Repository resource.
type RepositoryOpts struct {
	URI          string
	Distribution string // apt: distribution (e.g. "jammy"), dnf/yum: unused
	Components   string // apt: components (e.g. "main"), dnf/yum: unused
	GPGKey       string // GPG key URL
	Enabled      bool
	State        string // "present" or "absent"
	ManagerName  string // "apt", "dnf", or "yum"
	Critical     bool
}

// NewRepository creates a Repository resource. State defaults to "present"
// when zero-valued. Enabled is taken directly from opts; callers that want
// the repository enabled must set Enabled: true explicitly.
func NewRepository(name string, opts RepositoryOpts) *Repository {
	state := opts.State
	if state == "" {
		state = "present"
	}
	return &Repository{
		Name:         name,
		URI:          opts.URI,
		Distribution: opts.Distribution,
		Components:   opts.Components,
		GPGKey:       opts.GPGKey,
		Enabled:      opts.Enabled,
		State:        state,
		ManagerName:  opts.ManagerName,
		Critical:     opts.Critical,
	}
}

func (r *Repository) ID() string       { return fmt.Sprintf("repository:%s", r.Name) }
func (r *Repository) String() string   { return fmt.Sprintf("Repository %s (%s)", r.Name, r.ManagerName) }
func (r *Repository) IsCritical() bool { return r.Critical }

func (r *Repository) repoFilePath() string {
	switch r.ManagerName {
	case "apt":
		return filepath.Join("/etc/apt/sources.list.d", r.Name+".list")
	case "dnf", "yum":
		return filepath.Join("/etc/yum.repos.d", r.Name+".repo")
	default:
		return ""
	}
}

func (r *Repository) repoContent() string {
	switch r.ManagerName {
	case "apt":
		line := fmt.Sprintf("deb %s %s %s", r.URI, r.Distribution, r.Components)
		return strings.TrimSpace(line) + "\n"
	case "dnf", "yum":
		var b strings.Builder
		fmt.Fprintf(&b, "[%s]\n", r.Name)
		fmt.Fprintf(&b, "name=%s\n", r.Name)
		fmt.Fprintf(&b, "baseurl=%s\n", r.URI)
		if r.Enabled {
			fmt.Fprintln(&b, "enabled=1")
		} else {
			fmt.Fprintln(&b, "enabled=0")
		}
		if r.GPGKey != "" {
			fmt.Fprintln(&b, "gpgcheck=1")
			fmt.Fprintf(&b, "gpgkey=%s\n", r.GPGKey)
		} else {
			fmt.Fprintln(&b, "gpgcheck=0")
		}
		return b.String()
	default:
		return ""
	}
}

// Check compares the repo file on disk against desired state.
func (r *Repository) Check(_ context.Context) (*extensions.State, error) {
	path := r.repoFilePath()
	if path == "" {
		return nil, fmt.Errorf("unsupported package manager for repositories: %q", r.ManagerName)
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if r.State == "absent" {
			return &extensions.State{InSync: true}, nil
		}
		return &extensions.State{
			InSync: false,
			Changes: []extensions.Change{
				{Property: "repository", To: r.Name, Action: "add"},
			},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	if r.State == "absent" {
		return &extensions.State{
			InSync: false,
			Changes: []extensions.Change{
				{Property: "repository", From: r.Name, To: "", Action: "remove"},
			},
		}, nil
	}

	desired := r.repoContent()
	if string(data) == desired {
		return &extensions.State{InSync: true}, nil
	}

	return &extensions.State{
		InSync: false,
		Changes: []extensions.Change{
			{Property: "content", From: truncateStr(string(data), 60), To: truncateStr(desired, 60), Action: "modify"},
		},
	}, nil
}

// Apply writes, updates, or removes the repository file.
func (r *Repository) Apply(_ context.Context) (*extensions.Result, error) {
	path := r.repoFilePath()
	if path == "" {
		return nil, fmt.Errorf("unsupported package manager for repositories: %q", r.ManagerName)
	}

	if r.State == "absent" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("remove %s: %w", path, err)
		}
		return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "removed"}, nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	if err := os.WriteFile(path, []byte(r.repoContent()), 0644); err != nil {
		return nil, fmt.Errorf("write %s: %w", path, err)
	}

	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "configured"}, nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

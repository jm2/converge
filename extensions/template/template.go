// Package template manages files rendered from Go text/template strings.
// The template is evaluated at Check time to produce the desired content,
// then compared against the file on disk. Apply writes the rendered output.
package template

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"text/template"

	"github.com/TsekNet/converge/extensions"
	"github.com/TsekNet/converge/internal/shell"
)

// Template renders a Go text/template to a file and ensures the file
// matches the rendered output. Variables are provided via the Vars map.
type Template struct {
	Path     string
	Source   string            // Go text/template source
	Vars     map[string]string // template variables
	Mode     fs.FileMode
	Owner    string
	Group    string
	Critical bool
	FS       extensions.FS
}

// Opts holds configurable fields for a Template resource.
type Opts struct {
	Source   string            // Go text/template source
	Vars     map[string]string // template variables
	Mode     fs.FileMode
	Owner    string
	Group    string
	Critical bool
	FS       extensions.FS
}

// New creates a Template with required fields.
func New(path string, opts Opts) *Template {
	return &Template{
		Path:     path,
		Source:   opts.Source,
		Vars:     opts.Vars,
		Mode:     opts.Mode,
		Owner:    opts.Owner,
		Group:    opts.Group,
		Critical: opts.Critical,
		FS:       opts.FS,
	}
}

// fsys returns the configured FS, falling back to OSFS when nil.
func (t *Template) fsys() extensions.FS { return extensions.RealFS(t.FS) }

func (t *Template) ID() string       { return fmt.Sprintf("template:%s", t.Path) }
func (t *Template) String() string   { return fmt.Sprintf("Template %s", t.Path) }
func (t *Template) IsCritical() bool { return t.Critical }

// render executes the template and returns the rendered bytes.
func (t *Template) render() (string, error) {
	tmpl, err := template.New(filepath.Base(t.Path)).Option("missingkey=error").Parse(t.Source)
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", t.Path, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, t.Vars); err != nil {
		return "", fmt.Errorf("execute template %s: %w", t.Path, err)
	}
	return buf.String(), nil
}

// Check compares the rendered template output against the file on disk.
func (t *Template) Check(_ context.Context) (*extensions.State, error) {
	rendered, err := t.render()
	if err != nil {
		return nil, err
	}

	absPath, err := filepath.Abs(t.Path)
	if err != nil {
		return nil, fmt.Errorf("invalid path %q: %w", t.Path, err)
	}

	info, err := t.fsys().Stat(absPath)
	if errors.Is(err, fs.ErrNotExist) {
		return &extensions.State{
			InSync: false,
			Changes: []extensions.Change{
				{Property: "state", To: "create", Action: "add"},
				{Property: "content", To: shell.Truncate(rendered, 80), Action: "add"},
			},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", absPath, err)
	}

	var changes []extensions.Change

	existing, err := t.fsys().ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", absPath, err)
	}
	if string(existing) != rendered {
		changes = append(changes, extensions.Change{
			Property: "content",
			From:     shell.Truncate(string(existing), 80),
			To:       shell.Truncate(rendered, 80),
			Action:   "modify",
		})
	}

	if t.Mode != 0 && info.Mode().Perm() != t.Mode {
		changes = append(changes, extensions.Change{
			Property: "mode",
			From:     fmt.Sprintf("%04o", info.Mode().Perm()),
			To:       fmt.Sprintf("%04o", t.Mode),
			Action:   "modify",
		})
	}

	ownerChange, err := extensions.OwnershipChange(t.fsys(), absPath, t.Owner, t.Group)
	if err != nil {
		return nil, fmt.Errorf("check ownership %s: %w", absPath, err)
	}
	if ownerChange != nil {
		changes = append(changes, *ownerChange)
	}

	return &extensions.State{InSync: len(changes) == 0, Changes: changes}, nil
}

// Apply writes the rendered template to disk.
func (t *Template) Apply(_ context.Context) (*extensions.Result, error) {
	rendered, err := t.render()
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(t.Path)
	if err := t.fsys().MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	perm := t.Mode
	if perm == 0 {
		perm = 0644
	}
	if err := t.fsys().WriteFile(t.Path, []byte(rendered), perm); err != nil {
		return nil, fmt.Errorf("write %s: %w", t.Path, err)
	}

	if t.Mode != 0 {
		if err := t.fsys().Chmod(t.Path, t.Mode); err != nil {
			return nil, fmt.Errorf("chmod %s: %w", t.Path, err)
		}
	}

	if err := extensions.ApplyOwnership(t.fsys(), t.Path, t.Owner, t.Group); err != nil {
		return nil, fmt.Errorf("chown %s: %w", t.Path, err)
	}

	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "rendered"}, nil
}

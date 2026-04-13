package dsl

import (
	"errors"
	"fmt"

	"github.com/TsekNet/converge/condition"
	"github.com/TsekNet/converge/extensions"
	extpkg "github.com/TsekNet/converge/extensions/pkg"
	"github.com/TsekNet/converge/internal/graph"
	"github.com/TsekNet/converge/internal/platform"
)

// nodeMeta is the internal representation passed to addResource.
// Each DSL method constructs this from the flat Opts fields.
type nodeMeta struct {
	Noop      bool
	Retry     int
	Limit     float64
	AutoEdge  *bool
	AutoGroup *bool
	Condition extensions.Condition
}

func (m nodeMeta) toGraph() graph.NodeMeta {
	return graph.NodeMeta{
		Noop:      m.Noop,
		Retry:     m.Retry,
		Limit:     m.Limit,
		AutoEdge:  m.AutoEdge,
		AutoGroup: m.AutoGroup,
		Condition: m.Condition,
	}
}

// Run is the context passed to blueprints for declaring resources.
// Errors during resource declaration are accumulated, not panicked.
type Run struct {
	graph    *graph.Graph
	platform platform.Info
	app      *App
	errs     []error
}

func newRun(app *App) *Run {
	return &Run{
		graph:    graph.New(),
		platform: platform.Detect(),
		app:      app,
	}
}

func (r *Run) addResource(ext extensions.Extension, m nodeMeta) {
	if err := r.graph.AddNode(ext); err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s: %v", ext.ID(), err))
		return
	}

	if m.Condition != nil {
		for _, depID := range condition.ResourceIDs(m.Condition) {
			if err := r.graph.AddEdge(ext.ID(), depID); err != nil {
				r.errs = append(r.errs, fmt.Errorf("dependency %s -> %s: %v", ext.ID(), depID, err))
				return
			}
		}
		m.Condition = condition.StripResources(m.Condition)
	}

	r.graph.SetMeta(ext.ID(), m.toGraph())
}

func (r *Run) require(resource, field, value string) bool {
	if value == "" {
		r.errs = append(r.errs, fmt.Errorf("%s requires %s (got empty string)", resource, field))
		return false
	}
	return true
}

func (r *Run) Err() error       { return errors.Join(r.errs...) }
func (r *Run) Graph() *graph.Graph          { return r.graph }
func (r *Run) Resources() []extensions.Extension { return r.graph.OrderedExtensions() }
func (r *Run) Platform() platform.Info      { return r.platform }

func (r *Run) Include(name string) {
	if r.app == nil {
		r.errs = append(r.errs, fmt.Errorf("Include(%q): no app context", name))
		return
	}
	entry, ok := r.app.blueprints[name]
	if !ok {
		r.errs = append(r.errs, fmt.Errorf("Include(%q): blueprint not registered", name))
		return
	}
	entry.fn(r)
}

// nm extracts nodeMeta from flat Opts fields. Every Opts struct has the same
// 7 fields; callers pass them by name.
func nm(noop bool, retry int, limit float64, autoEdge, autoGroup *bool, cond extensions.Condition) nodeMeta {
	return nodeMeta{Noop: noop, Retry: retry, Limit: limit, AutoEdge: autoEdge, AutoGroup: autoGroup, Condition: cond}
}

func (r *Run) File(path string, opts FileOpts) {
	if !r.require("File", "path", path) {
		return
	}
	r.addResource(newFileExtension(path, opts), nm(opts.Noop, opts.Retry, opts.Limit, opts.AutoEdge, opts.AutoGroup, opts.Condition))
}

func (r *Run) Package(name string, opts PackageOpts) {
	if !r.require("Package", "name", name) {
		return
	}
	if opts.State == "" {
		opts.State = Present
	}
	r.addResource(newPackageExtension(name, opts, r.platform.PkgManager), nm(opts.Noop, opts.Retry, opts.Limit, opts.AutoEdge, opts.AutoGroup, opts.Condition))
}

func (r *Run) Service(name string, opts ServiceOpts) {
	if !r.require("Service", "name", name) {
		return
	}
	if opts.State == "" {
		opts.State = Running
	}
	r.addResource(newServiceExtension(name, opts, r.platform.InitSystem), nm(opts.Noop, opts.Retry, opts.Limit, opts.AutoEdge, opts.AutoGroup, opts.Condition))
}

func (r *Run) Exec(name string, opts ExecOpts) {
	if !r.require("Exec", "name", name) {
		return
	}
	if !r.require("Exec", "command", opts.Command) {
		return
	}
	r.addResource(newExecExtension(name, opts), nm(opts.Noop, opts.Retry, opts.Limit, opts.AutoEdge, opts.AutoGroup, opts.Condition))
}

func (r *Run) User(name string, opts UserOpts) {
	if !r.require("User", "name", name) {
		return
	}
	r.addResource(newUserExtension(name, opts), nm(opts.Noop, opts.Retry, opts.Limit, opts.AutoEdge, opts.AutoGroup, opts.Condition))
}

func (r *Run) Reboot(name string, opts RebootOpts) {
	if !r.require("Reboot", "name", name) {
		return
	}
	r.addResource(newRebootExtension(name, opts), nm(opts.Noop, opts.Retry, opts.Limit, opts.AutoEdge, opts.AutoGroup, opts.Condition))
}

func (r *Run) Firewall(name string, opts FirewallOpts) {
	if !r.require("Firewall", "name", name) {
		return
	}
	if opts.Protocol == "" {
		opts.Protocol = "tcp"
	}
	if opts.Direction == "" {
		opts.Direction = "inbound"
	}
	if opts.Action == "" {
		opts.Action = "allow"
	}
	r.addResource(newFirewallExtension(name, opts), nm(opts.Noop, opts.Retry, opts.Limit, opts.AutoEdge, opts.AutoGroup, opts.Condition))
}

func (r *Run) Template(path string, opts TemplateOpts) {
	if !r.require("Template", "path", path) {
		return
	}
	if !r.require("Template", "source", opts.Source) {
		return
	}
	r.addResource(newTemplateExtension(path, opts), nm(opts.Noop, opts.Retry, opts.Limit, opts.AutoEdge, opts.AutoGroup, opts.Condition))
}

func (r *Run) Hostname(name string, opts HostnameOpts) {
	if !r.require("Hostname", "name", name) {
		return
	}
	r.addResource(newHostnameExtension(name, opts), nm(opts.Noop, opts.Retry, opts.Limit, opts.AutoEdge, opts.AutoGroup, opts.Condition))
}

func (r *Run) Cron(name string, opts CronOpts) {
	if !r.require("Cron", "name", name) {
		return
	}
	if !r.require("Cron", "schedule", opts.Schedule) {
		return
	}
	if !r.require("Cron", "command", opts.Command) {
		return
	}
	if opts.State == "" {
		opts.State = Present
	}
	r.addResource(newCronExtension(name, opts), nm(opts.Noop, opts.Retry, opts.Limit, opts.AutoEdge, opts.AutoGroup, opts.Condition))
}

func (r *Run) Repository(name string, opts RepositoryOpts) {
	if !r.require("Repository", "name", name) {
		return
	}
	if !r.require("Repository", "uri", opts.URI) {
		return
	}
	if opts.State == "" {
		opts.State = Present
	}
	state := "present"
	if opts.State == Absent {
		state = "absent"
	}
	r.addResource(extpkg.NewRepository(name, extpkg.RepositoryOpts{
		URI:          opts.URI,
		Distribution: opts.Distribution,
		Components:   opts.Components,
		GPGKey:       opts.GPGKey,
		Enabled:      opts.Enabled,
		State:        state,
		ManagerName:  r.platform.PkgManager,
		Critical:     opts.Critical,
	}), nm(opts.Noop, opts.Retry, opts.Limit, opts.AutoEdge, opts.AutoGroup, opts.Condition))
}

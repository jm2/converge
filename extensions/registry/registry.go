package registry

import "fmt"

// Opts configures a Registry resource.
type Opts struct {
	Value    string
	Type     string
	Data     any
	State    string // "present" (default) or "absent"
	Critical bool
}

// Registry manages a single Windows registry value. Check/Apply use the native
// golang.org/x/sys/windows/registry API (no reg.exe).
type Registry struct {
	Key      string
	Value    string
	Type     string
	Data     any
	State    string // "present" (default) or "absent"
	Critical bool
}

func New(key string, opts Opts) *Registry {
	state := opts.State
	if state == "" {
		state = "present"
	}
	return &Registry{
		Key:      key,
		Value:    opts.Value,
		Type:     opts.Type,
		Data:     opts.Data,
		State:    state,
		Critical: opts.Critical,
	}
}

func (r *Registry) ID() string       { return fmt.Sprintf("registry:%s\\%s", r.Key, r.Value) }
func (r *Registry) String() string   { return fmt.Sprintf("Registry %s\\%s", r.Key, r.Value) }
func (r *Registry) IsCritical() bool { return r.Critical }

// Package dsl provides the public SDK for building desired-state
// blueprints that compile into a single cross-platform binary.
package dsl

import (
	"os"
	"time"

	"github.com/TsekNet/converge/extensions"
)

// Blueprint is a function that declares desired system state.
type Blueprint func(r *Run)

type ResourceState string

const (
	Present ResourceState = "present"
	Absent  ResourceState = "absent"
)

type ServiceState string

const (
	Running ServiceState = "running"
	Stopped ServiceState = "stopped"
)

// Meta holds common metadata shared by all Opts structs.
type Meta struct {
	DependsOn []string
	Critical  bool
	Noop      bool                 // skip Apply, only Check (per-resource dry-run)
	Retry     int                  // per-resource max retries (0 = use daemon default)
	Limit     float64              // per-resource rate limit (0 = use daemon default)
	AutoEdge  *bool                // nil = enabled (default), false = disable auto-edges for this resource
	AutoGroup *bool                // nil = enabled (default), false = disable auto-grouping for this resource
	Condition extensions.Condition // nil = no gate; non-nil = skip until condition is met
}

type FileOpts struct {
	Content      string
	Mode         os.FileMode
	Owner        string
	Group        string
	Append       bool
	URL          string // when set, download from this URL instead of using Content
	Checksum     string // expected SHA-256 hex digest (only with URL)
	BlockName    string // when set, manages a tagged block within the file instead of owning the entire file
	BlockComment string // comment prefix for block markers (default: "#")
	Meta
}

type PackageOpts struct {
	State ResourceState
	Meta
}

type ServiceOpts struct {
	State       ServiceState
	Enable      bool
	StartupType string // "auto", "delayed-auto", "manual", "disabled" (Windows SCM)
	Meta
}

type ExecOpts struct {
	Command     string
	Args        []string
	OnlyIf      string
	OnlyIfMatch string   // when set, OnlyIf compares trimmed stdout against this string
	Shell       string   // "powershell", "pwsh", "bash", "sh", or "" (direct exec)
	ShellParams []string // when set, replaces default shell flags
	Dir         string
	Env         []string
	Retries     int
	RetryDelay  time.Duration
	Meta
}

type UserOpts struct {
	Groups []string
	Shell  string
	Home   string
	System bool
	Meta
}

type RegistryOpts struct {
	Value string
	Type  string
	Data  any
	State ResourceState // Present (default) or Absent
	Meta
}

type SecurityPolicyOpts struct {
	Category string // "password" or "lockout"
	Key      string
	Value    string
	Meta
}

type AuditPolicyOpts struct {
	Subcategory string
	Success     bool
	Failure     bool
	Meta
}

type SysctlOpts struct {
	Value   string
	Persist bool
	Meta
}

type PlistOpts struct {
	Key   string
	Value any
	Type  string // "bool", "int", "float", "string"
	Host  bool   // true = /Library/Preferences (system-wide), false = ~/Library/Preferences
	Meta
}

type FirewallOpts struct {
	Port      int
	Protocol  string // "tcp" or "udp"
	Direction string // "inbound" or "outbound"
	Action    string // "allow" or "block"
	Source    string // Optional source address/CIDR
	Dest      string // Optional destination address/CIDR
	State     ResourceState
	Meta
}

type RebootOpts struct {
	Reason  string
	Message string // optional user-facing message shown in converge output before the reboot fires
	Delay   time.Duration
	Meta
}

type TemplateOpts struct {
	Source string            // Go text/template source
	Vars   map[string]string // template variables
	Mode   os.FileMode
	Owner  string
	Group  string
	Meta
}

type HostnameOpts struct {
	Meta
}

type KernelModuleState string

const (
	ModuleLoaded      KernelModuleState = "loaded"
	ModuleBlacklisted KernelModuleState = "blacklisted"
)

type KernelModuleOpts struct {
	State KernelModuleState
	Meta
}

type CronOpts struct {
	Schedule string // cron expression (Linux/macOS) or trigger spec (Windows)
	Command  string
	User     string // user to run as
	State    ResourceState
	Meta
}

type RepositoryOpts struct {
	URI          string
	Distribution string // apt: distribution (e.g. "jammy")
	Components   string // apt: components (e.g. "main")
	GPGKey       string // GPG key URL
	Enabled      bool
	State        ResourceState
	Meta
}

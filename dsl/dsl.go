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

// Common fields shared by all Opts structs. Each Opts struct includes these
// directly (no embedding) so blueprint authors write flat structs.
//
//   Critical  bool                 // false = best-effort, true = failure aborts the run
//   Noop      bool                 // skip Apply, only Check (per-resource dry-run)
//   Retry     int                  // per-resource max retries (0 = use daemon default)
//   Limit     float64              // per-resource rate limit (0 = use daemon default)
//   AutoEdge  *bool                // nil = enabled, false = disable auto-edges
//   AutoGroup *bool                // nil = enabled, false = disable auto-grouping
//   Condition extensions.Condition // nil = no gate; non-nil = skip until condition is met

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
	State        ResourceState
	Critical     bool
	Noop         bool
	Retry        int
	Limit        float64
	AutoEdge     *bool
	AutoGroup    *bool
	Condition    extensions.Condition
}

type PackageOpts struct {
	State     ResourceState
	Critical  bool
	Noop      bool
	Retry     int
	Limit     float64
	AutoEdge  *bool
	AutoGroup *bool
	Condition extensions.Condition
}

type ServiceOpts struct {
	State       ServiceState
	Enable      bool
	StartupType string // "auto", "delayed-auto", "manual", "disabled" (Windows SCM)
	Critical    bool
	Noop        bool
	Retry       int
	Limit       float64
	AutoEdge    *bool
	AutoGroup   *bool
	Condition   extensions.Condition
}

type ExecOpts struct {
	Command     string
	Args        []string
	Shell       string   // "powershell", "pwsh", "cmd", "bash", "sh", "auto", or custom path
	ShellParams []string // when set, replaces default shell flags
	Dir         string
	Env         []string
	Retries     int           // exec-specific: retries within a single Apply call
	RetryDelay  time.Duration // delay between exec-specific retries
	Critical    bool
	Creates     string // idempotency guard: skip if this path exists
	OnlyIf      string // idempotency guard: skip unless this command succeeds
	Unless      string // idempotency guard: skip if this command succeeds
	Noop        bool
	Retry       int // engine-level: how many times the daemon retries after Check/Apply failure
	Limit       float64
	AutoEdge    *bool
	AutoGroup   *bool
	Condition   extensions.Condition
}

type UserOpts struct {
	Groups    []string
	Shell     string
	Home      string
	System    bool
	Critical  bool
	Noop      bool
	Retry     int
	Limit     float64
	AutoEdge  *bool
	AutoGroup *bool
	Condition extensions.Condition
}

type RegistryOpts struct {
	Value     string
	Type      string
	Data      any
	State     ResourceState // Present (default) or Absent
	Critical  bool
	Noop      bool
	Retry     int
	Limit     float64
	AutoEdge  *bool
	AutoGroup *bool
	Condition extensions.Condition
}

type SecurityPolicyOpts struct {
	Category  string // "password" or "lockout"
	Key       string
	Value     string
	Critical  bool
	Noop      bool
	Retry     int
	Limit     float64
	AutoEdge  *bool
	AutoGroup *bool
	Condition extensions.Condition
}

type AuditPolicyOpts struct {
	Subcategory string
	Success     bool
	Failure     bool
	Critical    bool
	Noop        bool
	Retry       int
	Limit       float64
	AutoEdge    *bool
	AutoGroup   *bool
	Condition   extensions.Condition
}

type SysctlOpts struct {
	Value     string
	Persist   bool
	Critical  bool
	Noop      bool
	Retry     int
	Limit     float64
	AutoEdge  *bool
	AutoGroup *bool
	Condition extensions.Condition
}

type PlistOpts struct {
	Key       string
	Value     any
	Type      string // "bool", "int", "float", "string"
	Host      bool   // true = /Library/Preferences (system-wide), false = ~/Library/Preferences
	Critical  bool
	Noop      bool
	Retry     int
	Limit     float64
	AutoEdge  *bool
	AutoGroup *bool
	Condition extensions.Condition
}

type FirewallOpts struct {
	Port      int
	Protocol  string // "tcp" or "udp"
	Direction string // "inbound" or "outbound"
	Action    string // "allow" or "block"
	Source    string // Optional source address/CIDR
	Dest      string // Optional destination address/CIDR
	State     ResourceState
	Critical  bool
	Noop      bool
	Retry     int
	Limit     float64
	AutoEdge  *bool
	AutoGroup *bool
	Condition extensions.Condition
}

type RebootOpts struct {
	Reason    string
	Message   string // optional user-facing message shown in converge output before the reboot fires
	Delay     time.Duration
	Critical  bool
	Noop      bool
	Retry     int
	Limit     float64
	AutoEdge  *bool
	AutoGroup *bool
	Condition extensions.Condition
}

type TemplateOpts struct {
	Source    string            // Go text/template source
	Vars      map[string]string // template variables
	Mode      os.FileMode
	Owner     string
	Group     string
	Critical  bool
	Noop      bool
	Retry     int
	Limit     float64
	AutoEdge  *bool
	AutoGroup *bool
	Condition extensions.Condition
}

type HostnameOpts struct {
	Critical  bool
	Noop      bool
	Retry     int
	Limit     float64
	AutoEdge  *bool
	AutoGroup *bool
	Condition extensions.Condition
}

type KernelModuleState string

const (
	ModuleLoaded      KernelModuleState = "loaded"
	ModuleBlacklisted KernelModuleState = "blacklisted"
)

type KernelModuleOpts struct {
	State     KernelModuleState
	Critical  bool
	Noop      bool
	Retry     int
	Limit     float64
	AutoEdge  *bool
	AutoGroup *bool
	Condition extensions.Condition
}

type CronOpts struct {
	Schedule  string // cron expression (Linux/macOS) or trigger spec (Windows)
	Command   string
	User      string // user to run as
	State     ResourceState
	Critical  bool
	Noop      bool
	Retry     int
	Limit     float64
	AutoEdge  *bool
	AutoGroup *bool
	Condition extensions.Condition
}

type RepositoryOpts struct {
	URI          string
	Distribution string // apt: distribution (e.g. "jammy")
	Components   string // apt: components (e.g. "main")
	GPGKey       string // GPG key URL
	Enabled      bool
	State        ResourceState
	Critical     bool
	Noop         bool
	Retry        int
	Limit        float64
	AutoEdge     *bool
	AutoGroup    *bool
	Condition    extensions.Condition
}

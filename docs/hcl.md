# HCL Manifests

Converge supports authoring configuration in [HCL](https://github.com/hashicorp/hcl)
as an alternative to Go blueprints. HCL manifests are a Terraform-familiar,
Puppet-friendly surface for operators who prefer declarative config files over
compiling a Go blueprint.

The HCL front-end is **purely additive**: it parses a manifest into the same
`*internal/graph.Graph` of `extensions.Extension` resources that the Go DSL
produces, then hands it to the unchanged engine (`plan`) or daemon (`serve`).
Both front-ends share the entire provider, watcher, DAG, and reconciliation
core. Anything you can do in a Go blueprint with these resource types you can do
in HCL, and vice-versa.

---

## Running a manifest

Any `plan` or `serve` argument ending in `.hcl` is treated as a manifest path
instead of a registered blueprint name:

```
converge plan ./site.hcl
sudo converge serve ./site.hcl
```

Everything else (exit codes, flags, output formats, daemon behavior) is
identical to the blueprint commands — see [CLI](cli.md).

---

## Resource blocks

```hcl
resource "<type>" "<name>" {
  # type-specific attributes
}
```

The first label is the resource **type**; the second is a **name** used only to
reference the resource from other blocks. The actual resource key (file path,
service name, package name, ...) defaults to the block name but can be
overridden by the matching attribute (`path`, `name`, `key`, `module`).

```hcl
resource "package" "nginx" {
  ensure = "present"
}

resource "file" "app_conf" {
  path    = "/etc/nginx/conf.d/app.conf"
  content = "server { listen 80; }\n"
  mode    = "0644"
  require = [package.nginx]   # this file is written after nginx is installed
  notify  = [service.nginx]   # nginx is re-checked when this file changes
}

resource "service" "nginx" {
  ensure = "running"
  enable = true
}
```

`ensure` maps to each resource's state (`present`/`absent`, `running`/`stopped`,
`loaded`/`blacklisted`) and defaults to the same value the Go DSL uses
(`present` for package/cron/repository, `running` for service, `loaded` for
kernelmodule).

---

## Dependencies

Dependency edges use the full Puppet keyword set. Each takes a list of
references:

| Keyword | Meaning |
|---|---|
| `require` | this resource runs **after** the targets |
| `subscribe` | this resource runs **after** the targets |
| `before` | the targets run **after** this resource |
| `notify` | the targets run **after** this resource |
| `depends_on` | alias of `require` |

A reference is either a **symbolic** `type.name` (resolved via the block labels)
or a raw **ID string** (`"package:nginx"`) as an escape hatch — useful for
referencing IDs that aren't authored as their own block:

```hcl
require = [package.nginx, file.app_conf]
require = ["package:nginx"]
```

Blocks may be declared in any order; edges are resolved in a second pass after
every node exists. Converge's implicit edges (Service→Package, File→parent dir,
Service→config file) are also applied automatically.

> **Refresh semantics.** `notify`/`subscribe` create a dependency edge: when a
> dependency's `Apply` changes the system, the daemon re-checks its dependents.
> Converge has no separate "refresh action" — a notified resource only acts if
> its own `Check` then detects drift. This differs from Puppet, where `notify`
> triggers an explicit refresh (e.g. a service restart) regardless of the
> service's own state.

---

## Meta-arguments

Any block may carry per-resource meta, mapped onto `graph.NodeMeta`:

| Attribute | Type | Effect |
|---|---|---|
| `noop` | bool | check only, never apply (per-resource dry-run) |
| `retry` | number | per-resource max retries (daemon) |
| `auto_edge` | bool | set `false` to disable implicit edges for this node |
| `auto_group` | bool | set `false` to disable package auto-grouping for this node |
| `critical` | bool | failure halts the run (a type-specific attribute) |

`auto_edge`/`auto_group` are tri-state: omitting them keeps the default
(enabled); only an explicit `false` disables. There is intentionally **no**
`limit` attribute — the daemon does not consume `NodeMeta.Limit`, so it would be
a silent no-op.

---

## Supported resource types

All cross-platform: `file`, `package`, `repository`, `service`, `exec`, `cron`,
`firewall`, `hostname`, `reboot`, `template`, `user`.

Linux-only (registered only on Linux, matching the Go DSL): `sysctl`,
`kernelmodule`.

Field names mirror the corresponding `extensions/<type>.Opts`. Notable
conversions, since HCL has no native types for them:

- file/template `mode` is an **octal string** (`"0644"`).
- `retry_delay` (exec) and `delay` (reboot) are **Go duration strings** (`"5s"`).
- template `vars` is an HCL map (`{ Key = "value" }`).

`exec` has no `creates`/`only_if`/`unless` guards (converge's `exec` is
guardless by design; such guards belong to the condition system, which gates
only the initial convergence — see [Design](design.md)).

---

## Resource reference

HCL attribute names mirror the corresponding `extensions/<type>.Opts` fields
(snake_case). These examples parallel the Go DSL reference in
[Examples](examples.md#resource-reference).

### file

Content, permissions, ownership, remote downloads, and tagged blocks.

```hcl
# content mode (default)
resource "file" "motd" {
  path    = "/etc/motd"
  content = "Managed by Converge\n"
  mode    = "0644"
  owner   = "root"
  group   = "root"
}

# append to an existing file
resource "file" "hosts_entry" {
  path    = "/etc/hosts"
  content = "10.0.0.5 internal.example.com\n"
  append  = true
}

# remote download with SHA-256 verification
resource "file" "tool" {
  path     = "/opt/tools/binary"
  url      = "https://releases.example.com/v1.2/tool-linux-amd64"
  checksum = "e3b0c44298fc1c149afbf4c8996fb924..."
  mode     = "0755"
}

# manage a tagged block inside a file, leaving the rest untouched
resource "file" "internal_dns" {
  path       = "/etc/hosts"
  content    = "10.0.0.1 api.internal\n10.0.0.2 db.internal"
  block_name = "internal-dns"
}
```

`ensure = "absent"` removes a managed block (block mode only). On Windows
`mode`/`owner`/`group` are ignored.

### package

```hcl
resource "package" "curl" { ensure = "present" }
resource "package" "telnet" { ensure = "absent" }
```

The package manager is auto-detected (apt/dnf/yum/zypper/apk/pacman/brew/choco).

### service

```hcl
resource "service" "sshd" {
  ensure = "running"   # or "stopped"
  enable = true        # start on boot

  startup_type = "auto" # Windows SCM: auto / delayed-auto / manual / disabled
}
```

### exec

Run arbitrary commands. Use sparingly; prefer declarative resources.

```hcl
resource "exec" "flush_dns" {
  command = "/usr/bin/dscacheutil"
  args    = ["-flushcache"]
}

resource "exec" "download_agent" {
  command     = "curl"
  args        = ["-fsSL", "-o", "/tmp/agent.tar.gz", "https://example.com/agent.tar.gz"]
  retries     = 3
  retry_delay = "5s"
}

resource "exec" "configure_defender" {
  shell        = "pwsh" # auto / powershell / pwsh / cmd / bash / sh / custom path
  shell_params = ["-ExecutionPolicy", "RemoteSigned"]
  command      = "Set-MpPreference -DisableRealtimeMonitoring $false"
}
```

`exec` is not idempotent on its own and has no `creates`/`only_if`/`unless`
guards (see [Not yet supported](#not-yet-supported)).

### user

```hcl
resource "user" "deploy" {
  groups = ["docker", "sudo"]
  shell  = "/bin/bash"
  home   = "/home/deploy"
  system = false
}
```

### firewall

```hcl
resource "firewall" "allow_ssh" {
  port      = 22
  protocol  = "tcp"      # default tcp
  direction = "inbound"  # default inbound
  action    = "allow"    # default allow
  source    = "10.0.0.0/8"
}
```

### template

Render a Go `text/template` to a file.

```hcl
resource "template" "nginx_conf" {
  path   = "/etc/nginx/nginx.conf"
  source = "worker_processes {{.Workers}};\n"
  vars   = { Workers = "4" }
  mode   = "0644"
}
```

### cron

```hcl
resource "cron" "nightly_backup" {
  schedule = "0 2 * * *"
  command  = "/usr/local/bin/backup"
  user     = "root"
}
```

### repository

```hcl
resource "repository" "docker" {
  uri          = "https://download.docker.com/linux/ubuntu"
  distribution = "jammy"
  components   = "stable"
  gpg_key      = "https://download.docker.com/linux/ubuntu/gpg"
  enabled      = true
}
```

### hostname / reboot

```hcl
resource "hostname" "web01" {}

resource "reboot" "after_kernel" {
  reason = "kernel update"
  delay  = "1m"
}
```

### sysctl / kernelmodule (Linux only)

```hcl
resource "sysctl" "ip_forward" {
  key     = "net.ipv4.ip_forward"
  value   = "0"
  persist = true
}

resource "kernelmodule" "usb_storage" {
  module = "usb-storage"
  ensure = "blacklisted" # or "loaded"
}
```

See [`examples/`](../examples/) for complete, runnable manifests.

---

## Validation

Manifests are schema-validated with source-positioned diagnostics: unknown
resource types, unknown or missing attributes, malformed references, invalid
octal modes/durations, duplicate resource names, and resources whose
constructor rejects the input (e.g. a firewall rule with no port) all produce a
clear error pointing at the offending location, never a crash.

---

## Not yet supported

- Windows/macOS-only resource types (`registry`, `secpol`, `auditpol`, `plist`).
- HCL functions and a `secret(...)` helper over converge's encrypted config.
- Modules / `for_each` repetition and parameterized reusable bundles.
- `condition`-based gating from HCL.

These are tracked as follow-up work; the engine and provider layer already
support the underlying capabilities.

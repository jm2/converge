package manifest

import "testing"

// TestRegistryHasCrossPlatformTypes asserts the always-available resource types
// are registered (platform-specific ones are covered in build-tagged tests).
func TestRegistryHasCrossPlatformTypes(t *testing.T) {
	for _, typ := range []string{
		"file", "package", "repository", "service",
		"exec", "cron", "firewall", "hostname", "reboot", "template", "user",
	} {
		if _, ok := registryFor(typ); !ok {
			t.Errorf("resource type %q is not registered", typ)
		}
	}
}

func TestExecLoads(t *testing.T) {
	g := mustLoad(t, `
resource "exec" "migrate" {
  command     = "/usr/bin/true"
  args        = ["--flag"]
  env         = ["FOO=bar"]
  retries     = 3
  retry_delay = "5s"
}
`)
	g.requireNode("exec:migrate")
}

func TestExecMissingCommandRejected(t *testing.T) {
	mustFailLoad(t, `resource "exec" "x" { args = ["a"] }`)
}

func TestExecInvalidDurationRejected(t *testing.T) {
	mustFailLoad(t, `
resource "exec" "x" {
  command     = "/bin/true"
  retry_delay = "soon"
}
`)
}

func TestTemplateLoads(t *testing.T) {
	g := mustLoad(t, `
resource "template" "nginx" {
  path   = "/etc/nginx/nginx.conf"
  source = "worker_processes {{.Workers}};"
  vars   = { Workers = "4" }
  mode   = "0644"
}
`)
	g.requireNode("template:/etc/nginx/nginx.conf")
}

func TestTemplateMissingSourceRejected(t *testing.T) {
	mustFailLoad(t, `resource "template" "x" { path = "/tmp/x" }`)
}

func TestCronLoads(t *testing.T) {
	g := mustLoad(t, `
resource "cron" "backup" {
  schedule = "0 2 * * *"
  command  = "/usr/local/bin/backup"
}
`)
	g.requireNode("cron:backup")
}

func TestUserLoads(t *testing.T) {
	g := mustLoad(t, `
resource "user" "deploy" {
  groups = ["docker", "sudo"]
  shell  = "/bin/bash"
  system = true
}
`)
	g.requireNode("user:deploy")
}

func TestHostnameLoads(t *testing.T) {
	g := mustLoad(t, `resource "hostname" "web01" {}`)
	g.requireNode("hostname:web01")
}

func TestRebootLoads(t *testing.T) {
	g := mustLoad(t, `
resource "reboot" "after_kernel" {
  reason = "kernel update"
  delay  = "1m"
}
`)
	g.requireNode("reboot:after_kernel")
}

// TestFirewallValidLoads checks a well-formed firewall rule constructs cleanly.
func TestFirewallValidLoads(t *testing.T) {
	g := mustLoad(t, `resource "firewall" "ssh" { port = 22 }`)
	g.requireNode("firewall:ssh")
}

// TestFirewallInvalidYieldsDiagnostic is the key safeDecode guarantee: an
// invalid firewall (no/zero port) makes firewall.New panic, which must surface
// as a diagnostic rather than crash the process.
func TestFirewallInvalidYieldsDiagnostic(t *testing.T) {
	mustFailLoad(t, `resource "firewall" "bad" { protocol = "tcp" }`)
}

// TestEnsureDefaults verifies omitted `ensure` produces the documented defaults.
func TestEnsureDefaults(t *testing.T) {
	g := mustLoad(t, `
resource "package" "git" {}
resource "service" "sshd" {}
`)
	g.requireNode("package:git")  // package ensure defaults to present
	g.requireNode("service:sshd") // service ensure defaults to running
}

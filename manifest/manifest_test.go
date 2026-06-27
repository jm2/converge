package manifest

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"testing"

	"github.com/TsekNet/converge/internal/engine"
	"github.com/TsekNet/converge/internal/exit"
	"github.com/TsekNet/converge/internal/graph"
	"github.com/TsekNet/converge/internal/output"
)

func mustLoad(t *testing.T, src string) *graphHelper {
	t.Helper()
	g, diags := Load("test.hcl", []byte(src))
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics:\n%s", diags.Error())
	}
	return &graphHelper{t: t, g: g}
}

func mustFailLoad(t *testing.T, src string) {
	t.Helper()
	if _, diags := Load("test.hcl", []byte(src)); !diags.HasErrors() {
		t.Fatal("expected error diagnostics, got none")
	}
}

// TestLoadAndPlan is the end-to-end seam check: HCL -> file resource -> graph ->
// the real engine. The target file does not exist, so the plan must be pending.
func TestLoadAndPlan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "motd")
	src := fmt.Sprintf(`
resource "file" "motd" {
  path    = %q
  content = "hello from hcl\n"
  mode    = "0644"
}
`, path)

	g, diags := Load("test.hcl", []byte(src))
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics:\n%s", diags.Error())
	}
	exts := g.OrderedExtensions()
	if len(exts) != 1 {
		t.Fatalf("got %d resources, want 1", len(exts))
	}
	if got, want := exts[0].ID(), "file:"+path; got != want {
		t.Fatalf("resource ID = %q, want %q", got, want)
	}

	code, err := engine.RunPlanDAG(g, output.NewJSONPrinter(), engine.DefaultOptions())
	if err != nil {
		t.Fatalf("RunPlanDAG returned error: %v", err)
	}
	if code != exit.Pending {
		t.Fatalf("plan exit code = %d, want %d (exit.Pending)", code, exit.Pending)
	}
}

// TestPathDefaultsToLabel verifies the block name is used when path is omitted.
func TestPathDefaultsToLabel(t *testing.T) {
	g := mustLoad(t, `resource "file" "/etc/motd" { content = "hi" }`)
	g.requireNode("file:/etc/motd")
}

// TestEdgesAllDirections checks that the four Puppet keywords resolve to edges in
// the correct directions. graph.Children(id) returns the nodes that depend on id.
func TestEdgesAllDirections(t *testing.T) {
	src := `
resource "package" "nginx" { ensure = "present" }

resource "file" "conf" {
  path    = "/etc/nginx/nginx.conf"
  content = "..."
  require = [package.nginx]   # file depends on package
  notify  = [service.nginx]   # service depends on file
}

resource "service" "nginx" {
  ensure  = "running"
  before  = [file.conf]       # file depends on service
  subscribe = [package.nginx] # service depends on package
}
`
	g := mustLoad(t, src)
	// require: file depends on package -> package's children include file.
	g.requireEdge("package:nginx", "file:/etc/nginx/nginx.conf")
	// notify: service depends on file -> file's children include service.
	g.requireEdge("file:/etc/nginx/nginx.conf", "service:nginx")
	// before: file depends on service -> service's children include file.
	g.requireEdge("service:nginx", "file:/etc/nginx/nginx.conf")
	// subscribe: service depends on package -> package's children include service.
	g.requireEdge("package:nginx", "service:nginx")
}

// TestRawIDReference verifies the raw "type:id" escape-hatch reference form.
func TestRawIDReference(t *testing.T) {
	src := `
resource "package" "git" { ensure = "present" }
resource "file" "x" {
  path    = "/tmp/x"
  require = ["package:git"]
}
`
	g := mustLoad(t, src)
	g.requireEdge("package:git", "file:/tmp/x")
}

// TestDependsOnAlias verifies depends_on behaves like require.
func TestDependsOnAlias(t *testing.T) {
	src := `
resource "package" "git" { ensure = "present" }
resource "file" "x" {
  path       = "/tmp/x"
  depends_on = [package.git]
}
`
	g := mustLoad(t, src)
	g.requireEdge("package:git", "file:/tmp/x")
}

// TestMetaArgs verifies per-resource meta maps onto graph.NodeMeta.
func TestMetaArgs(t *testing.T) {
	src := `
resource "package" "git" {
  ensure     = "present"
  noop       = true
  retry      = 5
  auto_group = false
}
`
	g := mustLoad(t, src)
	n := g.g.Node("package:git")
	if n == nil {
		t.Fatal("package:git not found")
	}
	if !n.Meta.Noop {
		t.Error("Noop = false, want true")
	}
	if n.Meta.Retry != 5 {
		t.Errorf("Retry = %d, want 5", n.Meta.Retry)
	}
	if n.Meta.AutoGroup == nil || *n.Meta.AutoGroup {
		t.Errorf("AutoGroup = %v, want explicit false", n.Meta.AutoGroup)
	}
	if n.Meta.AutoEdge != nil {
		t.Errorf("AutoEdge = %v, want nil (unset)", n.Meta.AutoEdge)
	}
}

func TestUnknownResourceTypeRejected(t *testing.T) {
	mustFailLoad(t, `resource "frobnicate" "x" { whatever = true }`)
}

func TestUnknownAttributeRejected(t *testing.T) {
	mustFailLoad(t, `resource "file" "x" { path = "/tmp/x"`+"\n  color = \"red\"\n}")
}

func TestMissingRequiredAttributeRejected(t *testing.T) {
	// repository requires uri.
	mustFailLoad(t, `resource "repository" "x" { distribution = "jammy" }`)
}

func TestInvalidModeRejected(t *testing.T) {
	mustFailLoad(t, `resource "file" "x" { path = "/tmp/x"`+"\n  mode = \"not-octal\"\n}")
}

func TestDuplicateResourceRejected(t *testing.T) {
	mustFailLoad(t, `
resource "file" "a" { path = "/tmp/dup" }
resource "file" "b" { path = "/tmp/dup" }
`)
}

// TestDuplicateLabelRejected covers two blocks sharing a label but managing
// different resources: AddNode would not catch it (distinct IDs), but the label
// table would be silently clobbered, so Load must reject it.
func TestDuplicateLabelRejected(t *testing.T) {
	mustFailLoad(t, `
resource "file" "a" { path = "/tmp/one" }
resource "file" "a" { path = "/tmp/two" }
`)
}

// TestEmptyIdentifierRejected covers degenerate resources the Go DSL rejects via
// require(): an empty block label (with no overriding name/path), and a reboot
// "." name that the extension normalizes to empty.
func TestEmptyIdentifierRejected(t *testing.T) {
	mustFailLoad(t, `resource "file" "" {}`)
	mustFailLoad(t, `resource "package" "" { ensure = "present" }`)
	mustFailLoad(t, `resource "reboot" "." {}`)
}

// TestEmptyLabelWithExplicitKeyAccepted confirms the guard keys off the
// effective identifier, not the label: an empty label is fine if path is set.
func TestEmptyLabelWithExplicitKeyAccepted(t *testing.T) {
	g := mustLoad(t, `resource "file" "" { path = "/etc/motd" }`)
	g.requireNode("file:/etc/motd")
}

func TestUnknownReferenceRejected(t *testing.T) {
	mustFailLoad(t, `
resource "file" "x" {
  path    = "/tmp/x"
  require = [package.does_not_exist]
}
`)
}

func TestParseFileMode(t *testing.T) {
	tests := []struct {
		in   string
		want fs.FileMode
	}{
		{"0644", 0o644},
		{"0755", 0o755},
		{"4755", 0o755 | fs.ModeSetuid}, // setuid
		{"2755", 0o755 | fs.ModeSetgid}, // setgid
		{"1777", 0o777 | fs.ModeSticky}, // sticky
		{"6755", 0o755 | fs.ModeSetuid | fs.ModeSetgid},
	}
	for _, tt := range tests {
		got, err := parseFileMode(tt.in)
		if err != nil {
			t.Errorf("parseFileMode(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseFileMode(%q) = %v (%#o), want %v", tt.in, got, uint32(got), tt.want)
		}
	}

	if _, err := parseFileMode("not-octal"); err == nil {
		t.Error("parseFileMode(non-octal) should error")
	}
}

// graphHelper provides terse graph assertions for tests.
type graphHelper struct {
	t *testing.T
	g *graph.Graph
}

func (h *graphHelper) requireNode(id string) {
	h.t.Helper()
	if h.g.Node(id) == nil {
		h.t.Fatalf("expected node %q in graph", id)
	}
}

// requireEdge asserts that dependent depends on dependency (i.e. dependency's
// children include dependent).
func (h *graphHelper) requireEdge(dependency, dependent string) {
	h.t.Helper()
	if !slices.Contains(h.g.Children(dependency), dependent) {
		h.t.Fatalf("expected %q to depend on %q (children of %q = %v)",
			dependent, dependency, dependency, h.g.Children(dependency))
	}
}

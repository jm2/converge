// Package manifest is an HCL front-end for converge.
//
// It parses an HCL manifest into the existing extensions/* resources and builds
// an *internal/graph.Graph — the same artifact dsl/ produces from Go blueprints,
// just authored in HCL. It is purely additive: nothing below the
// extensions.Extension + graph.Graph boundary changes. The reactive core
// (internal/engine, internal/daemon, internal/graph) consumes the graph
// unchanged, so an HCL manifest can be run with `converge plan|serve`.
//
// Surface (Terraform-familiar, Puppet-friendly):
//
//	resource "<type>" "<name>" {
//	  # type-specific attributes (path, ensure, content, ...)
//
//	  # dependency edges (symbolic type.name refs, or raw "type:id" strings):
//	  require   = [package.nginx]   # this resource runs AFTER the targets
//	  subscribe = [file.conf]       # this resource runs AFTER the targets
//	  before    = [service.nginx]   # the targets run AFTER this resource
//	  notify    = [service.nginx]   # the targets run AFTER this resource
//	  depends_on = [package.nginx]  # alias of require
//
//	  # per-resource meta (mapped onto graph.NodeMeta):
//	  noop       = true             # check only, never apply
//	  retry      = 5                # per-resource max retries
//	  auto_edge  = false            # disable implicit edges for this node
//	  auto_group = false            # disable package auto-grouping for this node
//	}
//
// Edges are resolved in a second pass, so resources may be declared in any
// order. References accept either a symbolic type.name traversal (resolved via
// the block labels) or a raw extension-ID string (e.g. "package:nginx").
package manifest

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/TsekNet/converge/extensions"
	"github.com/TsekNet/converge/internal/graph"
	"github.com/TsekNet/converge/internal/graph/autoedge"
	"github.com/TsekNet/converge/internal/platform"
)

// unsupportedHint explains, for resource types that exist in converge but are
// not registered in this build, why they are unavailable — so the "unknown
// resource type" diagnostic does not misreport a real type as a typo (or a
// Windows/macOS type as a platform problem on the wrong OS).
var unsupportedHint = map[string]string{
	"sysctl":       "only available on Linux",
	"kernelmodule": "only available on Linux",
	"registry":     "not yet supported by the HCL front-end (Windows resource)",
	"secpol":       "not yet supported by the HCL front-end (Windows resource)",
	"auditpol":     "not yet supported by the HCL front-end (Windows resource)",
	"plist":        "not yet supported by the HCL front-end (macOS resource)",
}

// root is the top-level manifest schema: a flat list of resource blocks.
// gohcl rejects any other top-level construct with a source-positioned
// diagnostic, giving schema validation for free.
type root struct {
	Resources []resourceBlock `hcl:"resource,block"`
}

// resourceBlock is the generic `resource "<type>" "<name>" { ... }` envelope.
// The body is captured via ",remain" and split into common (edge/meta) and
// type-specific attributes in Load.
type resourceBlock struct {
	Type string   `hcl:"type,label"`
	Name string   `hcl:"name,label"`
	Body hcl.Body `hcl:",remain"`
}

// built pairs a constructed extension with the parsed block it came from, so the
// edge-resolution pass can reach both the resource ID and its common attributes.
type built struct {
	ext    extensions.Extension
	block  resourceBlock
	common commonAttrs
}

// Load parses HCL source and returns a graph of converge resources plus any
// diagnostics. Diagnostics are accumulated (mirroring dsl.Run.errs / Err())
// rather than failing on the first error. When the returned diagnostics
// HasErrors(), the graph is incomplete and must not be run.
//
// It runs in two passes because HCL is order-independent and graph.AddEdge
// errors if an endpoint is not yet present: pass 1 constructs every resource and
// adds its node (recording the type.name -> ID symbol table); pass 2 resolves
// the dependency references into edges and attaches per-resource meta.
func Load(filename string, src []byte) (*graph.Graph, hcl.Diagnostics) {
	f, diags := hclsyntax.ParseConfig(src, filename, hcl.InitialPos)
	if diags.HasErrors() {
		return nil, diags
	}

	var r root
	diags = append(diags, gohcl.DecodeBody(f.Body, nil, &r)...)
	if diags.HasErrors() {
		return nil, diags
	}

	dctx := &decodeContext{
		evalCtx:  evalContext(),
		platform: platform.Detect(),
	}

	g := graph.New()
	// labels maps "type.name" (the HCL block labels) to the constructed
	// extension's ID(); IDs are the resource's path/name/key, not the label,
	// so the table is the only way to resolve a symbolic reference to an edge.
	labels := make(map[string]string)
	var builts []built

	// Pass 1: construct + add every node.
	for _, rb := range r.Resources {
		var common commonAttrs
		// Peel the reserved edge/meta attributes off the body; the rest is
		// handed to the type-specific decoder, which strict-decodes it (so
		// unknown attributes still surface as diagnostics).
		if d := gohcl.DecodeBody(rb.Body, dctx.evalCtx, &common); d.HasErrors() {
			diags = append(diags, d...)
			continue
		}

		decode, ok := registryFor(rb.Type)
		if !ok {
			detail := fmt.Sprintf("%q is not a recognized resource type", rb.Type)
			if hint, known := unsupportedHint[rb.Type]; known {
				detail = fmt.Sprintf("resource type %q is %s", rb.Type, hint)
			}
			diags = append(diags, errDiag("unknown resource type", detail,
				rb.Body.MissingItemRange().Ptr()))
			continue
		}

		// Block labels are the reference handle; a collision would silently
		// misdirect every reference to the colliding name, so reject it.
		key := rb.Type + "." + rb.Name
		if _, dup := labels[key]; dup {
			diags = append(diags, errDiag("duplicate resource",
				"resource "+key+" is declared more than once",
				rb.Body.MissingItemRange().Ptr()))
			continue
		}

		ext, d := safeDecode(decode, rb.Name, common.Rest, dctx)
		diags = append(diags, d...)
		if ext == nil {
			continue
		}

		// Reject degenerate resources with an empty name/path/key. The Go DSL
		// rejects these at build time via require(); the extension constructors
		// do not, so without this guard the HCL front-end would build a graph
		// the DSL never would (e.g. an "ensure a package named ''" node).
		if resourceKey(ext.ID()) == "" {
			diags = append(diags, errDiag("invalid resource",
				fmt.Sprintf("resource %q has an empty name; give the block a non-empty label or set its name/path/key", key),
				rb.Body.MissingItemRange().Ptr()))
			continue
		}

		if err := g.AddNode(ext); err != nil {
			diags = append(diags, errDiag("duplicate resource", err.Error(),
				rb.Body.MissingItemRange().Ptr()))
			continue
		}
		labels[key] = ext.ID()
		builts = append(builts, built{ext: ext, block: rb, common: common})
	}

	// Pass 2: resolve edges and attach meta now that every node exists.
	for _, b := range builts {
		diags = append(diags, resolveEdges(g, b, labels, dctx.evalCtx)...)
		g.SetMeta(b.ext.ID(), b.common.nodeMeta())
	}

	// Match the Go-blueprint path: implicit ordering (Service->Package,
	// File->parent Dir, Service->config File) is added after explicit edges.
	if err := autoedge.AddAutoEdges(g); err != nil {
		diags = append(diags, errDiag("auto-edges", err.Error(), nil))
	}

	return g, diags
}

// safeDecode runs a resource decoder, converting a panic into a diagnostic.
// Several extension constructors (e.g. firewall.New) validate their input and
// panic on a violation, treating bad input as a programming error. That is fine
// for Go blueprints, but an HCL manifest is runtime-parsed user input, so such a
// failure must surface as an error rather than crash the process.
func safeDecode(fn decodeFunc, name string, body hcl.Body, dctx *decodeContext) (ext extensions.Extension, diags hcl.Diagnostics) {
	defer func() {
		if r := recover(); r != nil {
			ext = nil
			diags = hcl.Diagnostics{errDiag("invalid resource", fmt.Sprintf("%v", r), body.MissingItemRange().Ptr())}
		}
	}()
	return fn(name, body, dctx)
}

// resourceKey returns the key portion of an extension ID ("type:key"), e.g.
// "nginx" from "package:nginx" or "/etc/motd" from "file:/etc/motd". Converge
// IDs are always "type:key", so the first colon delimits the key; a Windows
// path like "C:\\x" in "file:C:\\x" keeps its drive letter since only the first
// colon is split on. An empty result means a degenerate, unnamed resource.
func resourceKey(id string) string {
	if _, key, ok := strings.Cut(id, ":"); ok {
		return key
	}
	return id
}

// errDiag is a small constructor for error diagnostics, keeping call sites terse.
func errDiag(summary, detail string, subject *hcl.Range) *hcl.Diagnostic {
	return &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  summary,
		Detail:   detail,
		Subject:  subject,
	}
}

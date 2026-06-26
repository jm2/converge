package manifest

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/TsekNet/converge/internal/graph"
)

// commonAttrs holds the reserved edge and meta attributes that every resource
// block may carry, regardless of type. Load decodes these off the block body
// first; the ",remain" body is then handed to the type-specific decoder, which
// strict-decodes it so unknown type-specific attributes still error.
//
// Edge attributes are captured as raw hcl.Expression (not decoded) so that a
// symbolic reference like `package.nginx` is not mistaken for a variable lookup
// at decode time — references are resolved separately in resolveEdges.
type commonAttrs struct {
	Require   hcl.Expression `hcl:"require,optional"`    // this resource AFTER targets
	Subscribe hcl.Expression `hcl:"subscribe,optional"`  // this resource AFTER targets
	Before    hcl.Expression `hcl:"before,optional"`     // targets AFTER this resource
	Notify    hcl.Expression `hcl:"notify,optional"`     // targets AFTER this resource
	DependsOn hcl.Expression `hcl:"depends_on,optional"` // alias of require

	Noop      *bool `hcl:"noop,optional"`       // check only, never apply
	Retry     *int  `hcl:"retry,optional"`      // per-resource max retries
	AutoEdge  *bool `hcl:"auto_edge,optional"`  // tri-state: nil = enabled
	AutoGroup *bool `hcl:"auto_group,optional"` // tri-state: nil = enabled

	Rest hcl.Body `hcl:",remain"`
}

// nodeMeta projects the common meta attributes onto graph.NodeMeta. AutoEdge and
// AutoGroup keep their tri-state pointer semantics (nil = default-enabled); Noop
// and Retry default to their zero values when unset.
//
// NOTE: graph.NodeMeta.Limit is intentionally not exposed — the daemon does not
// consume it (it always uses its built-in per-resource rate), so an HCL attr for
// it would be a silent no-op.
func (c commonAttrs) nodeMeta() graph.NodeMeta {
	m := graph.NodeMeta{AutoEdge: c.AutoEdge, AutoGroup: c.AutoGroup}
	if c.Noop != nil {
		m.Noop = *c.Noop
	}
	if c.Retry != nil {
		m.Retry = *c.Retry
	}
	return m
}

// resolveEdges turns a resource's edge attributes into graph edges. require,
// subscribe and depends_on make this resource depend on the targets (targets run
// first); before and notify make the targets depend on this resource.
func resolveEdges(g *graph.Graph, b built, labels map[string]string, ctx *hcl.EvalContext) hcl.Diagnostics {
	self := b.ext.ID()
	var diags hcl.Diagnostics

	// this-after-target: AddEdge(self, target) — self depends on target.
	for _, e := range []struct {
		kw   string
		expr hcl.Expression
	}{
		{"require", b.common.Require},
		{"subscribe", b.common.Subscribe},
		{"depends_on", b.common.DependsOn},
	} {
		ids, d := resolveRefs(e.kw, e.expr, ctx, labels, g)
		diags = append(diags, d...)
		for _, id := range ids {
			if err := g.AddEdge(self, id); err != nil {
				diags = append(diags, errDiag("invalid dependency", err.Error(), e.expr.Range().Ptr()))
			}
		}
	}

	// target-after-this: AddEdge(target, self) — target depends on self.
	for _, e := range []struct {
		kw   string
		expr hcl.Expression
	}{
		{"before", b.common.Before},
		{"notify", b.common.Notify},
	} {
		ids, d := resolveRefs(e.kw, e.expr, ctx, labels, g)
		diags = append(diags, d...)
		for _, id := range ids {
			if err := g.AddEdge(id, self); err != nil {
				diags = append(diags, errDiag("invalid dependency", err.Error(), e.expr.Range().Ptr()))
			}
		}
	}

	return diags
}

// resolveRefs evaluates one edge attribute (a list) into a slice of resource
// IDs. Each element is either a symbolic type.name traversal resolved via the
// block-label table, or a raw extension-ID string (e.g. "package:nginx").
func resolveRefs(kw string, expr hcl.Expression, ctx *hcl.EvalContext, labels map[string]string, g *graph.Graph) ([]string, hcl.Diagnostics) {
	if exprAbsent(expr) {
		return nil, nil
	}
	elems, diags := hcl.ExprList(expr)
	if diags.HasErrors() {
		return nil, diags
	}

	var ids []string
	for _, elem := range elems {
		// Prefer a symbolic reference: type.name.
		if trav, td := hcl.AbsTraversalForExpr(elem); !td.HasErrors() {
			key, ok := traversalKey(trav)
			if !ok {
				diags = append(diags, errDiag("invalid resource reference",
					"expected a reference of the form type.name (e.g. package.nginx)",
					trav.SourceRange().Ptr()))
				continue
			}
			id, found := labels[key]
			if !found {
				diags = append(diags, errDiag("unknown resource reference",
					fmt.Sprintf("%s refers to %q, which is not declared in this manifest", kw, key),
					trav.SourceRange().Ptr()))
				continue
			}
			ids = append(ids, id)
			continue
		}

		// Otherwise it must be a raw ID string, e.g. "package:nginx".
		v, vd := elem.Value(ctx)
		if vd.HasErrors() {
			diags = append(diags, vd...)
			continue
		}
		if v.Type() != cty.String {
			diags = append(diags, errDiag("invalid resource reference",
				"a reference must be a type.name symbol or an ID string", elem.Range().Ptr()))
			continue
		}
		id := v.AsString()
		if g.Node(id) == nil {
			diags = append(diags, errDiag("unknown resource reference",
				fmt.Sprintf("%s refers to ID %q, which is not declared in this manifest", kw, id),
				elem.Range().Ptr()))
			continue
		}
		ids = append(ids, id)
	}
	return ids, diags
}

// exprAbsent reports whether an edge attribute was omitted. gohcl assigns a
// synthesized null expression (not Go nil) to an absent optional hcl.Expression
// field, so a plain nil check is insufficient. A real reference list evaluates
// with errors under the empty context (unknown traversals), so only the
// null placeholder is treated as absent.
func exprAbsent(expr hcl.Expression) bool {
	if expr == nil {
		return true
	}
	v, diags := expr.Value(nil)
	return !diags.HasErrors() && v.IsNull()
}

// traversalKey extracts a "type.name" key from a two-step absolute traversal.
func traversalKey(t hcl.Traversal) (string, bool) {
	if len(t) != 2 {
		return "", false
	}
	root, ok := t[0].(hcl.TraverseRoot)
	if !ok {
		return "", false
	}
	attr, ok := t[1].(hcl.TraverseAttr)
	if !ok {
		return "", false
	}
	return root.Name + "." + attr.Name, true
}

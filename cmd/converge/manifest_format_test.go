package main

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
)

// TestFormatDiagnostics covers both rendering branches directly: diagnostics
// that carry a source position (Subject != nil) include it, and graph-global
// diagnostics without a position omit the "<file>:" prefix instead of printing
// "<nil>:". loadManifestGraph only ever produces the positioned variant, so the
// no-Subject branch needs a direct call.
func TestFormatDiagnostics(t *testing.T) {
	diags := hcl.Diagnostics{
		{
			Severity: hcl.DiagError,
			Summary:  "with position",
			Detail:   "has a subject",
			Subject:  &hcl.Range{Filename: "site.hcl"},
		},
		{
			Severity: hcl.DiagError,
			Summary:  "global",
			Detail:   "no subject",
		},
	}

	out := formatDiagnostics(diags)

	if !strings.Contains(out, "with position") || !strings.Contains(out, "has a subject") {
		t.Errorf("positioned diagnostic not rendered:\n%s", out)
	}
	if !strings.Contains(out, "global; no subject") {
		t.Errorf("global diagnostic not rendered without a position prefix:\n%s", out)
	}
	if strings.Contains(out, "<nil>") {
		t.Errorf("diagnostic without a Subject leaked a <nil> position:\n%s", out)
	}
}

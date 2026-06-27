package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"

	"github.com/TsekNet/converge/internal/graph"
	"github.com/TsekNet/converge/manifest"
)

// isManifestPath reports whether the CLI argument names an HCL manifest file
// rather than a registered blueprint. Blueprint names never end in ".hcl", so a
// suffix check is an unambiguous, additive discriminator: existing blueprint
// invocations are untouched. The check is case-insensitive so a file named
// site.HCL on a case-insensitive filesystem is still recognized.
func isManifestPath(arg string) bool {
	return strings.HasSuffix(strings.ToLower(arg), ".hcl")
}

// loadManifestGraph reads and parses an HCL manifest into a resource graph. All
// accumulated HCL diagnostics (each carrying a source position) are rendered,
// not just the first, so a manifest with several problems reports them together.
func loadManifestGraph(path string) (*graph.Graph, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	g, diags := manifest.Load(path, src)
	if diags.HasErrors() {
		return nil, fmt.Errorf("invalid manifest:\n%s", formatDiagnostics(diags))
	}
	return g, nil
}

// formatDiagnostics renders every diagnostic on its own line. hcl.Diagnostics.Error
// reports only the first plus a count; here we want the full list. Diagnostics
// without a source range (e.g. a graph-global error) omit the position rather
// than printing "<nil>:".
func formatDiagnostics(diags hcl.Diagnostics) string {
	var b strings.Builder
	for _, d := range diags {
		if d.Subject != nil {
			fmt.Fprintf(&b, "  %s: %s; %s\n", d.Subject, d.Summary, d.Detail)
		} else {
			fmt.Fprintf(&b, "  %s; %s\n", d.Summary, d.Detail)
		}
	}
	return b.String()
}

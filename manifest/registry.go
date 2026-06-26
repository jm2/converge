package manifest

import (
	"fmt"
	"io/fs"
	"strconv"
	"time"

	"github.com/hashicorp/hcl/v2"

	"github.com/TsekNet/converge/extensions"
	"github.com/TsekNet/converge/internal/platform"
)

// decodeContext carries everything a per-type decoder needs beyond its own
// block body: the HCL evaluation context (functions/variables) and the detected
// platform (package manager, init system) that several resources require.
type decodeContext struct {
	evalCtx  *hcl.EvalContext
	platform platform.Info
}

// decodeFunc decodes a single resource block body into a concrete extension.
// name is the block's second label (e.g. "nginx" in resource "service" "nginx").
// body contains only the type-specific attributes (edge/meta attributes have
// already been peeled off by Load), so decoders should strict-decode it to get
// schema validation. A decoder returns a nil extension when it emits errors.
type decodeFunc func(name string, body hcl.Body, dctx *decodeContext) (extensions.Extension, hcl.Diagnostics)

// registry maps a resource type name to its decoder. It is populated by per-type
// init() functions in resource_*.go files; platform-specific types live in
// build-tagged files so they only register on the OS that supports them.
var registry = map[string]decodeFunc{}

// register wires a decoder into the registry. It panics on a duplicate type,
// which can only happen from a programming error at init time.
func register(typeName string, fn decodeFunc) {
	if _, dup := registry[typeName]; dup {
		panic("manifest: duplicate resource type registered: " + typeName)
	}
	registry[typeName] = fn
}

// registryFor looks up a decoder by resource type.
func registryFor(typeName string) (decodeFunc, bool) {
	fn, ok := registry[typeName]
	return fn, ok
}

// evalContext builds the HCL evaluation context (functions and variables)
// exposed to manifests. It is empty for now; built-in functions (file,
// templatefile, env) and secret() are added in a later phase.
func evalContext() *hcl.EvalContext {
	return &hcl.EvalContext{}
}

// parseFileMode parses an octal permission string (e.g. "0644") into fs.FileMode.
// Mode is the one common field that is not a plain string on the Go side, so the
// HCL layer must convert it explicitly.
func parseFileMode(s string) (fs.FileMode, error) {
	n, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("mode must be octal (e.g. \"0644\"): %w", err)
	}
	return fs.FileMode(n), nil
}

// parseDuration parses a Go duration string (e.g. "5s", "2m") for resource
// fields typed time.Duration. HCL has no native duration type, so these are
// authored as strings.
func parseDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("duration must be a Go duration string (e.g. \"5s\"): %w", err)
	}
	return d, nil
}

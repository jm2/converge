package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

// loadExample parses an example manifest from ../examples and fails on any
// diagnostic. This keeps the documented, runnable examples accurate to the
// parser/registry as the implementation evolves.
func loadExample(t *testing.T, name string) {
	t.Helper()
	path := filepath.Join("..", "examples", name)
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if _, diags := Load(path, src); diags.HasErrors() {
		t.Fatalf("%s has diagnostics:\n%s", name, diags.Error())
	}
}

func TestExampleBaseline(t *testing.T) { loadExample(t, "baseline.hcl") }
func TestExampleWebStack(t *testing.T) { loadExample(t, "web-stack.hcl") }

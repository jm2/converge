package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsManifestPath(t *testing.T) {
	cases := map[string]bool{
		"baseline":        false,
		"cis":             false,
		"./site.hcl":      true,
		"/etc/conv/x.hcl": true,
		"x.hcl":           true,
		"site.HCL":        true, // case-insensitive
		"site.Hcl":        true,
		"x.tf":            false,
	}
	for arg, want := range cases {
		if got := isManifestPath(arg); got != want {
			t.Errorf("isManifestPath(%q) = %v, want %v", arg, got, want)
		}
	}
}

func TestLoadManifestGraph(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site.hcl")
	if err := os.WriteFile(path, []byte(`
resource "file" "motd" {
  path    = "/tmp/motd"
  content = "hi"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := loadManifestGraph(path)
	if err != nil {
		t.Fatalf("loadManifestGraph: %v", err)
	}
	if len(g.OrderedExtensions()) != 1 {
		t.Fatalf("got %d resources, want 1", len(g.OrderedExtensions()))
	}
}

func TestLoadManifestGraphReportsDiagnostics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.hcl")
	if err := os.WriteFile(path, []byte(`resource "nope" "x" {}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadManifestGraph(path); err == nil {
		t.Fatal("expected an error for an unknown resource type, got nil")
	}
}

func TestLoadManifestGraphMissingFile(t *testing.T) {
	if _, err := loadManifestGraph(filepath.Join(t.TempDir(), "does-not-exist.hcl")); err == nil {
		t.Fatal("expected an error for a missing manifest file, got nil")
	}
}

package extensions

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestOSFS_RoundTrip exercises the OSFS delegation methods against a real
// temp directory: write, stat, read, chmod, mkdir, and remove.
func TestOSFS_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	o := OSFS{}

	file := filepath.Join(dir, "data.txt")
	want := []byte("hello converge")

	if err := o.WriteFile(file, want, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	info, err := o.Stat(file)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() != int64(len(want)) {
		t.Errorf("Stat size = %d, want %d", info.Size(), len(want))
	}

	got, err := o.ReadFile(file)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("ReadFile = %q, want %q", got, want)
	}

	if err := o.Chmod(file, 0o600); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	info, err = o.Stat(file)
	if err != nil {
		t.Fatalf("Stat after chmod: %v", err)
	}
	// Go on Windows only models the read-only bit, so os.Stat does not round-trip
	// Unix sub-mode bits (it reports 0666); assert the exact perm on Unix only.
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("perm after Chmod = %o, want 600", perm)
		}
	}

	if err := o.Remove(file); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := o.Stat(file); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat after Remove err = %v, want ErrNotExist", err)
	}
}

// TestOSFS_MkdirAll verifies nested directory creation.
func TestOSFS_MkdirAll(t *testing.T) {
	dir := t.TempDir()
	o := OSFS{}

	nested := filepath.Join(dir, "a", "b", "c")
	if err := o.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	info, err := o.Stat(nested)
	if err != nil {
		t.Fatalf("Stat nested: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("MkdirAll did not create a directory")
	}
}

// TestOSFS_Errors confirms the delegation methods surface OS errors on
// missing paths rather than swallowing them.
func TestOSFS_Errors(t *testing.T) {
	o := OSFS{}
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	if _, err := o.Stat(missing); err == nil {
		t.Error("Stat on missing path should error")
	}
	if _, err := o.ReadFile(missing); err == nil {
		t.Error("ReadFile on missing path should error")
	}
	if err := o.Chmod(missing, 0o644); err == nil {
		t.Error("Chmod on missing path should error")
	}
	if err := o.Remove(missing); err == nil {
		t.Error("Remove on missing path should error")
	}
}

// Compile-time guard: OSFS must satisfy the FS interface (incl. Owner).
var _ FS = OSFS{}

func TestOSFS_WriteFilePerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not representable on Windows (os.Stat reports 0666)")
	}
	dir := t.TempDir()
	o := OSFS{}
	file := filepath.Join(dir, "perm.txt")
	if err := o.WriteFile(file, []byte("x"), 0o640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	// umask may clear bits, so only assert no extra bits beyond requested.
	if info.Mode().Perm()&^0o640 != 0 {
		t.Errorf("perm = %o, has bits beyond 640", info.Mode().Perm())
	}
}

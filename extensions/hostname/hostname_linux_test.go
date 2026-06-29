//go:build linux

package hostname

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/TsekNet/converge/internal/testutil"
)

// withSethostname swaps the package-level sethostname seam for the duration of
// a test, restoring the real syscall wrapper afterward.
func withSethostname(t *testing.T, fn func([]byte) error) {
	t.Helper()
	orig := sethostname
	sethostname = fn
	t.Cleanup(func() { sethostname = orig })
}

func TestHostname_Apply_Linux(t *testing.T) {
	ctx := context.Background()
	fs := testutil.NewMapFS()

	var got []byte
	withSethostname(t, func(name []byte) error {
		got = append([]byte(nil), name...)
		return nil
	})

	h := New("web01.example.com", Opts{FS: fs})
	res, err := h.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !res.Changed {
		t.Error("Result.Changed = false, want true")
	}

	if string(got) != "web01.example.com" {
		t.Errorf("sethostname called with %q, want %q", got, "web01.example.com")
	}

	data, err := fs.ReadFile("/etc/hostname")
	if err != nil {
		t.Fatalf("ReadFile(/etc/hostname) error = %v", err)
	}
	if string(data) != "web01.example.com\n" {
		t.Errorf("/etc/hostname = %q, want %q", data, "web01.example.com\n")
	}
}

func TestHostname_Apply_Linux_SethostnameError(t *testing.T) {
	ctx := context.Background()
	fs := testutil.NewMapFS()

	wantErr := errors.New("operation not permitted")
	withSethostname(t, func([]byte) error { return wantErr })

	h := New("web01", Opts{FS: fs})
	if _, err := h.Apply(ctx); err == nil {
		t.Fatal("Apply() error = nil, want non-nil when sethostname fails")
	}

	// /etc/hostname must not be written when the syscall fails first.
	if _, err := fs.ReadFile("/etc/hostname"); err == nil {
		t.Error("/etc/hostname was written despite sethostname failure")
	}
}

func TestHostname_Apply_Linux_WriteFileError(t *testing.T) {
	ctx := context.Background()

	withSethostname(t, func([]byte) error { return nil })

	h := New("web01", Opts{FS: &errFS{testutil.NewMapFS()}})
	if _, err := h.Apply(ctx); err == nil {
		t.Fatal("Apply() error = nil, want non-nil when WriteFile fails")
	}
}

// errFS is an extensions.FS whose WriteFile always fails, exercising the
// persistence error path of Apply. Other methods delegate to MapFS.
type errFS struct{ *testutil.MapFS }

func (*errFS) WriteFile(string, []byte, fs.FileMode) error { return errors.New("write failed") }

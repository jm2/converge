package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TsekNet/converge/internal/shell"
	"github.com/TsekNet/converge/internal/testutil"
)

func TestFile_Check(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		setup       func(t *testing.T, dir string)
		file        func(dir string) *File
		wantSync    bool
		wantChanges int
	}{
		{
			"file does not exist",
			func(t *testing.T, dir string) {},
			func(dir string) *File {
				return New(filepath.Join(dir, "new.txt"), Opts{Content: "hello\n", Mode: 0644})
			},
			false, 3,
		},
		{
			"file exists with correct content and mode",
			func(t *testing.T, dir string) {
				t.Helper()
				p := filepath.Join(dir, "ok.txt")
				os.WriteFile(p, []byte("hello\n"), 0644)
				os.Chmod(p, 0644)
			},
			func(dir string) *File {
				return New(filepath.Join(dir, "ok.txt"), Opts{Content: "hello\n", Mode: 0644})
			},
			true, 0,
		},
		{
			"file exists with wrong content",
			func(t *testing.T, dir string) {
				t.Helper()
				p := filepath.Join(dir, "wrong.txt")
				os.WriteFile(p, []byte("old\n"), 0644)
				os.Chmod(p, 0644)
			},
			func(dir string) *File {
				return New(filepath.Join(dir, "wrong.txt"), Opts{Content: "new\n", Mode: 0644})
			},
			false, 1,
		},
		{
			"file exists with wrong mode",
			func(t *testing.T, dir string) {
				t.Helper()
				os.WriteFile(filepath.Join(dir, "mode.txt"), []byte("hello\n"), 0755)
			},
			func(dir string) *File {
				return New(filepath.Join(dir, "mode.txt"), Opts{Content: "hello\n", Mode: 0644})
			},
			false, 1,
		},
		{
			"file exists with wrong content and mode",
			func(t *testing.T, dir string) {
				t.Helper()
				os.WriteFile(filepath.Join(dir, "both.txt"), []byte("old\n"), 0755)
			},
			func(dir string) *File {
				return New(filepath.Join(dir, "both.txt"), Opts{Content: "new\n", Mode: 0644})
			},
			false, 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			f := tt.file(dir)

			state, err := f.Check(ctx)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if state.InSync != tt.wantSync {
				t.Errorf("InSync = %v, want %v", state.InSync, tt.wantSync)
			}
			if len(state.Changes) != tt.wantChanges {
				t.Errorf("len(Changes) = %d, want %d: %+v", len(state.Changes), tt.wantChanges, state.Changes)
			}
		})
	}
}

func TestFile_Apply(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name   string
		setup  func(t *testing.T, dir string)
		file   func(dir string) *File
		verify func(t *testing.T, dir string)
	}{
		{
			"create new file",
			func(t *testing.T, dir string) {},
			func(dir string) *File {
				return New(filepath.Join(dir, "new.txt"), Opts{Content: "hello\n", Mode: 0644})
			},
			func(t *testing.T, dir string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, "new.txt"))
				if err != nil {
					t.Fatalf("read: %v", err)
				}
				if string(data) != "hello\n" {
					t.Errorf("content = %q, want %q", data, "hello\n")
				}
			},
		},
		{
			"overwrite existing file",
			func(t *testing.T, dir string) {
				t.Helper()
				os.WriteFile(filepath.Join(dir, "exist.txt"), []byte("old\n"), 0644)
			},
			func(dir string) *File {
				return New(filepath.Join(dir, "exist.txt"), Opts{Content: "new\n", Mode: 0644})
			},
			func(t *testing.T, dir string) {
				t.Helper()
				data, _ := os.ReadFile(filepath.Join(dir, "exist.txt"))
				if string(data) != "new\n" {
					t.Errorf("content = %q, want %q", data, "new\n")
				}
			},
		},
		{
			"append to file",
			func(t *testing.T, dir string) {
				t.Helper()
				os.WriteFile(filepath.Join(dir, "append.txt"), []byte("line1\n"), 0644)
			},
			func(dir string) *File {
				return New(filepath.Join(dir, "append.txt"), Opts{Content: "line2\n", Mode: 0644, Append: true})
			},
			func(t *testing.T, dir string) {
				t.Helper()
				data, _ := os.ReadFile(filepath.Join(dir, "append.txt"))
				if string(data) != "line1\nline2\n" {
					t.Errorf("content = %q, want %q", data, "line1\nline2\n")
				}
			},
		},
		{
			"create nested directory",
			func(t *testing.T, dir string) {},
			func(dir string) *File {
				return New(filepath.Join(dir, "sub", "deep", "file.txt"), Opts{Content: "nested\n", Mode: 0644})
			},
			func(t *testing.T, dir string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, "sub", "deep", "file.txt"))
				if err != nil {
					t.Fatalf("read: %v", err)
				}
				if string(data) != "nested\n" {
					t.Errorf("content = %q", data)
				}
			},
		},
		{
			"set mode",
			func(t *testing.T, dir string) {
				t.Helper()
				os.WriteFile(filepath.Join(dir, "perm.txt"), []byte("x"), 0755)
			},
			func(dir string) *File {
				return New(filepath.Join(dir, "perm.txt"), Opts{Content: "x", Mode: 0600})
			},
			func(t *testing.T, dir string) {
				t.Helper()
				info, _ := os.Stat(filepath.Join(dir, "perm.txt"))
				if info.Mode().Perm() != 0600 {
					t.Errorf("mode = %04o, want 0600", info.Mode().Perm())
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			f := tt.file(dir)

			result, err := f.Apply(ctx)
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if !result.Changed {
				t.Error("Changed should be true")
			}
			tt.verify(t, dir)
		})
	}
}

func TestFile_CheckThenApplyIdempotent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "idem.txt")

	f := New(path, Opts{Content: "converge\n", Mode: 0644})

	state, _ := f.Check(ctx)
	if state.InSync {
		t.Fatal("should not be in sync before Apply")
	}

	f.Apply(ctx)

	f2 := New(path, Opts{Content: "converge\n", Mode: 0644})
	state, _ = f2.Check(ctx)
	if !state.InSync {
		t.Errorf("should be in sync after Apply, changes: %+v", state.Changes)
	}
}

func TestFile_IDAndString(t *testing.T) {
	tests := []struct {
		path    string
		wantID  string
		wantStr string
	}{
		{"/etc/motd", "file:/etc/motd", "File /etc/motd"},
		{"/tmp/test.txt", "file:/tmp/test.txt", "File /tmp/test.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			f := New(tt.path, Opts{})
			if f.ID() != tt.wantID {
				t.Errorf("ID() = %q, want %q", f.ID(), tt.wantID)
			}
			if f.String() != tt.wantStr {
				t.Errorf("String() = %q, want %q", f.String(), tt.wantStr)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a long string", 10, "this is a ..."},
		{"", 5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := shell.Truncate(tt.input, tt.max); got != tt.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
			}
		})
	}
}

func TestFile_IsCritical(t *testing.T) {
	f := New("/tmp/test", Opts{Content: "content", Mode: 0644})
	if f.IsCritical() {
		t.Error("IsCritical() should be false by default")
	}
	fc := New("/tmp/test", Opts{Content: "content", Mode: 0644, Critical: true})
	if !fc.IsCritical() {
		t.Error("IsCritical() should be true when set")
	}
}

func TestFile_MapFS_CheckAndApply(t *testing.T) {
	ctx := context.Background()
	mfs := testutil.NewMapFS()

	f := New("/etc/motd", Opts{Content: "hello\n", Mode: 0644, FS: mfs})

	state, err := f.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if state.InSync {
		t.Error("should not be in sync before Apply")
	}

	result, err := f.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !result.Changed {
		t.Error("Changed should be true")
	}

	data, ok := mfs.Get("/etc/motd")
	if !ok {
		t.Fatal("file should exist in MapFS after Apply")
	}
	if string(data) != "hello\n" {
		t.Errorf("content = %q, want %q", data, "hello\n")
	}

	f2 := New("/etc/motd", Opts{Content: "hello\n", Mode: 0644, FS: mfs})
	state, err = f2.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !state.InSync {
		t.Errorf("should be in sync after Apply, changes: %+v", state.Changes)
	}
}

func TestFile_MapFS_WholeFileAbsent(t *testing.T) {
	ctx := context.Background()
	mfs := testutil.NewMapFS()
	mfs.Set("/etc/legacy.conf", []byte("old\n"), 0644)

	f := New("/etc/legacy.conf", Opts{State: "absent", FS: mfs})

	// Drift: the file exists but is declared absent.
	state, err := f.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if state.InSync {
		t.Fatal("should report drift when an absent-managed file exists")
	}

	// Apply removes it.
	res, err := f.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !res.Changed {
		t.Error("Apply should report Changed when removing the file")
	}
	if _, ok := mfs.Get("/etc/legacy.conf"); ok {
		t.Error("file should be removed from MapFS after Apply")
	}

	// Idempotent: now in sync; Apply is a no-op.
	state, err = f.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !state.InSync {
		t.Error("should be in sync once the file is absent")
	}
	res, err = f.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply() (no-op) error = %v", err)
	}
	if res.Changed {
		t.Error("Apply should be a no-op when the file is already absent")
	}
}

// TestFile_SensitiveRedactsContent verifies a Sensitive file never emits its
// content into Check diffs (which flow to plan/JSON/log output).
func TestFile_SensitiveRedactsContent(t *testing.T) {
	ctx := context.Background()
	const secret = "TOP-SECRET-VALUE"

	// Modify case: existing file differs.
	mfs := testutil.NewMapFS()
	mfs.Set("/etc/secret", []byte("old-secret-content"), 0600)
	f := New("/etc/secret", Opts{Content: secret, Mode: 0600, Sensitive: true, FS: mfs})
	st, err := f.Check(ctx)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, c := range st.Changes {
		if strings.Contains(c.From, "old-secret") || strings.Contains(c.To, secret) {
			t.Errorf("sensitive content leaked into change: %+v", c)
		}
	}

	// Create case: new file.
	mfs2 := testutil.NewMapFS()
	f2 := New("/etc/new-secret", Opts{Content: secret, Sensitive: true, FS: mfs2})
	st2, err := f2.Check(ctx)
	if err != nil {
		t.Fatalf("Check (create): %v", err)
	}
	for _, c := range st2.Changes {
		if strings.Contains(c.To, secret) {
			t.Errorf("sensitive content leaked into create change: %+v", c)
		}
	}
}

func TestFile_AbsentRejectsContent(t *testing.T) {
	f := New("/etc/x", Opts{State: "absent", Content: "oops"})
	if _, err := f.Check(context.Background()); err == nil {
		t.Error("State=absent combined with Content should be rejected")
	}
}

func TestFile_MapFS_Append(t *testing.T) {
	ctx := context.Background()
	mfs := testutil.NewMapFS()
	mfs.Set("/etc/config", []byte("line1\n"), 0644)

	f := New("/etc/config", Opts{Content: "line2\n", Mode: 0644, Append: true, FS: mfs})
	f.Apply(ctx)

	data, _ := mfs.Get("/etc/config")
	if string(data) != "line1\nline2\n" {
		t.Errorf("content = %q, want %q", data, "line1\nline2\n")
	}
}

func TestFile_MapFS_ContentDrift(t *testing.T) {
	ctx := context.Background()
	mfs := testutil.NewMapFS()
	mfs.Set("/etc/motd", []byte("old\n"), 0644)

	f := New("/etc/motd", Opts{Content: "new\n", Mode: 0644, FS: mfs})
	state, _ := f.Check(ctx)
	if state.InSync {
		t.Error("should detect content drift")
	}
	if len(state.Changes) == 0 {
		t.Error("should report changes")
	}
}

// --- Remote mode tests (URL + Checksum) ---

func sha256sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func TestFile_Remote_CheckMissing(t *testing.T) {
	mfs := testutil.NewMapFS()
	f := New("/opt/tool", Opts{
		URL:      "https://example.com/tool",
		Checksum: "abc123",
		FS:       mfs,
	})
	testutil.AssertDrifted(t, f)
}

func TestFile_Remote_CheckChecksumMatch(t *testing.T) {
	mfs := testutil.NewMapFS()
	content := []byte("binary-content-here")
	mfs.Set("/opt/tool", content, 0755)

	f := New("/opt/tool", Opts{
		URL:      "https://example.com/tool",
		Checksum: sha256sum(content),
		FS:       mfs,
	})
	testutil.AssertInSync(t, f)
}

func TestFile_Remote_CheckChecksumMismatch(t *testing.T) {
	mfs := testutil.NewMapFS()
	mfs.Set("/opt/tool", []byte("old-version"), 0755)

	f := New("/opt/tool", Opts{
		URL:      "https://example.com/tool",
		Checksum: sha256sum([]byte("new-version")),
		FS:       mfs,
	})
	testutil.AssertDrifted(t, f)
}

func TestFile_Remote_ApplyAndVerify(t *testing.T) {
	body := []byte("downloaded-payload-v2")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer ts.Close()

	mfs := testutil.NewMapFS()
	f := New("/opt/tool", Opts{
		URL:      ts.URL + "/tool",
		Checksum: sha256sum(body),
		Mode:     0755,
		FS:       mfs,
	})

	ctx := context.Background()
	result, err := f.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !result.Changed {
		t.Error("Apply() should report Changed=true")
	}

	got, ok := mfs.Get("/opt/tool")
	if !ok {
		t.Fatal("file should exist after Apply")
	}
	if string(got) != string(body) {
		t.Errorf("content = %q, want %q", got, body)
	}

	// Verify Check reports in-sync after Apply.
	f2 := New("/opt/tool", Opts{
		URL:      ts.URL + "/tool",
		Checksum: sha256sum(body),
		Mode:     0755,
		FS:       mfs,
	})
	testutil.AssertInSync(t, f2)
}

func TestFile_Remote_ChecksumMismatchRejectsDownload(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("wrong-content"))
	}))
	defer ts.Close()

	mfs := testutil.NewMapFS()
	f := New("/opt/tool", Opts{
		URL:      ts.URL + "/tool",
		Checksum: sha256sum([]byte("expected-content")),
		FS:       mfs,
	})

	ctx := context.Background()
	_, err := f.Apply(ctx)
	if err == nil {
		t.Fatal("Apply() should error on checksum mismatch")
	}
}

func TestFile_Remote_RequiresChecksum(t *testing.T) {
	mfs := testutil.NewMapFS()
	f := New("/opt/tool", Opts{
		URL: "https://example.com/tool",
		FS:  mfs,
	})

	ctx := context.Background()
	_, err := f.Check(ctx)
	if err == nil {
		t.Fatal("Check() should error when Checksum is empty with URL")
	}
}

func TestFile_Remote_MutualExclusion(t *testing.T) {
	mfs := testutil.NewMapFS()
	f := New("/opt/tool", Opts{
		Content:  "literal",
		URL:      "https://example.com/tool",
		Checksum: "abc",
		FS:       mfs,
	})

	ctx := context.Background()
	_, err := f.Check(ctx)
	if err == nil {
		t.Fatal("Check() should error when both Content and URL are set")
	}
}

// --- Block mode tests (BlockName) ---

func TestFile_Block_CheckMissing(t *testing.T) {
	mfs := testutil.NewMapFS()
	f := New("/etc/config", Opts{
		BlockName: "myapp",
		Content:   "key=value",
		FS:        mfs,
	})
	testutil.AssertDrifted(t, f)
}

func TestFile_Block_CheckInSync(t *testing.T) {
	mfs := testutil.NewMapFS()
	existing := "# BEGIN converge:myapp\nkey=value\n# END converge:myapp\n"
	mfs.Set("/etc/config", []byte(existing), 0644)

	f := New("/etc/config", Opts{
		BlockName: "myapp",
		Content:   "key=value",
		FS:        mfs,
	})
	testutil.AssertInSync(t, f)
}

func TestFile_Block_CheckDrift(t *testing.T) {
	mfs := testutil.NewMapFS()
	existing := "# BEGIN converge:myapp\nold=value\n# END converge:myapp\n"
	mfs.Set("/etc/config", []byte(existing), 0644)

	f := New("/etc/config", Opts{
		BlockName: "myapp",
		Content:   "new=value",
		FS:        mfs,
	})
	testutil.AssertDrifted(t, f)
}

func TestFile_Block_Converges(t *testing.T) {
	mfs := testutil.NewMapFS()
	f := New("/etc/config", Opts{
		BlockName: "myapp",
		Content:   "key=value",
		FS:        mfs,
	})
	testutil.AssertConverges(t, f)
}

func TestFile_Block_InsertIntoExisting(t *testing.T) {
	mfs := testutil.NewMapFS()
	mfs.Set("/etc/config", []byte("header=true\n"), 0644)

	f := New("/etc/config", Opts{
		BlockName: "myapp",
		Content:   "key=value",
		FS:        mfs,
	})

	ctx := context.Background()
	_, err := f.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	data, _ := mfs.Get("/etc/config")
	got := string(data)
	want := "header=true\n\n# BEGIN converge:myapp\nkey=value\n# END converge:myapp"
	if got != want {
		t.Errorf("content =\n%q\nwant\n%q", got, want)
	}
}

func TestFile_Block_UpdateExisting(t *testing.T) {
	mfs := testutil.NewMapFS()
	existing := "before\n# BEGIN converge:myapp\nold=val\n# END converge:myapp\nafter"
	mfs.Set("/etc/config", []byte(existing), 0644)

	f := New("/etc/config", Opts{
		BlockName: "myapp",
		Content:   "new=val",
		FS:        mfs,
	})

	ctx := context.Background()
	_, err := f.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	data, _ := mfs.Get("/etc/config")
	got := string(data)
	want := "before\n# BEGIN converge:myapp\nnew=val\n# END converge:myapp\nafter"
	if got != want {
		t.Errorf("content =\n%q\nwant\n%q", got, want)
	}
}

func TestFile_Block_Remove(t *testing.T) {
	mfs := testutil.NewMapFS()
	existing := "before\n# BEGIN converge:myapp\nkey=value\n# END converge:myapp\nafter"
	mfs.Set("/etc/config", []byte(existing), 0644)

	f := New("/etc/config", Opts{
		BlockName: "myapp",
		State:     "absent",
		FS:        mfs,
	})

	ctx := context.Background()
	_, err := f.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	data, _ := mfs.Get("/etc/config")
	got := string(data)
	want := "before\nafter"
	if got != want {
		t.Errorf("content =\n%q\nwant\n%q", got, want)
	}
}

func TestFile_Block_MissingEndMarker(t *testing.T) {
	mfs := testutil.NewMapFS()
	// Begin marker without matching end marker.
	existing := "# BEGIN converge:myapp\nkey=value\n"
	mfs.Set("/etc/config", []byte(existing), 0644)

	f := New("/etc/config", Opts{
		BlockName: "myapp",
		Content:   "key=value",
		FS:        mfs,
	})

	ctx := context.Background()
	_, err := f.Check(ctx)
	if err == nil {
		t.Fatal("Check() should error when end marker is missing")
	}
}

func TestFile_Block_RespectsMode(t *testing.T) {
	mfs := testutil.NewMapFS()
	f := New("/etc/config", Opts{
		BlockName: "myapp",
		Content:   "key=value",
		Mode:      0600,
		FS:        mfs,
	})

	ctx := context.Background()
	_, err := f.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	info, err := mfs.Stat("/etc/config")
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("mode = %04o, want 0600", info.Mode().Perm())
	}
}

// --- Helper unit tests ---

func TestExtractBlock(t *testing.T) {
	begin := "# BEGIN converge:test"
	end := "# END converge:test"

	tests := []struct {
		name    string
		data    string
		want    string
		wantErr bool
	}{
		{
			name: "block present",
			data: fmt.Sprintf("before\n%s\ncontent-line\n%s\nafter", begin, end),
			want: "content-line",
		},
		{
			name: "block absent",
			data: "no markers here",
			want: "",
		},
		{
			name: "empty block",
			data: fmt.Sprintf("%s\n%s", begin, end),
			want: "",
		},
		{
			name:    "missing end marker",
			data:    fmt.Sprintf("%s\ncontent-line\n", begin),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractBlock(tt.data, begin, end)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("extractBlock() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUpsertBlock(t *testing.T) {
	begin := "# BEGIN converge:test"
	end := "# END converge:test"
	block := begin + "\nnew-content\n" + end

	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "insert into empty",
			data: "",
			want: fmt.Sprintf("\n%s\nnew-content\n%s", begin, end),
		},
		{
			name: "replace existing",
			data: fmt.Sprintf("before\n%s\nold-content\n%s\nafter", begin, end),
			want: fmt.Sprintf("before\n%s\nnew-content\n%s\nafter", begin, end),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := upsertBlock(tt.data, begin, end, block)
			if got != tt.want {
				t.Errorf("upsertBlock() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestExtractBlock_DuplicateMarkers(t *testing.T) {
	begin := "# BEGIN converge:test"
	end := "# END converge:test"

	tests := []struct {
		name string
		data string
	}{
		{
			"nested begin marker",
			fmt.Sprintf("%s\na\n%s\nb\n%s", begin, begin, end),
		},
		{
			"two complete blocks",
			fmt.Sprintf("%s\na\n%s\n%s\nb\n%s", begin, end, begin, end),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := extractBlock(tt.data, begin, end); err == nil {
				t.Fatal("expected error for malformed/duplicate markers, got nil")
			}
		})
	}
}

// TestFile_Block_RejectsMarkerInContent verifies that content containing a line
// matching the block sentinel is rejected (it would otherwise corrupt the block
// boundaries and grow the file unboundedly across applies).
func TestFile_Block_RejectsMarkerInContent(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		content string
	}{
		{"contains end marker", "key=value\n# END converge:myapp\nevil=true"},
		{"contains begin marker", "# BEGIN converge:myapp\nkey=value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mfs := testutil.NewMapFS()
			f := New("/etc/config", Opts{
				BlockName: "myapp",
				Content:   tt.content,
				FS:        mfs,
			})

			if _, err := f.Check(ctx); err == nil {
				t.Error("Check() should reject content containing a block marker")
			}
			if _, err := f.Apply(ctx); err == nil {
				t.Error("Apply() should reject content containing a block marker")
			}
			if mfs.Has("/etc/config") {
				t.Error("file must not be written when content is rejected")
			}
		})
	}
}

// TestFile_Block_RejectsMalformedExisting verifies that an existing file with
// malformed markers is not blindly rewritten (which would grow it unboundedly).
func TestFile_Block_RejectsMalformedExisting(t *testing.T) {
	ctx := context.Background()
	mfs := testutil.NewMapFS()
	existing := "# BEGIN converge:myapp\na\n# BEGIN converge:myapp\nb\n# END converge:myapp\n"
	mfs.Set("/etc/config", []byte(existing), 0644)

	f := New("/etc/config", Opts{BlockName: "myapp", Content: "key=value", FS: mfs})
	if _, err := f.Apply(ctx); err == nil {
		t.Fatal("Apply() should error on malformed existing markers")
	}

	// The file must be left untouched (not grown) when markers are malformed.
	data, _ := mfs.Get("/etc/config")
	if string(data) != existing {
		t.Errorf("file was modified despite malformed markers:\n%q", data)
	}
}

// recordingFS wraps a MapFS and records the order of WriteFile/Chmod calls so
// tests can assert mode-before-content ordering.
type recordingFS struct {
	*testutil.MapFS
	ops []fsOp
}

type fsOp struct {
	op   string // "write" or "chmod"
	mode fs.FileMode
}

func (r *recordingFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	r.ops = append(r.ops, fsOp{op: "write", mode: perm})
	return r.MapFS.WriteFile(name, data, perm)
}

func (r *recordingFS) Chmod(name string, mode fs.FileMode) error {
	r.ops = append(r.ops, fsOp{op: "chmod", mode: mode})
	return r.MapFS.Chmod(name, mode)
}

// TestFile_Apply_TightensModeBeforeWrite verifies that when tightening the mode
// of an existing file, the chmod happens before the new content is written, so
// the content is never briefly exposed under the looser permissions.
func TestFile_Apply_TightensModeBeforeWrite(t *testing.T) {
	ctx := context.Background()
	rec := &recordingFS{MapFS: testutil.NewMapFS()}
	// Pre-existing file with a loose, world-readable mode.
	rec.Set("/etc/secret", []byte("old\n"), 0644)

	f := New("/etc/secret", Opts{Content: "topsecret\n", Mode: 0600, FS: rec})
	if _, err := f.Apply(ctx); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	writeIdx := -1
	for i, op := range rec.ops {
		if op.op == "write" {
			writeIdx = i
			break
		}
	}
	if writeIdx < 0 {
		t.Fatalf("no write recorded, ops = %+v", rec.ops)
	}

	tightenedBefore := false
	for i := 0; i < writeIdx; i++ {
		if rec.ops[i].op == "chmod" && rec.ops[i].mode == 0600 {
			tightenedBefore = true
		}
	}
	if !tightenedBefore {
		t.Errorf("expected chmod to 0600 before write, ops = %+v", rec.ops)
	}

	// Final state must still be correct.
	data, _ := rec.Get("/etc/secret")
	if string(data) != "topsecret\n" {
		t.Errorf("content = %q, want %q", data, "topsecret\n")
	}
	info, _ := rec.Stat("/etc/secret")
	if info.Mode().Perm() != 0600 {
		t.Errorf("mode = %04o, want 0600", info.Mode().Perm())
	}
}

// TestFile_Apply_RefusesSymlink verifies that Apply refuses to write through a
// pre-planted symlink on the real filesystem (Linux/Unix).
func TestFile_Apply_RefusesSymlink(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(target, []byte("untouched\n"), 0600); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	f := New(link, Opts{Content: "attacker\n", Mode: 0644})
	if _, err := f.Apply(ctx); err == nil {
		t.Fatal("Apply() should refuse to write through a symlink")
	}

	// The symlink target must be untouched.
	data, _ := os.ReadFile(target)
	if string(data) != "untouched\n" {
		t.Errorf("target content = %q, want %q", data, "untouched\n")
	}
}

func TestRemoveBlock(t *testing.T) {
	begin := "# BEGIN converge:test"
	end := "# END converge:test"

	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "block present",
			data: fmt.Sprintf("before\n%s\ncontent\n%s\nafter", begin, end),
			want: "before\nafter",
		},
		{
			name: "block absent",
			data: "no markers here",
			want: "no markers here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeBlock(tt.data, begin, end)
			if got != tt.want {
				t.Errorf("removeBlock() = %q, want %q", got, tt.want)
			}
		})
	}
}

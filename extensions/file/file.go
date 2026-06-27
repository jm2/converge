package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/TsekNet/converge/extensions"
	"github.com/TsekNet/converge/internal/shell"
)

// maxDownloadSize caps remote file downloads to prevent OOM from malicious servers.
const maxDownloadSize = 512 << 20 // 512 MiB

// httpClient is a shared client with a sane timeout for remote downloads.
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("too many redirects")
		}
		if req.URL.Host != via[0].URL.Host {
			return fmt.Errorf("redirect to different host: %s", req.URL.Host)
		}
		if req.URL.Scheme != via[0].URL.Scheme {
			return fmt.Errorf("redirect changes scheme: %s -> %s", via[0].URL.Scheme, req.URL.Scheme)
		}
		return nil
	},
}

// File manages content, permissions, and ownership of a file on disk.
//
// Modes of operation (mutually exclusive, determined by which fields are set):
//   - Content set: write literal content (or append if Append is true)
//   - URL set: download from URL with optional SHA-256 Checksum verification
//   - BlockName set: manage a tagged block within an existing file
type File struct {
	Path         string
	Content      string
	Mode         fs.FileMode
	Owner        string
	Group        string
	Append       bool
	URL          string // when set, download from this URL instead of using Content
	Checksum     string // expected SHA-256 hex digest (required with URL)
	BlockName    string // when set, manages a tagged block instead of the entire file
	BlockComment string // comment prefix for block markers (default: "#")
	State        string // "present" or "absent" (absent removes the file, or the block in block mode)
	Critical     bool
	Sensitive    bool          // when true, redact content from Check diffs (e.g. secret-bearing files)
	FS           extensions.FS // nil uses the real OS filesystem
}

// Opts holds all configurable fields for a File resource.
type Opts struct {
	Content      string
	Mode         fs.FileMode
	Owner        string
	Group        string
	Append       bool
	URL          string // when set, download from this URL instead of using Content
	Checksum     string // expected SHA-256 hex digest (required with URL)
	BlockName    string // when set, manages a tagged block instead of the entire file
	BlockComment string // comment prefix for block markers (default: "#")
	State        string // "present" or "absent" (absent removes the file, or the block in block mode)
	Critical     bool
	Sensitive    bool          // when true, redact content from Check diffs (e.g. secret-bearing files)
	FS           extensions.FS // inject a mock for testing
}

func New(path string, opts Opts) *File {
	comment := opts.BlockComment
	if opts.BlockName != "" && comment == "" {
		comment = "#"
	}
	state := opts.State
	if opts.BlockName != "" && state == "" {
		state = "present"
	}
	return &File{
		Path:         path,
		Content:      opts.Content,
		Mode:         opts.Mode,
		Owner:        opts.Owner,
		Group:        opts.Group,
		Append:       opts.Append,
		URL:          opts.URL,
		Checksum:     opts.Checksum,
		BlockName:    opts.BlockName,
		BlockComment: comment,
		State:        state,
		Critical:     opts.Critical,
		Sensitive:    opts.Sensitive,
		FS:           opts.FS,
	}
}

func (f *File) ID() string {
	if f.BlockName != "" {
		return fmt.Sprintf("file:%s[%s]", f.Path, f.BlockName)
	}
	return fmt.Sprintf("file:%s", f.Path)
}

func (f *File) String() string {
	if f.BlockName != "" {
		return fmt.Sprintf("File %s [%s]", f.Path, f.BlockName)
	}
	return fmt.Sprintf("File %s", f.Path)
}

func (f *File) IsCritical() bool { return f.Critical }

func (f *File) fsys() extensions.FS { return extensions.RealFS(f.FS) }

// resolvePath returns the path used for filesystem operations.
//
// For the real OS filesystem (FS is nil, the production case) the path is made
// absolute via filepath.Abs, which resolves relative paths against the working
// directory and, on Windows, attaches the volume name — exactly the OS-native
// behavior callers expect.
//
// When a custom FS is injected (only the in-memory MapFS used in tests), the
// path is a logical key into that filesystem rather than a real OS path. Running
// filepath.Abs on it would be OS-dependent — on Windows it rewrites a key like
// "/etc/motd" into "C:\\etc\\motd", which no longer matches the forward-slash
// key the FS was seeded with. So for an injected FS the path is cleaned with the
// OS-independent "path" package, keeping logical keys stable across platforms.
func (f *File) resolvePath() (string, error) {
	if f.FS != nil {
		return path.Clean(f.Path), nil
	}
	return filepath.Abs(f.Path)
}

// refuseSymlink rejects operating on a final path component that is a symlink,
// preventing a pre-planted symlink from redirecting a privileged write or chmod
// to an unintended target. It applies only to the real OS filesystem: the
// extensions.FS abstraction's Stat follows symlinks, so an os.Lstat-based check
// is used directly here. When a filesystem is injected (e.g. the in-memory MapFS
// used in tests) there is no symlink concept and the check is skipped.
func (f *File) refuseSymlink(absPath string) error {
	if f.FS != nil {
		return nil
	}
	info, err := os.Lstat(absPath)
	if err != nil {
		if isNotExist(err) {
			return nil
		}
		return fmt.Errorf("lstat %s: %w", absPath, err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("refusing to write through symlink %s", absPath)
	}
	return nil
}

// isNotExist checks for file-not-found using errors.Is(fs.ErrNotExist),
// which works with both real OS errors and mock FS implementations.
func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err)
}

// modeMask is the set of fs.FileMode bits the file resource manages: the 9
// permission bits plus the setuid/setgid/sticky special bits. Comparing against
// this mask (rather than Mode().Perm(), which is perm-only) ensures special bits
// are detected — otherwise a setuid/setgid/sticky file would report perpetual
// drift or never be corrected.
const modeMask = fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky

// modeDrift reports whether the current mode differs from the desired one across
// the managed bits. A desired mode of 0 means "unmanaged" (no opinion).
func (f *File) modeDrift(current fs.FileMode) bool {
	return f.Mode != 0 && current&modeMask != f.Mode&modeMask
}

// posixMode renders an fs.FileMode as a 4-digit POSIX octal string, including
// the setuid/setgid/sticky bits (which fs.FileMode keeps as high-bit flags, so a
// plain %o would print a meaningless large number).
func posixMode(m fs.FileMode) string {
	o := uint32(m.Perm())
	if m&fs.ModeSetuid != 0 {
		o |= 0o4000
	}
	if m&fs.ModeSetgid != 0 {
		o |= 0o2000
	}
	if m&fs.ModeSticky != 0 {
		o |= 0o1000
	}
	return fmt.Sprintf("%04o", o)
}

// mode returns the active mode: "block", "remote", or "content".
// Returns an error if multiple mutually exclusive fields are set.
func (f *File) mode() (string, error) {
	count := 0
	active := "content"
	if f.URL != "" {
		count++
		active = "remote"
	}
	if f.BlockName != "" {
		count++
		active = "block"
	}
	if count > 1 {
		return "", fmt.Errorf("file %s: URL and BlockName are mutually exclusive", f.Path)
	}
	if f.URL != "" && f.Content != "" {
		return "", fmt.Errorf("file %s: Content and URL are mutually exclusive", f.Path)
	}
	// Whole-file "absent" means delete the file; it cannot be combined with
	// content-producing fields. (Block mode handles its own absent semantics.)
	if f.State == "absent" && f.BlockName == "" && (f.Content != "" || f.URL != "" || f.Append) {
		return "", fmt.Errorf("file %s: State \"absent\" cannot be combined with Content, URL, or Append", f.Path)
	}
	return active, nil
}

func (f *File) Check(ctx context.Context) (*extensions.State, error) {
	m, err := f.mode()
	if err != nil {
		return nil, err
	}
	switch m {
	case "block":
		return f.checkBlock()
	case "remote":
		if f.Checksum == "" {
			return nil, fmt.Errorf("file %s: Checksum is required when URL is set", f.Path)
		}
		return f.checkRemote()
	default:
		return f.checkFull()
	}
}

func (f *File) Apply(ctx context.Context) (*extensions.Result, error) {
	m, err := f.mode()
	if err != nil {
		return nil, err
	}
	switch m {
	case "block":
		return f.applyBlock()
	case "remote":
		if f.Checksum == "" {
			return nil, fmt.Errorf("file %s: Checksum is required when URL is set", f.Path)
		}
		return f.applyRemote(ctx)
	default:
		return f.applyFull()
	}
}

// contentForChange renders the desired content for a Change, redacting it when
// the file is marked Sensitive so secret-bearing content never leaks into
// plan/JSON/log output.
func (f *File) contentForChange() string {
	if f.Sensitive {
		return fmt.Sprintf("(sensitive, %d bytes)", len(f.Content))
	}
	return summarizeContent(f.Content)
}

// checkFull compares the entire file content against desired state.
func (f *File) checkFull() (*extensions.State, error) {
	absPath, err := f.resolvePath()
	if err != nil {
		return nil, fmt.Errorf("invalid path %q: %w", f.Path, err)
	}
	// Whole-file absent: in sync only when the file does not exist.
	if f.State == "absent" {
		if _, statErr := f.fsys().Stat(absPath); isNotExist(statErr) {
			return &extensions.State{InSync: true}, nil
		} else if statErr != nil {
			return nil, fmt.Errorf("stat %s: %w", absPath, statErr)
		}
		return &extensions.State{InSync: false, Changes: []extensions.Change{
			{Property: "state", From: "present", To: "absent", Action: "remove"},
		}}, nil
	}
	info, err := f.fsys().Stat(absPath)
	if isNotExist(err) {
		changes := []extensions.Change{
			{Property: "state", To: "create", Action: "add"},
			{Property: "content", To: f.contentForChange(), Action: "add"},
		}
		if f.Mode != 0 {
			changes = append(changes, extensions.Change{
				Property: "mode", To: posixMode(f.Mode), Action: "add",
			})
		}
		return &extensions.State{InSync: false, Changes: changes}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", absPath, err)
	}

	var changes []extensions.Change

	if f.Content != "" && !f.Append {
		existing, err := f.fsys().ReadFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", absPath, err)
		}
		if string(existing) != f.Content {
			if f.Sensitive {
				changes = append(changes, extensions.Change{Property: "content", From: "(sensitive)", To: "(sensitive)", Action: "modify"})
			} else {
				changes = append(changes, diffContent(string(existing), f.Content)...)
			}
		}
	}

	if f.modeDrift(info.Mode()) {
		changes = append(changes, extensions.Change{
			Property: "mode",
			From:     posixMode(info.Mode()),
			To:       posixMode(f.Mode),
			Action:   "modify",
		})
	}

	ownerChange, err := extensions.OwnershipChange(f.fsys(), absPath, f.Owner, f.Group)
	if err != nil {
		return nil, fmt.Errorf("check ownership %s: %w", absPath, err)
	}
	if ownerChange != nil {
		changes = append(changes, *ownerChange)
	}

	return &extensions.State{InSync: len(changes) == 0, Changes: changes}, nil
}

// applyFull writes the entire file content.
func (f *File) applyFull() (*extensions.Result, error) {
	absPath, err := f.resolvePath()
	if err != nil {
		return nil, fmt.Errorf("invalid path %q: %w", f.Path, err)
	}

	// Whole-file absent: remove the file (no-op if already gone).
	if f.State == "absent" {
		if err := f.fsys().Remove(absPath); err != nil {
			if isNotExist(err) {
				return &extensions.Result{Changed: false, Status: extensions.StatusOK, Message: "already absent"}, nil
			}
			return nil, fmt.Errorf("remove %s: %w", absPath, err)
		}
		return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "removed"}, nil
	}

	if err := f.refuseSymlink(absPath); err != nil {
		return nil, err
	}

	dir := filepath.Dir(absPath)
	if err := f.fsys().MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	if f.Content != "" {
		var content string
		if f.Append {
			existing, err := f.fsys().ReadFile(absPath)
			if err != nil && !isNotExist(err) {
				return nil, fmt.Errorf("read %s: %w", absPath, err)
			}
			content = string(existing) + f.Content
		} else {
			content = f.Content
		}

		perm := f.Mode
		if perm == 0 {
			perm = 0644
		}
		// Tighten the mode of an existing file before writing so new (possibly
		// secret) content is never briefly exposed under looser permissions. New
		// files are created directly with perm by WriteFile, so no window exists.
		if f.Mode != 0 {
			if _, statErr := f.fsys().Stat(absPath); statErr == nil {
				if err := f.fsys().Chmod(absPath, f.Mode); err != nil {
					return nil, fmt.Errorf("chmod %s: %w", absPath, err)
				}
			}
		}
		if err := f.fsys().WriteFile(absPath, []byte(content), perm); err != nil {
			return nil, fmt.Errorf("write %s: %w", absPath, err)
		}
	}

	if f.Mode != 0 {
		if err := f.fsys().Chmod(absPath, f.Mode); err != nil {
			return nil, fmt.Errorf("chmod %s: %w", absPath, err)
		}
	}

	if err := extensions.ApplyOwnership(f.fsys(), absPath, f.Owner, f.Group); err != nil {
		return nil, fmt.Errorf("chown %s: %w", absPath, err)
	}

	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "Updated"}, nil
}

// --- Remote file (download URL to local path) ---

// checkRemote verifies the local file exists and its checksum matches.
// Checksum is required for URL mode (enforced in Check).
func (f *File) checkRemote() (*extensions.State, error) {
	absPath, err := f.resolvePath()
	if err != nil {
		return nil, fmt.Errorf("invalid path %q: %w", f.Path, err)
	}

	info, err := f.fsys().Stat(absPath)
	if isNotExist(err) {
		return &extensions.State{
			InSync:  false,
			Changes: []extensions.Change{{Property: "state", To: "download from " + f.URL, Action: "add"}},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", absPath, err)
	}

	var changes []extensions.Change

	actual, err := fsFileSHA256(f.fsys(), absPath)
	if err != nil {
		return nil, fmt.Errorf("checksum %s: %w", absPath, err)
	}
	if !strings.EqualFold(actual, f.Checksum) {
		changes = append(changes, extensions.Change{
			Property: "sha256",
			From:     actual[:min(12, len(actual))] + "...",
			To:       f.Checksum[:min(12, len(f.Checksum))] + "...",
			Action:   "modify",
		})
	}

	if f.modeDrift(info.Mode()) {
		changes = append(changes, extensions.Change{
			Property: "mode",
			From:     posixMode(info.Mode()),
			To:       posixMode(f.Mode),
			Action:   "modify",
		})
	}

	return &extensions.State{InSync: len(changes) == 0, Changes: changes}, nil
}

// applyRemote downloads the URL to the local path.
func (f *File) applyRemote(ctx context.Context) (*extensions.Result, error) {
	absPath, err := f.resolvePath()
	if err != nil {
		return nil, fmt.Errorf("invalid path %q: %w", f.Path, err)
	}

	if err := f.refuseSymlink(absPath); err != nil {
		return nil, err
	}

	dir := filepath.Dir(absPath)
	if err := f.fsys().MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	data, err := httpGet(ctx, f.URL)
	if err != nil {
		return nil, err
	}

	actual := sha256Hex(data)
	if !strings.EqualFold(actual, f.Checksum) {
		return nil, fmt.Errorf("checksum mismatch for %s: got %s, want %s", absPath, actual, f.Checksum)
	}

	perm := f.Mode
	if perm == 0 {
		perm = 0644
	}
	// Tighten the mode of an existing file before writing so the downloaded
	// payload is never briefly exposed under looser permissions.
	if f.Mode != 0 {
		if _, statErr := f.fsys().Stat(absPath); statErr == nil {
			if err := f.fsys().Chmod(absPath, f.Mode); err != nil {
				return nil, fmt.Errorf("chmod %s: %w", absPath, err)
			}
		}
	}
	if err := f.fsys().WriteFile(absPath, data, perm); err != nil {
		return nil, fmt.Errorf("write %s: %w", absPath, err)
	}

	if f.Mode != 0 {
		if err := f.fsys().Chmod(absPath, f.Mode); err != nil {
			return nil, fmt.Errorf("chmod %s: %w", absPath, err)
		}
	}

	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "downloaded"}, nil
}

func httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request for %s: %w", url, err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, maxDownloadSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read body %s: %w", url, err)
	}
	if int64(len(data)) > maxDownloadSize {
		return nil, fmt.Errorf("download %s: response exceeds %d bytes", url, maxDownloadSize)
	}
	return data, nil
}

// fsFileSHA256 hashes a file through the FS abstraction.
func fsFileSHA256(fsys extensions.FS, path string) (string, error) {
	data, err := fsys.ReadFile(path)
	if err != nil {
		return "", err
	}
	return sha256Hex(data), nil
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// --- Block management (manages a tagged block within the file) ---

func (f *File) beginMarker() string {
	return fmt.Sprintf("%s BEGIN converge:%s", f.BlockComment, f.BlockName)
}

func (f *File) endMarker() string {
	return fmt.Sprintf("%s END converge:%s", f.BlockComment, f.BlockName)
}

// validateBlockContent ensures the managed block content does not itself contain
// a line matching the block's begin or end marker. Such a line would corrupt the
// block boundaries on the next read, causing perpetual drift and unbounded file
// growth across repeated applies.
func (f *File) validateBlockContent() error {
	begin, end := f.beginMarker(), f.endMarker()
	for _, line := range strings.Split(f.Content, "\n") {
		if line == begin || line == end {
			return fmt.Errorf("block %s[%s]: content contains a reserved marker line %q", f.Path, f.BlockName, line)
		}
	}
	return nil
}

func (f *File) checkBlock() (*extensions.State, error) {
	if f.State != "absent" {
		if err := f.validateBlockContent(); err != nil {
			return nil, err
		}
	}

	absPath, err := f.resolvePath()
	if err != nil {
		return nil, fmt.Errorf("invalid path %q: %w", f.Path, err)
	}

	data, err := f.fsys().ReadFile(absPath)
	if isNotExist(err) {
		if f.State == "absent" {
			return &extensions.State{InSync: true}, nil
		}
		return &extensions.State{
			InSync:  false,
			Changes: []extensions.Change{{Property: "block", To: f.BlockName, Action: "add"}},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", absPath, err)
	}

	existing, err := extractBlock(string(data), f.beginMarker(), f.endMarker())
	if err != nil {
		return nil, fmt.Errorf("block %s[%s]: %w", absPath, f.BlockName, err)
	}

	if f.State == "absent" {
		if existing == "" {
			return &extensions.State{InSync: true}, nil
		}
		return &extensions.State{
			InSync:  false,
			Changes: []extensions.Change{{Property: "block", From: f.BlockName, To: "", Action: "remove"}},
		}, nil
	}

	if existing == f.Content {
		return &extensions.State{InSync: true}, nil
	}

	action := "modify"
	if existing == "" {
		action = "add"
	}
	return &extensions.State{
		InSync:  false,
		Changes: []extensions.Change{{Property: "block", From: shell.Truncate(existing, 60), To: shell.Truncate(f.Content, 60), Action: action}},
	}, nil
}

func (f *File) applyBlock() (*extensions.Result, error) {
	if f.State != "absent" {
		if err := f.validateBlockContent(); err != nil {
			return nil, err
		}
	}

	absPath, err := f.resolvePath()
	if err != nil {
		return nil, fmt.Errorf("invalid path %q: %w", f.Path, err)
	}

	if err := f.refuseSymlink(absPath); err != nil {
		return nil, err
	}

	data, err := f.fsys().ReadFile(absPath)
	if isNotExist(err) && f.State == "absent" {
		return &extensions.Result{Changed: false, Status: extensions.StatusOK, Message: "file absent"}, nil
	}
	if isNotExist(err) {
		data = nil
	} else if err != nil {
		return nil, fmt.Errorf("read %s: %w", absPath, err)
	}

	// Refuse to rewrite a file whose existing markers are malformed (missing end
	// or duplicated); blindly upserting would corrupt and grow the file.
	if data != nil {
		if _, err := extractBlock(string(data), f.beginMarker(), f.endMarker()); err != nil {
			return nil, fmt.Errorf("block %s[%s]: %w", absPath, f.BlockName, err)
		}
	}

	var result string
	if f.State == "absent" {
		result = removeBlock(string(data), f.beginMarker(), f.endMarker())
	} else {
		block := f.beginMarker() + "\n" + f.Content + "\n" + f.endMarker()
		result = upsertBlock(string(data), f.beginMarker(), f.endMarker(), block)
	}

	dir := filepath.Dir(absPath)
	if err := f.fsys().MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	perm := f.Mode
	if perm == 0 {
		perm = 0644
	}
	if err := f.fsys().WriteFile(absPath, []byte(result), perm); err != nil {
		return nil, fmt.Errorf("write %s: %w", absPath, err)
	}

	msg := "updated"
	if f.State == "absent" {
		msg = "removed"
	}
	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: msg}, nil
}

// extractBlock returns the content between begin and end markers, or "" if not found.
// Returns an error if the begin marker is found but the end marker is missing.
func extractBlock(data, beginMarker, endMarker string) (string, error) {
	lines := strings.Split(data, "\n")
	var inside, seen bool
	var block []string

	for _, line := range lines {
		if line == beginMarker {
			// A second begin marker (nested or after a completed block) means the
			// markers are malformed; refuse rather than silently merging blocks.
			if inside || seen {
				return "", fmt.Errorf("duplicate begin marker")
			}
			inside = true
			seen = true
			continue
		}
		if line == endMarker {
			if !inside {
				continue
			}
			inside = false
			continue
		}
		if inside {
			block = append(block, line)
		}
	}

	if inside {
		return "", fmt.Errorf("begin marker found but end marker missing")
	}

	if len(block) == 0 {
		return "", nil
	}
	return strings.Join(block, "\n"), nil
}

func upsertBlock(data, beginMarker, endMarker, block string) string {
	lines := strings.Split(data, "\n")
	var result []string
	var inside bool
	var replaced bool

	for _, line := range lines {
		if line == beginMarker {
			inside = true
			replaced = true
			result = append(result, strings.Split(block, "\n")...)
			continue
		}
		if line == endMarker {
			inside = false
			continue
		}
		if !inside {
			result = append(result, line)
		}
	}

	if !replaced {
		if len(data) > 0 && !strings.HasSuffix(data, "\n") {
			result = append(result, "")
		}
		result = append(result, strings.Split(block, "\n")...)
	}

	return strings.Join(result, "\n")
}

func removeBlock(data, beginMarker, endMarker string) string {
	lines := strings.Split(data, "\n")
	var result []string
	var inside bool

	for _, line := range lines {
		if line == beginMarker {
			inside = true
			continue
		}
		if line == endMarker {
			inside = false
			continue
		}
		if !inside {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// diffContent produces a human-readable line-by-line diff, capped at 5 changes for readability.
func diffContent(old, new string) []extensions.Change {
	oldLines := strings.Split(strings.TrimRight(old, "\n"), "\n")
	newLines := strings.Split(strings.TrimRight(new, "\n"), "\n")

	var changes []extensions.Change

	maxLines := max(len(oldLines), len(newLines))
	shown := 0
	for i := range maxLines {
		if shown >= 5 {
			remaining := 0
			for j := i; j < maxLines; j++ {
				oldL, newL := lineAt(oldLines, j), lineAt(newLines, j)
				if oldL != newL {
					remaining++
				}
			}
			if remaining > 0 {
				changes = append(changes, extensions.Change{
					Property: "content", To: fmt.Sprintf("... and %d more lines", remaining), Action: "modify",
				})
			}
			break
		}
		oldL := lineAt(oldLines, i)
		newL := lineAt(newLines, i)
		if oldL == newL {
			continue
		}
		if oldL == "" {
			changes = append(changes, extensions.Change{
				Property: fmt.Sprintf("line %d", i+1), To: shell.Truncate(newL, 60), Action: "add",
			})
		} else if newL == "" {
			changes = append(changes, extensions.Change{
				Property: fmt.Sprintf("line %d", i+1), From: shell.Truncate(oldL, 60), Action: "remove",
			})
		} else {
			changes = append(changes, extensions.Change{
				Property: fmt.Sprintf("line %d", i+1), From: shell.Truncate(oldL, 40), To: shell.Truncate(newL, 40), Action: "modify",
			})
		}
		shown++
	}

	return changes
}

func lineAt(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}

func summarizeContent(s string) string {
	s = strings.TrimRight(s, "\n\r")
	lines := strings.Count(s, "\n") + 1
	if lines == 1 {
		return shell.Truncate(s, 60)
	}
	return fmt.Sprintf("%d lines, %d bytes", lines, len(s))
}

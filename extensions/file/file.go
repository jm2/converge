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

// isNotExist checks for file-not-found using errors.Is(fs.ErrNotExist),
// which works with both real OS errors and mock FS implementations.
func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err)
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

// checkFull compares the entire file content against desired state.
func (f *File) checkFull() (*extensions.State, error) {
	absPath, err := filepath.Abs(f.Path)
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
			{Property: "content", To: summarizeContent(f.Content), Action: "add"},
		}
		if f.Mode != 0 {
			changes = append(changes, extensions.Change{
				Property: "mode", To: fmt.Sprintf("%04o", f.Mode), Action: "add",
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
			changes = append(changes, diffContent(string(existing), f.Content)...)
		}
	}

	if f.Mode != 0 && info.Mode().Perm() != f.Mode {
		changes = append(changes, extensions.Change{
			Property: "mode",
			From:     fmt.Sprintf("%04o", info.Mode().Perm()),
			To:       fmt.Sprintf("%04o", f.Mode),
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
	absPath, err := filepath.Abs(f.Path)
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
	absPath, err := filepath.Abs(f.Path)
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

	if f.Mode != 0 && info.Mode().Perm() != f.Mode {
		changes = append(changes, extensions.Change{
			Property: "mode",
			From:     fmt.Sprintf("%04o", info.Mode().Perm()),
			To:       fmt.Sprintf("%04o", f.Mode),
			Action:   "modify",
		})
	}

	return &extensions.State{InSync: len(changes) == 0, Changes: changes}, nil
}

// applyRemote downloads the URL to the local path.
func (f *File) applyRemote(ctx context.Context) (*extensions.Result, error) {
	absPath, err := filepath.Abs(f.Path)
	if err != nil {
		return nil, fmt.Errorf("invalid path %q: %w", f.Path, err)
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

func (f *File) checkBlock() (*extensions.State, error) {
	absPath, err := filepath.Abs(f.Path)
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
	absPath, err := filepath.Abs(f.Path)
	if err != nil {
		return nil, fmt.Errorf("invalid path %q: %w", f.Path, err)
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
	var inside bool
	var block []string

	for _, line := range lines {
		if line == beginMarker {
			inside = true
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

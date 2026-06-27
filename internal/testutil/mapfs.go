// Package testutil provides an in-memory FS implementation for testing
// extensions without touching the real filesystem.
package testutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/TsekNet/converge/extensions"
)

// MapFS is a concurrency-safe, in-memory filesystem for testing.
type MapFS struct {
	mu    sync.RWMutex
	files map[string]*memFile
}

type memFile struct {
	data  []byte
	mode  fs.FileMode
	isDir bool
	uid   int
	gid   int
}

// Compile-time check.
var _ extensions.FS = (*MapFS)(nil)

// NewMapFS creates an empty in-memory filesystem.
func NewMapFS() *MapFS {
	return &MapFS{files: make(map[string]*memFile)}
}

func (m *MapFS) Stat(name string) (fs.FileInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.files[name]
	if !ok {
		return nil, &os.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return &memFileInfo{name: filepath.Base(name), size: int64(len(f.data)), mode: f.mode, isDir: f.isDir}, nil
}

func (m *MapFS) ReadFile(name string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.files[name]
	if !ok {
		return nil, &os.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
	}
	cp := make([]byte, len(f.data))
	copy(cp, f.data)
	return cp, nil
}

func (m *MapFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	m.files[name] = &memFile{data: cp, mode: perm}
	return nil
}

func (m *MapFS) MkdirAll(path string, perm fs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Walk each component and create directory entries so Stat/IsDir work.
	parts := strings.Split(filepath.Clean(path), string(filepath.Separator))
	cur := ""
	for _, p := range parts {
		if p == "" {
			cur = string(filepath.Separator)
			continue
		}
		cur = filepath.Join(cur, p)
		if !strings.HasPrefix(cur, string(filepath.Separator)) {
			cur = string(filepath.Separator) + cur
		}
		if _, ok := m.files[cur]; !ok {
			m.files[cur] = &memFile{mode: perm | fs.ModeDir, isDir: true}
		}
	}
	return nil
}

func (m *MapFS) Chmod(name string, mode fs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.files[name]
	if !ok {
		return &os.PathError{Op: "chmod", Path: name, Err: fs.ErrNotExist}
	}
	f.mode = mode
	return nil
}

func (m *MapFS) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.files[name]; !ok {
		return &os.PathError{Op: "remove", Path: name, Err: fs.ErrNotExist}
	}
	delete(m.files, name)
	return nil
}

func (m *MapFS) Chown(name string, uid, gid int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.files[name]
	if !ok {
		return &os.PathError{Op: "chown", Path: name, Err: fs.ErrNotExist}
	}
	if uid >= 0 {
		f.uid = uid
	}
	if gid >= 0 {
		f.gid = gid
	}
	return nil
}

func (m *MapFS) Owner(name string) (uid, gid int, err error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.files[name]
	if !ok {
		return 0, 0, &os.PathError{Op: "owner", Path: name, Err: fs.ErrNotExist}
	}
	return f.uid, f.gid, nil
}

// SetOwner seeds ownership on an existing file, for test setup.
func (m *MapFS) SetOwner(name string, uid, gid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if f, ok := m.files[name]; ok {
		f.uid, f.gid = uid, gid
	}
}

// Get returns the raw bytes stored at path, for test assertions.
func (m *MapFS) Get(name string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.files[name]
	if !ok {
		return nil, false
	}
	return f.data, true
}

// Set seeds a file into the map, for test setup.
func (m *MapFS) Set(name string, data []byte, mode fs.FileMode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[name] = &memFile{data: data, mode: mode}
}

// Has returns true if the path exists.
func (m *MapFS) Has(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.files[name]
	return ok
}

// memFileInfo implements fs.FileInfo for in-memory files.
type memFileInfo struct {
	name  string
	size  int64
	mode  fs.FileMode
	isDir bool
}

func (fi *memFileInfo) Name() string       { return fi.name }
func (fi *memFileInfo) Size() int64        { return fi.size }
func (fi *memFileInfo) Mode() fs.FileMode  { return fi.mode }
func (fi *memFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *memFileInfo) IsDir() bool        { return fi.isDir }
func (fi *memFileInfo) Sys() any           { return nil }

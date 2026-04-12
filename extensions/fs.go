package extensions

import (
	"io/fs"
	"os"
)

// FS abstracts filesystem operations for extensions that manage files.
// When nil, extensions fall back to OSFS (the real OS filesystem).
// Inject a mock implementation in tests to verify Check/Apply without root.
type FS interface {
	Stat(name string) (fs.FileInfo, error)
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
	MkdirAll(path string, perm fs.FileMode) error
	Chmod(name string, mode fs.FileMode) error
	Remove(name string) error
}

// OSFS delegates all operations to the os package.
type OSFS struct{}

func (OSFS) Stat(name string) (fs.FileInfo, error) { return os.Stat(name) }
func (OSFS) ReadFile(name string) ([]byte, error)  { return os.ReadFile(name) }
func (OSFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm)
}
func (OSFS) MkdirAll(path string, perm fs.FileMode) error { return os.MkdirAll(path, perm) }
func (OSFS) Chmod(name string, mode fs.FileMode) error    { return os.Chmod(name, mode) }
func (OSFS) Remove(name string) error                     { return os.Remove(name) }

// RealFS returns OSFS if fsys is nil, otherwise returns fsys.
func RealFS(fsys FS) FS {
	if fsys != nil {
		return fsys
	}
	return OSFS{}
}

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
	// Chown sets ownership. A uid or gid of -1 leaves that field unchanged.
	Chown(name string, uid, gid int) error
	// Owner returns the current uid/gid of name. On platforms without POSIX
	// ownership it returns an error; callers gate on ownershipSupported.
	Owner(name string) (uid, gid int, err error)
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
func (OSFS) Chown(name string, uid, gid int) error        { return os.Chown(name, uid, gid) }

// OSFS.Owner is implemented per-platform (fs_unix.go / fs_windows.go).

// RealFS returns OSFS if fsys is nil, otherwise returns fsys.
func RealFS(fsys FS) FS {
	if fsys != nil {
		return fsys
	}
	return OSFS{}
}

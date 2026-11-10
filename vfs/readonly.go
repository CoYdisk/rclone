package vfs

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rclone/rclone/fs"
)

// readOnlyPathsReloadInterval is how often the read only paths file is
// checked for changes. The file is only re-read if its modification
// time or size changed.
const readOnlyPathsReloadInterval = 500 * time.Millisecond

// readOnlyPathsManifest is the on disk format of the read only paths
// file.
//
//	{
//	    "paths": ["dir/sub/file.docx", "other/file.xlsx"]
//	}
type readOnlyPathsManifest struct {
	Paths []string `json:"paths"`
}

// readOnlyPaths is a set of paths (relative to the root of the VFS,
// slash separated) which must not be modified even though the
// underlying remote would allow it.
//
// This is used by the ydisk client for files which are locked by
// another user: the lock has to be enforced on the whole path, not
// just on the file contents, because applications (eg Microsoft Word)
// "save" a file by writing a temporary file and then replacing the
// original with it - which means deleting/renaming the original.
//
// The set is loaded from a JSON file which the client rewrites when
// the set of locks changes.
type readOnlyPaths struct {
	path string

	mu       sync.Mutex
	checked  time.Time // when the file was last stat'd
	loaded   bool      // whether set has been loaded at least once
	size     int64     // size of file when last read
	modTime  time.Time // modification time of file when last read
	set      map[string]struct{}
	lastErr  string // last error to avoid spamming the log
}

// newReadOnlyPaths returns a readOnlyPaths for the file passed in, or
// nil if path is empty.
func newReadOnlyPaths(filePath string) *readOnlyPaths {
	if filePath == "" {
		return nil
	}
	return &readOnlyPaths{
		path: filePath,
		set:  map[string]struct{}{},
	}
}

// cleanReadOnlyPath normalises a path for comparison: backslashes are
// converted to forward slashes, and leading/trailing slashes removed.
func cleanReadOnlyPath(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.Trim(name, "/")
	if name == "." {
		return ""
	}
	return name
}

// reload reads the file if it has changed since the last read.
//
// Call with mu held.
func (r *readOnlyPaths) reload(now time.Time) {
	if now.Sub(r.checked) < readOnlyPathsReloadInterval {
		return
	}
	r.checked = now
	fi, err := os.Stat(r.path)
	if err != nil {
		if r.loaded && os.IsNotExist(err) {
			// File has gone - treat as empty set
			if len(r.set) != 0 {
				fs.Debugf(nil, "vfs: read only paths file %q has gone - clearing read only paths", r.path)
			}
			r.set = map[string]struct{}{}
			r.loaded = true
			r.size = 0
			r.modTime = time.Time{}
			r.lastErr = ""
			return
		}
		if errStr := err.Error(); errStr != r.lastErr {
			fs.Errorf(nil, "vfs: failed to read read only paths file %q: %v", r.path, err)
			r.lastErr = errStr
		}
		return
	}
	if r.loaded && fi.Size() == r.size && fi.ModTime().Equal(r.modTime) {
		return // unchanged
	}
	data, err := os.ReadFile(r.path)
	if err != nil {
		if errStr := err.Error(); errStr != r.lastErr {
			fs.Errorf(nil, "vfs: failed to read read only paths file %q: %v", r.path, err)
			r.lastErr = errStr
		}
		return
	}
	var manifest readOnlyPathsManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		if errStr := err.Error(); errStr != r.lastErr {
			fs.Errorf(nil, "vfs: failed to parse read only paths file %q: %v", r.path, err)
			r.lastErr = errStr
		}
		return
	}
	set := make(map[string]struct{}, len(manifest.Paths))
	for _, p := range manifest.Paths {
		if p = cleanReadOnlyPath(p); p != "" {
			set[p] = struct{}{}
		}
	}
	r.set = set
	r.loaded = true
	r.size = fi.Size()
	r.modTime = fi.ModTime()
	r.lastErr = ""
	fs.Debugf(nil, "vfs: read only paths set to %d paths from %q", len(set), r.path)
}

// Contains returns true if name is in the read only set.
func (r *readOnlyPaths) Contains(name string) bool {
	if r == nil {
		return false
	}
	name = cleanReadOnlyPath(name)
	if name == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reload(time.Now())
	_, found := r.set[name]
	return found
}

// ContainsUnder returns true if name is in the read only set **or** is a
// parent directory of a path in the read only set.
//
// This is used for operations which affect a whole subtree (remove a
// directory, rename a directory): those must be refused too, otherwise
// deleting the parent directory would delete the protected file with it
// (or, worse, delete half of the directory before failing on it).
func (r *readOnlyPaths) ContainsUnder(name string) bool {
	if r == nil {
		return false
	}
	name = cleanReadOnlyPath(name)
	if name == "" {
		return false // the root is never protected
	}
	prefix := name + "/"
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reload(time.Now())
	if _, found := r.set[name]; found {
		return true
	}
	for p := range r.set {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// Len returns the number of paths in the read only set (after
// reloading it).
func (r *readOnlyPaths) Len() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reload(time.Now())
	return len(r.set)
}

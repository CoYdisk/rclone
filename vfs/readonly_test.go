package vfs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rclone/rclone/fstest"
	"github.com/rclone/rclone/vfs/vfscommon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeReadOnlyPathsFile writes a read only paths manifest to a temp
// file and returns its name.
func writeReadOnlyPathsFile(t *testing.T, paths ...string) string {
	fileName := filepath.Join(t.TempDir(), "readonly-paths.json")
	data, err := json.Marshal(readOnlyPathsManifest{Paths: paths})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(fileName, data, 0600))
	return fileName
}

func TestReadOnlyPaths(t *testing.T) {
	fileName := writeReadOnlyPathsFile(t, "dir/sub/file.docx", "\\other\\file.xlsx", "/top.txt")

	r := newReadOnlyPaths(fileName)
	require.NotNil(t, r)
	assert.Equal(t, 3, r.Len())
	assert.True(t, r.Contains("dir/sub/file.docx"))
	assert.True(t, r.Contains("dir\\sub\\file.docx"), "backslashes are normalised")
	assert.True(t, r.Contains("/dir/sub/file.docx"), "leading slash is normalised")
	assert.True(t, r.Contains("other/file.xlsx"))
	assert.True(t, r.Contains("top.txt"))
	assert.False(t, r.Contains("dir/sub/other.docx"))
	assert.False(t, r.Contains(""))

	// A nil set is safe to use
	var nilSet *readOnlyPaths
	assert.False(t, nilSet.Contains("anything"))
	assert.Equal(t, 0, nilSet.Len())

	// An empty file name gives a nil set (feature off)
	assert.Nil(t, newReadOnlyPaths(""))
}

func TestReadOnlyPathsReloads(t *testing.T) {
	fileName := writeReadOnlyPathsFile(t, "one.txt")
	r := newReadOnlyPaths(fileName)
	require.True(t, r.Contains("one.txt"))
	require.False(t, r.Contains("two.txt"))

	// Rewrite the file with a different set and a newer mtime
	time.Sleep(20 * time.Millisecond)
	data, err := json.Marshal(readOnlyPathsManifest{Paths: []string{"two.txt"}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(fileName, data, 0600))
	now := time.Now()
	require.NoError(t, os.Chtimes(fileName, now, now))

	// The reload is throttled so retry for a while
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if r.Contains("two.txt") && !r.Contains("one.txt") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	assert.True(t, r.Contains("two.txt"), "new paths should be picked up")
	assert.False(t, r.Contains("one.txt"), "removed paths should be dropped")
}

func TestReadOnlyPathsMissingFile(t *testing.T) {
	r := newReadOnlyPaths(filepath.Join(t.TempDir(), "does-not-exist.json"))
	assert.Equal(t, 0, r.Len())
	assert.False(t, r.Contains("anything"))
}

func TestReadOnlyPathsBadJSON(t *testing.T) {
	fileName := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(fileName, []byte("not json"), 0600))
	r := newReadOnlyPaths(fileName)
	// An unparsable file must not lock anything - better to allow than freeze
	assert.False(t, r.Contains("anything"))
}

// newTestReadOnlyVFS makes a test VFS which has rels in its read only
// paths file.
func newTestReadOnlyVFS(t *testing.T, rels ...string) (*fstest.Run, *VFS) {
	opt := vfscommon.Opt
	// Disable the cache cleaner as it interferes with these tests
	opt.CachePollInterval = 0
	// Disable synchronous write
	opt.WriteBack = 0
	// Disable handle caching so existing tests get immediate close behavior
	opt.HandleCaching = 0
	opt.ReadOnlyPaths = writeReadOnlyPathsFile(t, rels...)
	return newTestVFSOpt(t, &opt)
}

// TestVFSReadOnlyPaths checks that a listed path can be read but not
// modified in any way, and that the rest of the tree is unaffected.
func TestVFSReadOnlyPaths(t *testing.T) {
	const remote = "dir/file.docx"
	const contents = "hello world"

	r, v := newTestReadOnlyVFS(t, remote)
	r.WriteObject(context.Background(), remote, contents, time.Now())
	r.WriteObject(context.Background(), "dir/other.docx", "other", time.Now())

	// Reading works
	fd, err := v.Open(remote)
	require.NoError(t, err)
	buf := make([]byte, len(contents))
	n, err := fd.ReadAt(buf, 0)
	require.NoError(t, err)
	assert.Equal(t, contents, string(buf[:n]))
	require.NoError(t, fd.Close())

	// Opening for write is refused
	_, err = v.OpenFile(remote, os.O_WRONLY, 0666)
	assert.ErrorIs(t, err, EPERM, "open for write must be refused")
	_, err = v.OpenFile(remote, os.O_RDWR, 0666)
	assert.ErrorIs(t, err, EPERM, "open for read+write must be refused")
	_, err = v.OpenFile(remote, os.O_RDWR|os.O_TRUNC, 0666)
	assert.ErrorIs(t, err, EPERM, "truncating open must be refused")

	// The file is reported read only so that applications open it read
	// only rather than trying to write to it
	node, err := v.Stat(remote)
	require.NoError(t, err)
	file := node.(*File)
	assert.Zero(t, file.Mode().Perm()&0222, "write bits must be cleared: %v", file.Mode())

	// Removing it is refused
	assert.ErrorIs(t, v.Remove(remote), EPERM, "remove must be refused")

	// Renaming it away is refused
	assert.ErrorIs(t, v.Rename(remote, "dir/moved.docx"), EPERM, "rename of a read only path must be refused")

	// Renaming something over it is refused - this is how Office saves
	assert.ErrorIs(t, v.Rename("dir/other.docx", remote), EPERM, "rename over a read only path must be refused")

	// Setting the modification time is refused
	assert.ErrorIs(t, v.Chtimes(remote, time.Now(), time.Now()), EPERM)

	// Everything still works for paths which are not read only
	fd2, err := v.OpenFile("dir/other.docx", os.O_RDWR, 0666)
	require.NoError(t, err)
	require.NoError(t, fd2.Close())
	node2, err := v.Stat("dir/other.docx")
	require.NoError(t, err)
	assert.NotZero(t, node2.(*File).Mode().Perm()&0200, "normal files still writable")
	assert.NoError(t, v.Rename("dir/other.docx", "dir/other2.docx"))
	assert.NoError(t, v.Remove("dir/other2.docx"))

	// The read only file is still there and unchanged
	node3, err := v.Stat(remote)
	require.NoError(t, err)
	assert.Equal(t, int64(len(contents)), node3.Size())
}

// TestVFSReadOnlyPathsDirectory checks directories can be protected
// too.
func TestVFSReadOnlyPathsDirectory(t *testing.T) {
	r, v := newTestReadOnlyVFS(t, "dir")
	r.WriteObject(context.Background(), "dir/file.txt", "hello", time.Now())

	assert.ErrorIs(t, v.Remove("dir"), EPERM)
	assert.ErrorIs(t, v.Rename("dir", "dir2"), EPERM)

	// Children of a read only directory are not protected unless listed
	assert.NoError(t, v.Mkdir("dir2", 0777))
	require.NoError(t, v.MkdirAll("dir2/sub", 0777))
	assert.NoError(t, v.Remove("dir2/sub"))
	assert.NoError(t, v.Remove("dir2"))

	// Reading the read only directory is fine
	node, err := v.Stat("dir")
	require.NoError(t, err)
	assert.True(t, node.IsDir())
}

// TestVFSReadOnlyPathsNoList checks that without a read only paths file
// nothing is protected (the feature must be opt in).
func TestVFSReadOnlyPathsNoList(t *testing.T) {
	r, v := newTestVFS(t)
	r.WriteObject(context.Background(), "dir/file.txt", "hello", time.Now())

	fd, err := v.OpenFile("dir/file.txt", os.O_RDWR, 0666)
	require.NoError(t, err)
	require.NoError(t, fd.Close())
	assert.NoError(t, v.Remove("dir/file.txt"))
}

// TestVFSReadOnlyPathsSiblings checks that protecting one file does not
// affect the other files in the same directory - that was the obvious
// risk with a "make this path read only" feature.
func TestVFSReadOnlyPathsSiblings(t *testing.T) {
	const locked = "dir/locked.docx"

	r, v := newTestReadOnlyVFS(t, locked)
	for _, name := range []string{locked, "dir/siblingA.txt", "dir/siblingB.txt", "dir/sub/deep.txt", "top.txt"} {
		r.WriteObject(context.Background(), name, "hello "+name, time.Now())
	}

	// Siblings can be opened for write, truncated, removed and renamed
	fd, err := v.OpenFile("dir/siblingA.txt", os.O_RDWR, 0666)
	require.NoError(t, err)
	require.NoError(t, fd.Close())
	require.NoError(t, v.Remove("dir/siblingA.txt"))

	fd, err = v.OpenFile("dir/siblingB.txt", os.O_RDWR|os.O_TRUNC, 0666)
	require.NoError(t, err)
	require.NoError(t, fd.Close())
	require.NoError(t, v.Rename("dir/siblingB.txt", "dir/siblingB2.txt"))
	require.NoError(t, v.Chtimes("dir/siblingB2.txt", time.Now(), time.Now()))

	// Files in sub directories and at the top level are unaffected too
	require.NoError(t, v.Remove("dir/sub/deep.txt"))
	require.NoError(t, v.Mkdir("dir/newdir", 0777))
	require.NoError(t, v.Remove("top.txt"))

	// A sibling's mode still has its write bits
	node, err := v.Stat("dir/siblingB2.txt")
	require.NoError(t, err)
	assert.NotZero(t, node.(*File).Mode().Perm()&0200)

	// The locked file itself is still protected
	_, err = v.OpenFile(locked, os.O_RDWR, 0666)
	assert.ErrorIs(t, err, EPERM)
	assert.ErrorIs(t, v.Remove(locked), EPERM)
}

// TestVFSReadOnlyPathsParentDir checks that operations which affect a
// whole subtree containing a protected path are refused as a whole, so a
// "delete this folder" cannot delete the protected file as a side
// effect (or delete half the folder before failing on it).
func TestVFSReadOnlyPathsParentDir(t *testing.T) {
	r, v := newTestReadOnlyVFS(t, "dir/sub/locked.docx")
	r.WriteObject(context.Background(), "dir/sub/locked.docx", "locked", time.Now())
	r.WriteObject(context.Background(), "dir/sub/other.txt", "other", time.Now())
	r.WriteObject(context.Background(), "dir/other2.txt", "other2", time.Now())

	// Removing or renaming a parent directory of a protected path is refused
	assert.ErrorIs(t, v.Remove("dir/sub"), EPERM, "parent dir remove must be refused")
	assert.ErrorIs(t, v.Remove("dir"), EPERM, "grandparent dir remove must be refused")
	assert.ErrorIs(t, v.Rename("dir/sub", "dir/moved"), EPERM, "parent dir rename must be refused")
	assert.ErrorIs(t, v.Rename("dir", "moved"), EPERM)

	// Nothing was deleted as a side effect
	for _, name := range []string{"dir/sub/locked.docx", "dir/sub/other.txt", "dir/other2.txt"} {
		node, err := v.Stat(name)
		require.NoError(t, err, "%s must still exist", name)
		assert.False(t, node.IsDir())
	}

	// Other directories are unaffected
	require.NoError(t, v.MkdirAll("other/deeper", 0777))
	require.NoError(t, v.Remove("other/deeper"))
	require.NoError(t, v.Remove("other"))
}

// TestReadOnlyPathsContainsUnder checks the subtree matching rules.
func TestReadOnlyPathsContainsUnder(t *testing.T) {
	r := newReadOnlyPaths(writeReadOnlyPathsFile(t, "dir/sub/locked.docx"))

	assert.True(t, r.ContainsUnder("dir/sub/locked.docx"))
	assert.True(t, r.ContainsUnder("dir/sub"))
	assert.True(t, r.ContainsUnder("dir"))
	assert.False(t, r.ContainsUnder("dir/sub2"), "a name with the same prefix is a different path")
	assert.False(t, r.ContainsUnder("other"))
	assert.False(t, r.ContainsUnder(""), "the root is never protected")
	assert.False(t, r.Contains("dir"), "parent dirs are not themselves protected")
}

// TestDefaultReadOnlyPathsFile checks the "manifest next to the rclone
// config" default: it is only used when the file actually exists.
func TestDefaultReadOnlyPathsFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "rclone.conf")
	require.NoError(t, os.WriteFile(configPath, []byte("[ydisk-webdav]\n"), 0600))

	assert.Equal(t, "", defaultReadOnlyPathsFile(configPath), "no manifest => not enabled")
	assert.Equal(t, "", defaultReadOnlyPathsFile(""), "no config path => not enabled")

	manifest := filepath.Join(dir, readOnlyPathsFileName)
	require.NoError(t, os.WriteFile(manifest, []byte(`{"paths":["a/b.docx"]}`), 0600))
	assert.Equal(t, manifest, defaultReadOnlyPathsFile(configPath))
}

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

// writeReadOnlyPrefixesFile writes a read only paths manifest which
// carries protected container prefixes (and no exact paths) to a temp
// file and returns its name.
func writeReadOnlyPrefixesFile(t *testing.T, prefixes ...string) string {
	fileName := filepath.Join(t.TempDir(), "readonly-paths.json")
	data, err := json.Marshal(readOnlyPathsManifest{Prefixes: prefixes})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(fileName, data, 0600))
	return fileName
}

// newTestReadOnlyPrefixVFS makes a test VFS which has prefixes in its
// read only paths file.
func newTestReadOnlyPrefixVFS(t *testing.T, prefixes ...string) (*fstest.Run, *VFS) {
	opt := vfscommon.Opt
	// Disable the cache cleaner as it interferes with these tests
	opt.CachePollInterval = 0
	// Disable synchronous write
	opt.WriteBack = 0
	// Disable handle caching so existing tests get immediate close behavior
	opt.HandleCaching = 0
	opt.ReadOnlyPaths = writeReadOnlyPrefixesFile(t, prefixes...)
	return newTestVFSOpt(t, &opt)
}

// mustMkdirRemote creates directories straight on the remote, the way
// the server does - bypassing the VFS so the guard does not interfere.
func mustMkdirRemote(t *testing.T, r *fstest.Run, dirs ...string) {
	for _, dir := range dirs {
		require.NoError(t, r.Fremote.Mkdir(context.Background(), dir), "mkdir remote %q", dir)
	}
}

// TestReadOnlyPrefixes checks the matching rules of the "protected
// container" prefixes: the container itself and its direct children are
// protected, everything deeper is not.
func TestReadOnlyPrefixes(t *testing.T) {
	r := newReadOnlyPaths(writeReadOnlyPrefixesFile(t, "", "协同空间", "我的空间"))

	// A protected container itself
	assert.True(t, r.ContainsContainer("协同空间"))
	assert.True(t, r.ContainsContainer("我的空间"))
	assert.False(t, r.ContainsContainer(""), "the root is never a container itself")
	assert.False(t, r.ContainsContainer("协同空间/test"), "children of a container are not containers")

	// "" means "nothing may be created in the root"
	assert.True(t, r.ContainsContainerParent("11111"), "a new dir in the root must be refused")
	assert.True(t, r.ContainsContainerParent("我的空间/x.txt"), "a new file in a group dir must be refused")
	assert.True(t, r.ContainsContainerParent("协同空间"), "the container itself must be refused")
	assert.True(t, r.ContainsContainerParent("我的空间"))

	// Everything inside a real space is unaffected
	assert.False(t, r.ContainsContainerParent("协同空间/test/a.txt"))
	assert.False(t, r.ContainsContainerParent("协同空间/test/y"))

	// The exact path set is untouched by prefixes
	assert.False(t, r.Contains("协同空间/test/a.txt"))

	// A nil set is safe to use
	var nilSet *readOnlyPaths
	assert.False(t, nilSet.ContainsContainer("协同空间"))
	assert.False(t, nilSet.ContainsContainerParent("a"))
}

// TestReadOnlyPrefixesSegmentWise checks the comparison is per path
// segment: a prefix must not swallow a sibling whose name merely starts
// with the same characters.
func TestReadOnlyPrefixesSegmentWise(t *testing.T) {
	r := newReadOnlyPaths(writeReadOnlyPrefixesFile(t, "协同空间/test"))

	assert.True(t, r.ContainsContainer("协同空间/test"))
	assert.True(t, r.ContainsContainerParent("协同空间/test/a.txt"))
	assert.False(t, r.ContainsContainer("协同空间/test2"))
	assert.False(t, r.ContainsContainerParent("协同空间/test2/a.txt"), "test must not eat test2")
	assert.False(t, r.ContainsContainerParent("协同空间/test2"), "test must not eat test2")
	assert.False(t, r.ContainsContainerParent("协同空间备份/a.txt"))
}

// TestReadOnlyPrefixesLegacyManifest checks that a manifest without the
// prefixes key ("paths" only) behaves exactly as before.
func TestReadOnlyPrefixesLegacyManifest(t *testing.T) {
	r := newReadOnlyPaths(writeReadOnlyPathsFile(t, "dir/file.docx"))

	assert.True(t, r.Contains("dir/file.docx"))
	assert.False(t, r.ContainsContainer("dir"))
	assert.False(t, r.ContainsContainerParent("dir"))
	assert.False(t, r.ContainsContainerParent("11111"), "no prefixes => nothing is a container")
	assert.False(t, r.ContainsContainerParent("dir/file.docx"))
}

// TestVFSReadOnlyPrefixes checks the VFS level: creating / removing /
// renaming at the root or in a group dir is refused, while everything
// inside a real space keeps working.
func TestVFSReadOnlyPrefixes(t *testing.T) {
	r, v := newTestReadOnlyPrefixVFS(t, "", "协同空间", "我的空间")
	mustMkdirRemote(t, r, "协同空间", "协同空间/test", "协同空间/test2", "我的空间")
	r.WriteObject(context.Background(), "协同空间/test/a.txt", "hello", time.Now())

	// Nothing may be created in the mount root
	assert.ErrorIs(t, v.Mkdir("11111", 0777), EPERM, "mkdir in the root must be refused")
	// Nor in a group dir
	assert.ErrorIs(t, v.Mkdir("我的空间/x", 0777), EPERM, "mkdir in a group dir must be refused")
	assert.ErrorIs(t, v.Mkdir("协同空间/x", 0777), EPERM)
	_, err := v.OpenFile("我的空间/a.txt", os.O_WRONLY|os.O_CREATE, 0666)
	assert.ErrorIs(t, err, EPERM, "new file in a group dir must be refused")

	// The containers themselves cannot be removed or renamed
	assert.ErrorIs(t, v.Remove("协同空间"), EPERM, "removing a container must be refused")
	assert.ErrorIs(t, v.Remove("协同空间/test"), EPERM, "removing a direct child of a container must be refused")
	assert.ErrorIs(t, v.Rename("协同空间/test", "协同空间/testX"), EPERM)
	assert.ErrorIs(t, v.Rename("协同空间/test2", "11111"), EPERM, "renaming into the root must be refused")

	// Setting the mtime of a container is refused ...
	assert.ErrorIs(t, v.Chtimes("协同空间", time.Now(), time.Now()), EPERM)

	// ... but everything strictly deeper still works
	require.NoError(t, v.Mkdir("协同空间/test/sub", 0777), "mkdir inside a space must work")
	require.NoError(t, v.Mkdir("协同空间/test2/sub", 0777), "test2 is not covered by test")
	fd, err := v.OpenFile("协同空间/test/new.txt", os.O_WRONLY|os.O_CREATE, 0666)
	require.NoError(t, err, "new file inside a space must work")
	require.NoError(t, fd.Close())
	require.NoError(t, v.Remove("协同空间/test/new.txt"), "remove inside a space must work")
	require.NoError(t, v.Rename("协同空间/test/sub", "协同空间/test/sub2"), "rename inside a space must work")
	require.NoError(t, v.Rename("协同空间/test/a.txt", "协同空间/test/b.txt"), "rename inside a space must work")
	// Office saves an existing file by rewriting it and setting its mtime
	fd, err = v.OpenFile("协同空间/test/b.txt", os.O_RDWR|os.O_TRUNC, 0666)
	require.NoError(t, err, "overwriting a file inside a space must work")
	require.NoError(t, fd.Close())
	require.NoError(t, v.Chtimes("协同空间/test/b.txt", time.Now(), time.Now()),
		"setting the mtime of a file inside a space must work (Office save)")
	require.NoError(t, v.Remove("协同空间/test/b.txt"))
}

// TestVFSReadOnlyPrefixesDirMethods checks the Dir level methods.
//
// cmount - the mount used on Windows, ie the virtual disk - calls
// Dir.Create, Dir.Mkdir and Dir.RemoveName directly for CreateEx, Mkdir,
// Unlink and Rmdir (cmd/cmount/fs.go), so guarding only the VFS methods
// would let the real mount through.
func TestVFSReadOnlyPrefixesDirMethods(t *testing.T) {
	r, v := newTestReadOnlyPrefixVFS(t, "", "协同空间")
	mustMkdirRemote(t, r, "协同空间", "协同空间/test")
	r.WriteObject(context.Background(), "协同空间/test/a.txt", "hello", time.Now())

	root, err := v.Root()
	require.NoError(t, err)
	group, err := v.Stat("协同空间")
	require.NoError(t, err)
	groupDir := group.(*Dir)
	space, err := v.Stat("协同空间/test")
	require.NoError(t, err)
	spaceDir := space.(*Dir)

	// CreateEx / Mkdir / Unlink / Rmdir as cmount issues them
	_, err = root.Create("11111", os.O_RDWR|os.O_CREATE)
	assert.ErrorIs(t, err, EPERM, "creating a file in the root must be refused")
	_, err = root.Mkdir("11111")
	assert.ErrorIs(t, err, EPERM, "creating a dir in the root must be refused")
	_, err = groupDir.Create("a.txt", os.O_RDWR|os.O_CREATE)
	assert.ErrorIs(t, err, EPERM, "creating a file in a group dir must be refused")
	_, err = groupDir.Mkdir("x")
	assert.ErrorIs(t, err, EPERM, "creating a dir in a group dir must be refused")
	assert.ErrorIs(t, groupDir.RemoveName("test"), EPERM, "removing an entry of a group dir must be refused")
	assert.ErrorIs(t, groupDir.Remove(), EPERM, "removing a container must be refused")
	assert.ErrorIs(t, groupDir.RemoveAll(), EPERM, "removing a container recursively must be refused")
	assert.ErrorIs(t, groupDir.Rename("test", "testX", groupDir), EPERM, "rename inside a group dir must be refused")
	assert.ErrorIs(t, root.Rename("协同空间", "x", root), EPERM, "renaming a container must be refused")
	assert.ErrorIs(t, spaceDir.Rename("a.txt", "b.txt", root), EPERM, "renaming out into the root must be refused")
	// A container's own mtime is refused (cmount Utimens lands on Dir.SetModTime)
	assert.ErrorIs(t, groupDir.SetModTime(time.Now()), EPERM, "a container's mtime must be refused")

	// Deeper operations are unaffected
	_, err = spaceDir.Mkdir("sub")
	require.NoError(t, err, "mkdir inside a space must work")
	require.NoError(t, spaceDir.RemoveName("sub"), "remove inside a space must work")
	f, err := spaceDir.Create("b.txt", os.O_RDWR|os.O_CREATE)
	require.NoError(t, err, "create inside a space must work")
	require.NoError(t, f.Remove())
}

// TestVFSReadOnlyPrefixesLegacy checks a "paths" only manifest keeps the
// old behaviour: nothing is a container, so the root stays writable.
func TestVFSReadOnlyPrefixesLegacy(t *testing.T) {
	r, v := newTestReadOnlyVFS(t, "dir/locked.docx")
	r.WriteObject(context.Background(), "dir/locked.docx", "locked", time.Now())

	require.NoError(t, v.Mkdir("11111", 0777), "root must stay writable without prefixes")
	require.NoError(t, v.Mkdir("dir/sub", 0777), "group dirs are ordinary dirs without prefixes")
	assert.ErrorIs(t, v.Remove("dir/locked.docx"), EPERM, "exact paths still protected")
}

// newTestReadOnlyPrefixCacheVFS is like newTestReadOnlyPrefixVFS but sets
// the cache mode, which decides which file handle implementation is used
// (openRW instead of openWrite) - the production virtual disk runs with
// --vfs-cache-mode full, ie openRW.
func newTestReadOnlyPrefixCacheVFS(t *testing.T, cacheMode vfscommon.CacheMode, prefixes ...string) (*fstest.Run, *VFS) {
	opt := vfscommon.Opt
	opt.CachePollInterval = 0
	opt.WriteBack = 0
	opt.HandleCaching = 0
	opt.CacheMode = cacheMode
	opt.ReadOnlyPaths = writeReadOnlyPrefixesFile(t, prefixes...)
	return newTestVFSOpt(t, &opt)
}

// TestVFSReadOnlyPrefixesFileInContainer pins the file handle layer: a
// file which already sits directly inside a protected container must not
// be writable, while reading it is untouched.
//
// This is a tripwire rather than a red-first case (it passed as soon as it
// was written): nothing else covers File.openWrite/openRW/
// SetModTime, and without it deleting those checks leaves every other
// prefix test green. cmount reaches them because CreateEx on an existing
// file returns the node and then opens it.
func TestVFSReadOnlyPrefixesFileInContainer(t *testing.T) {
	// No cache: write opens go through File.openWrite
	r, v := newTestReadOnlyPrefixVFS(t, "我的空间")
	r.WriteObject(context.Background(), "我的空间/a.txt", "hello", time.Now())

	node, err := v.Stat("我的空间/a.txt")
	require.NoError(t, err)
	_, err = node.Open(os.O_WRONLY)
	assert.ErrorIs(t, err, EPERM, "write open of a file in a container must be refused")
	_, err = node.Open(os.O_RDWR)
	assert.ErrorIs(t, err, EPERM, "read+write open of a file in a container must be refused")
	_, err = node.Open(os.O_WRONLY | os.O_TRUNC)
	assert.ErrorIs(t, err, EPERM, "truncating open of a file in a container must be refused")

	// Reading is unaffected
	data, err := v.ReadFile("我的空间/a.txt")
	require.NoError(t, err, "reading a file in a container must work")
	assert.Equal(t, "hello", string(data))
	fd, err := node.Open(os.O_RDONLY)
	require.NoError(t, err, "read open of a file in a container must work")
	require.NoError(t, fd.Close())

	// A path listed in prefixes is protected even when it is a file: only
	// the container itself rule applies to the mtime, and this pins it.
	r2, v2 := newTestReadOnlyPrefixVFS(t, "协同空间/test/a.txt")
	r2.WriteObject(context.Background(), "协同空间/test/a.txt", "hello", time.Now())
	node2, err := v2.Stat("协同空间/test/a.txt")
	require.NoError(t, err)
	assert.ErrorIs(t, node2.SetModTime(time.Now()), EPERM,
		"a listed path must not have its mtime changed even when it is a file")
	// ... and the same path is refused for writing through the VFS too
	assert.ErrorIs(t, v2.Chtimes("协同空间/test/a.txt", time.Now(), time.Now()), EPERM)

	// Cache mode full (what the virtual disk uses): write opens go through
	// File.openRW instead
	r3, v3 := newTestReadOnlyPrefixCacheVFS(t, vfscommon.CacheModeFull, "我的空间")
	r3.WriteObject(context.Background(), "我的空间/a.txt", "hello", time.Now())
	node3, err := v3.Stat("我的空间/a.txt")
	require.NoError(t, err)
	_, err = node3.Open(os.O_WRONLY)
	assert.ErrorIs(t, err, EPERM, "openRW: write open must be refused")
	_, err = node3.Open(os.O_RDWR)
	assert.ErrorIs(t, err, EPERM, "openRW: read+write open must be refused")
	data3, err := v3.ReadFile("我的空间/a.txt")
	require.NoError(t, err, "openRW: reading must still work")
	assert.Equal(t, "hello", string(data3))
}

// TestVFSReadOnlyPrefixesIdempotentMkdir pins the "already exists" branch:
// mkdir of an existing container still returns the existing node (no
// error), and only creating a *new* entry is refused.
func TestVFSReadOnlyPrefixesIdempotentMkdir(t *testing.T) {
	r, v := newTestReadOnlyPrefixVFS(t, "", "协同空间")
	mustMkdirRemote(t, r, "协同空间", "协同空间/test")
	r.WriteObject(context.Background(), "协同空间/a.txt", "leftover", time.Now())

	// Existing container: unchanged behaviour (idempotent, no error)
	require.NoError(t, v.Mkdir("协同空间", 0777), "mkdir of an existing container must stay idempotent")
	require.NoError(t, v.MkdirAll("协同空间", 0777), "mkdir -p of an existing container must stay idempotent")

	root, err := v.Root()
	require.NoError(t, err)
	existing, err := root.Mkdir("协同空间")
	require.NoError(t, err, "Dir.Mkdir of an existing container must return the existing node")
	require.NotNil(t, existing)

	// New entries are refused
	assert.ErrorIs(t, v.Mkdir("协同空间/new", 0777), EPERM, "mkdir of a new dir in a container must be refused")
	assert.ErrorIs(t, v.MkdirAll("协同空间/new/deep", 0777), EPERM, "mkdir -p below a container must be refused")
	_, err = root.Mkdir("11111")
	assert.ErrorIs(t, err, EPERM, "mkdir of a new dir in the root must be refused")

	// Dir.Create on an existing file is idempotent too, and the write open
	// which follows it is what gets refused (this is cmount's CreateEx)
	group, err := v.Stat("协同空间")
	require.NoError(t, err)
	groupDir := group.(*Dir)
	f, err := groupDir.Create("a.txt", os.O_RDWR|os.O_CREATE)
	require.NoError(t, err, "Create of an existing file must return the existing node")
	require.NotNil(t, f)
	_, err = f.Open(os.O_RDWR | os.O_CREATE)
	assert.ErrorIs(t, err, EPERM, "the write open which follows Create must be refused")
	_, err = groupDir.Create("b.txt", os.O_RDWR|os.O_CREATE)
	assert.ErrorIs(t, err, EPERM, "Create of a new file in a container must be refused")
}

// TestReadOnlyPrefixesManifestLiteral pins the on-disk JSON contract with
// literal bytes rather than with the Go struct: the client writes this
// file (plan Task 3), so the key name itself is the interface.
func TestReadOnlyPrefixesManifestLiteral(t *testing.T) {
	dir := t.TempDir()

	// The new manifest the client will write: root only
	fileName := filepath.Join(dir, "prefixes.json")
	require.NoError(t, os.WriteFile(fileName, []byte(`{"paths":[],"prefixes":[""]}`), 0600))
	r := newReadOnlyPaths(fileName)
	assert.True(t, r.ContainsContainerParent("11111"), "a child of the root must be refused")
	assert.True(t, r.ContainsContainerParent("协同空间"))
	assert.False(t, r.ContainsContainerParent("协同空间/test/a.txt"), "a space stays writable")
	assert.False(t, r.ContainsContainer(""), "the root is never a container itself")

	// Literal empty-string prefix with the other three, as the client sends
	fileName2 := filepath.Join(dir, "prefixes2.json")
	require.NoError(t, os.WriteFile(fileName2, []byte(`{"paths":[],"prefixes":["","我的空间","协同空间","公共空间"]}`), 0600))
	r2 := newReadOnlyPaths(fileName2)
	if !assert.True(t, r2.ContainsContainer("公共空间")) {
		t.Fatal(`the "prefixes" key must be parsed - check the json struct tag`)
	}
	assert.True(t, r2.ContainsContainerParent("我的空间/a.txt"))
	assert.False(t, r2.ContainsContainerParent("协同空间/test/a.txt"))

	// The legacy manifest, with no "prefixes" key at all
	fileName3 := filepath.Join(dir, "legacy.json")
	require.NoError(t, os.WriteFile(fileName3, []byte(`{"paths":["a/b.txt"]}`), 0600))
	r3 := newReadOnlyPaths(fileName3)
	assert.True(t, r3.Contains("a/b.txt"), "exact paths still refuse")
	assert.False(t, r3.ContainsContainer("a"), "no prefixes key => nothing is a container")
	assert.False(t, r3.ContainsContainerParent("11111"), "no prefixes key => the root is writable")
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

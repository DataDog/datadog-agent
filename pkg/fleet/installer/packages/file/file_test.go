// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package file

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The trees walked by Permission.Ensure with Recursive are writable by the unprivileged
// dd-agent user, so a symlink planted in them must never redirect these root-run operations
// onto a file outside the tree.

func TestPermissionEnsureRecursiveDoesNotFollowSymlink(t *testing.T) {
	base := t.TempDir()
	tree := filepath.Join(base, "tree")
	require.NoError(t, os.MkdirAll(tree, 0o755))

	victim := filepath.Join(base, "victim")
	require.NoError(t, os.WriteFile(victim, []byte("victim"), 0o600))
	require.NoError(t, os.Symlink(victim, filepath.Join(tree, "planted")))

	permission := Permission{Path: ".", Mode: 0o750, Recursive: true}
	require.NoError(t, permission.Ensure(t.Context(), tree))

	info, err := os.Stat(victim)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"a file outside the walked tree must not be re-permissioned through a planted symlink")
}

func TestPermissionEnsureRecursiveDoesNotFollowSymlinkedDirectory(t *testing.T) {
	base := t.TempDir()
	tree := filepath.Join(base, "tree")
	require.NoError(t, os.MkdirAll(tree, 0o755))

	// WalkDir does not descend into a symlinked directory, so the link itself is the entry the
	// permission is applied to. Resolving it would re-permission a directory outside the tree.
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(outside, 0o700))
	require.NoError(t, os.Chmod(outside, 0o700))
	require.NoError(t, os.Symlink(outside, filepath.Join(tree, "planted")))

	permission := Permission{Path: ".", Mode: 0o750, Recursive: true}
	require.NoError(t, permission.Ensure(t.Context(), tree))

	info, err := os.Stat(outside)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(),
		"a directory outside the walked tree must not be re-permissioned through a planted symlink")
}

func TestPermissionEnsureRecursiveKeepsRegularFiles(t *testing.T) {
	tree := t.TempDir()
	regular := filepath.Join(tree, "regular")
	require.NoError(t, os.WriteFile(regular, []byte("regular"), 0o600))

	permission := Permission{Path: ".", Mode: 0o750, Recursive: true}
	require.NoError(t, permission.Ensure(t.Context(), tree))

	info, err := os.Stat(regular)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o750), info.Mode().Perm(), "regular files must still be re-permissioned")
}

func TestPermissionEnsureRecursiveCoversInTreeSymlinkTargets(t *testing.T) {
	tree := t.TempDir()
	target := filepath.Join(tree, "target")
	require.NoError(t, os.WriteFile(target, []byte("target"), 0o600))
	require.NoError(t, os.Symlink("target", filepath.Join(tree, "link")))

	permission := Permission{Path: ".", Mode: 0o750, Recursive: true}
	require.NoError(t, permission.Ensure(t.Context(), tree))

	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o750), info.Mode().Perm(),
		"a symlink target inside the tree is walked on its own and must keep being covered")
}

func TestPermissionEnsureNamedPathAppliesMode(t *testing.T) {
	// The non-recursive branch is what the Agent package uses for system-probe.yaml and
	// security-agent.yaml, and it goes through the same mode path as the recursive walk.
	dir := t.TempDir()
	named := filepath.Join(dir, "system-probe.yaml")
	require.NoError(t, os.WriteFile(named, []byte("config\n"), 0o600))

	permission := Permission{Path: "system-probe.yaml", Mode: 0o440}
	require.NoError(t, permission.Ensure(t.Context(), dir))

	info, err := os.Stat(named)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o440), info.Mode().Perm())
}

func TestPermissionEnsureNamedPathSkipsMissing(t *testing.T) {
	permission := Permission{Path: "absent.yaml", Mode: 0o440}
	require.NoError(t, permission.Ensure(t.Context(), t.TempDir()))
}

func TestPermissionEnsureRecursiveDoesNotChownSymlinkTarget(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root to change file ownership")
	}
	owner, group := unprivilegedOwner(t)

	base := t.TempDir()
	tree := filepath.Join(base, "tree")
	require.NoError(t, os.MkdirAll(tree, 0o755))

	victim := filepath.Join(base, "victim")
	require.NoError(t, os.WriteFile(victim, []byte("victim"), 0o600))
	require.NoError(t, os.Chown(victim, 0, 0))
	link := filepath.Join(tree, "planted")
	require.NoError(t, os.Symlink(victim, link))

	permission := Permission{Path: ".", Owner: owner, Group: group, Recursive: true}
	require.NoError(t, permission.Ensure(t.Context(), tree))

	assert.Equal(t, uint32(0), ownerUID(t, victim, os.Stat),
		"a file outside the walked tree must not change owner through a planted symlink")
	assert.NotEqual(t, uint32(0), ownerUID(t, link, os.Lstat),
		"the symlink itself is the entry that must be re-owned")
}

// unprivilegedOwner returns a user and group name that exist on the host and are not root.
func unprivilegedOwner(t *testing.T) (string, string) {
	t.Helper()
	var owner, group string
	for _, name := range []string{"nobody", "daemon"} {
		if u, err := user.Lookup(name); err == nil {
			if uid, err := strconv.Atoi(u.Uid); err == nil && uid != 0 {
				owner = name
				break
			}
		}
	}
	for _, name := range []string{"nogroup", "nobody", "daemon"} {
		if g, err := user.LookupGroup(name); err == nil {
			if gid, err := strconv.Atoi(g.Gid); err == nil && gid != 0 {
				group = name
				break
			}
		}
	}
	if owner == "" || group == "" {
		t.Skip("no unprivileged user and group available on this host")
	}
	return owner, group
}

func ownerUID(t *testing.T, path string, stat func(string) (os.FileInfo, error)) uint32 {
	t.Helper()
	info, err := stat(path)
	require.NoError(t, err)
	sys, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok, "unexpected FileInfo.Sys type")
	return sys.Uid
}

func TestUserAndGroupIDsAreCached(t *testing.T) {
	current, err := user.Current()
	require.NoError(t, err)
	group, err := user.LookupGroupId(current.Gid)
	if err != nil {
		t.Skipf("cannot resolve the current group: %v", err)
	}

	userCache.Delete(current.Username)
	groupCache.Delete(group.Name)
	t.Cleanup(func() {
		userCache.Delete(current.Username)
		groupCache.Delete(group.Name)
	})

	uid, gid, err := UserAndGroupIDs(t.Context(), current.Username, group.Name)
	require.NoError(t, err)
	assert.Equal(t, current.Uid, strconv.Itoa(uid))
	assert.Equal(t, current.Gid, strconv.Itoa(gid))

	// Resolving a name forks a getent subprocess, so callers applying ownership per entry must
	// hit these caches rather than the user package.
	cachedUID, ok := userCache.Load(current.Username)
	require.True(t, ok, "the user lookup must be cached")
	assert.Equal(t, uid, cachedUID)
	cachedGID, ok := groupCache.Load(group.Name)
	require.True(t, ok, "the group lookup must be cached")
	assert.Equal(t, gid, cachedGID)
}

// The tests below exercise the nested-os.Root walk rather than a specific vulnerability: it
// runs as root over the whole Agent package tree on every install, so it has to hold up
// against concurrent tampering, depth, width and odd entries.

// stopAfterListing arranges for swap to run once, right after the directory named dir has been
// listed and before any of its entries is acted on.
func stopAfterListing(t *testing.T, dir string, swap func()) {
	t.Helper()
	fired := false
	afterListingDirectory = func(listed string) {
		if fired || filepath.Base(listed) != dir {
			return
		}
		fired = true
		swap()
	}
	t.Cleanup(func() {
		afterListingDirectory = nil
		assert.True(t, fired, "the walk never listed %q, so nothing was proven", dir)
	})
}

// symlinkSwapFixture lays out a tree holding a directory the attacker owns, a decoy inside it
// under the name it wants re-permissioned, and a read-only file outside the tree that the decoy
// name will alias once the directory is swapped for a symlink.
func symlinkSwapFixture(t *testing.T) (tree, swapped, outside, victim string) {
	t.Helper()
	base := t.TempDir()

	outside = filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(outside, 0o755))
	victim = filepath.Join(outside, "passwd")
	require.NoError(t, os.WriteFile(victim, []byte("root:x:0:0:::\n"), 0o600))
	require.NoError(t, os.Chmod(victim, 0o400))

	tree = filepath.Join(base, "tree")
	swapped = filepath.Join(tree, "swapped")
	require.NoError(t, os.MkdirAll(swapped, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(swapped, "passwd"), []byte("decoy\n"), 0o600))
	return tree, swapped, outside, victim
}

// swapForSymlink replaces the directory with a symlink to target, which is what an attacker
// owning that directory can do at any moment.
func swapForSymlink(t *testing.T, dir, target string) {
	t.Helper()
	require.NoError(t, os.RemoveAll(dir))
	require.NoError(t, os.Symlink(target, dir))
}

func TestPermissionEnsureRecursiveRefusesDirectorySwappedAfterListing(t *testing.T) {
	tree, swapped, outside, victim := symlinkSwapFixture(t)

	// The listing has already reported "swapped" as a directory; a path-based pass would go on
	// to act on the entries it recorded under it.
	stopAfterListing(t, "tree", func() { swapForSymlink(t, swapped, outside) })

	permission := Permission{Path: ".", Mode: 0o750, Recursive: true}
	err := permission.Ensure(t.Context(), tree)

	// Checked before the error: this is the assertion that distinguishes a pass that carries
	// path strings from one that carries directory handles.
	info, statErr := os.Stat(victim)
	require.NoError(t, statErr)
	require.Equal(t, os.FileMode(0o400), info.Mode().Perm(),
		"the file the swapped directory points at must be untouched")
	assert.Error(t, err, "the swap must be reported, not walked through")
}

func TestPermissionEnsureRecursiveRefusesNestedDirectorySwappedAfterListing(t *testing.T) {
	tree, swapped, outside, victim := symlinkSwapFixture(t)

	// Same swap one level down, to cover a nested handle rather than the top-level one.
	nested := filepath.Join(swapped, "inner")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "passwd"), []byte("decoy\n"), 0o600))
	stopAfterListing(t, "swapped", func() { swapForSymlink(t, nested, outside) })

	permission := Permission{Path: ".", Mode: 0o750, Recursive: true}
	err := permission.Ensure(t.Context(), tree)

	// Checked before the error: this is the assertion that distinguishes a pass that carries
	// path strings from one that carries directory handles.
	info, statErr := os.Stat(victim)
	require.NoError(t, statErr)
	require.Equal(t, os.FileMode(0o400), info.Mode().Perm(),
		"the file the swapped directory points at must be untouched")
	assert.Error(t, err, "the swap must be reported, not walked through")
}

func TestPermissionEnsureRecursiveSurvivesParentDirectorySwap(t *testing.T) {
	base := t.TempDir()

	// A file outside the tree that must never be touched, whatever the walk is racing with.
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(outside, 0o755))
	victim := filepath.Join(outside, "passwd")
	require.NoError(t, os.WriteFile(victim, []byte("root:x:0:0:::\n"), 0o600))
	// Read-only so the swapper below cannot reach it by writing the decoy through its own
	// symlink; the mode is what this test watches, and only the walk may change it.
	require.NoError(t, os.Chmod(victim, 0o400))

	// Wide enough that a pass takes long enough to be raced.
	tree := filepath.Join(base, "tree")
	for i := range 40 {
		dir := filepath.Join(tree, fmt.Sprintf("dir%02d", i))
		require.NoError(t, os.MkdirAll(dir, 0o755))
		for j := range 20 {
			require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%02d", j)), []byte("x"), 0o600))
		}
	}

	// A directory the attacker owns, holding a decoy under the name it wants re-permissioned.
	swapped := filepath.Join(tree, "swapped")
	plant := func() {
		_ = os.MkdirAll(swapped, 0o755)
		_ = os.WriteFile(filepath.Join(swapped, "passwd"), []byte("decoy\n"), 0o600)
	}
	plant()

	stop := make(chan struct{})
	swapping := make(chan struct{})
	go func() {
		defer close(swapping)
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = os.RemoveAll(swapped)
			_ = os.Symlink(outside, swapped)
			// Held for a moment rather than flipped as fast as possible: a path-based walk is
			// only caught if the link is in place when it applies the entry it recorded.
			time.Sleep(200 * time.Microsecond)
			_ = os.Remove(swapped)
			plant()
		}
	}()

	permission := Permission{Path: ".", Mode: 0o750, Recursive: true}
	// Containment itself is proven deterministically by the two tests above. What this one adds
	// is tolerance of sustained churn: entries vanishing and reappearing mid-walk must neither
	// escape the tree nor be mistaken for a failure. Either outcome is acceptable per pass, and
	// the error text is deliberately not asserted because a hostile filesystem can make the
	// walk refuse in more than one way.
	for range 50 {
		if err := permission.Ensure(t.Context(), tree); err != nil {
			t.Logf("pass reported tampering: %v", err)
		}
		info, statErr := os.Stat(victim)
		require.NoError(t, statErr)
		require.Equal(t, os.FileMode(0o400), info.Mode().Perm(),
			"a swapped parent directory must not redirect the walk outside the tree")
	}
	close(stop)
	<-swapping
}

func TestPermissionEnsureRecursiveDoesNotLeakHandles(t *testing.T) {
	base := t.TempDir()
	deepest := base
	const depth = 40
	for i := range depth {
		deepest = filepath.Join(deepest, fmt.Sprintf("level%02d", i))
	}
	require.NoError(t, os.MkdirAll(deepest, 0o755))
	leaf := filepath.Join(deepest, "leaf")
	require.NoError(t, os.WriteFile(leaf, []byte("x"), 0o600))

	before := openHandles(t)
	permission := Permission{Path: ".", Mode: 0o750, Recursive: true}
	// A handle leaked per level would exhaust the process limit well before the last pass.
	for range 300 {
		require.NoError(t, permission.Ensure(t.Context(), base))
	}
	if after := openHandles(t); before >= 0 && after >= 0 {
		assert.LessOrEqual(t, after, before+2, "descending must not leave handles open")
	}

	info, err := os.Stat(leaf)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o750), info.Mode().Perm(), "the walk must reach the deepest entry")
}

func TestPermissionEnsureRecursiveAppliesModeWithoutSearchBit(t *testing.T) {
	tree := t.TempDir()
	nested := filepath.Join(tree, "sub")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	leaf := filepath.Join(nested, "file")
	require.NoError(t, os.WriteFile(leaf, []byte("x"), 0o600))

	// A mode without the search bit must not stop the descent: entries are handled before the
	// directory holding them.
	permission := Permission{Path: ".", Mode: 0o640, Recursive: true}
	require.NoError(t, permission.Ensure(t.Context(), tree))

	// The directories now lack the search bit, so restore it before reading the result back.
	require.NoError(t, os.Chmod(tree, 0o755))
	require.NoError(t, os.Chmod(nested, 0o755))

	info, err := os.Stat(leaf)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
}

func TestPermissionEnsureRecursiveHandlesMixedEntries(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.WriteFile(outside, []byte("x"), 0o600))

	tree := filepath.Join(base, "tree")
	require.NoError(t, os.MkdirAll(filepath.Join(tree, "empty"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tree, "nested"), 0o755))
	regular := filepath.Join(tree, "nested", "regular")
	require.NoError(t, os.WriteFile(regular, []byte("x"), 0o600))
	require.NoError(t, os.Symlink("nested/regular", filepath.Join(tree, "link-in")))
	require.NoError(t, os.Symlink(outside, filepath.Join(tree, "link-out")))
	require.NoError(t, os.Symlink(filepath.Join(base, "absent"), filepath.Join(tree, "link-dangling")))
	require.NoError(t, syscall.Mkfifo(filepath.Join(tree, "fifo"), 0o600))

	permission := Permission{Path: ".", Mode: 0o750, Recursive: true}
	require.NoError(t, permission.Ensure(t.Context(), tree), "odd entries must not fail the walk")

	for _, path := range []string{regular, filepath.Join(tree, "empty"), filepath.Join(tree, "fifo")} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o750), info.Mode().Perm(), path)
	}
	info, err := os.Stat(outside)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "a symlink target outside the tree stays untouched")
}

func TestPermissionEnsureRecursiveWideDirectory(t *testing.T) {
	tree := t.TempDir()
	const count = 500
	for i := range count {
		require.NoError(t, os.WriteFile(filepath.Join(tree, fmt.Sprintf("file%03d", i)), []byte("x"), 0o600))
	}

	permission := Permission{Path: ".", Mode: 0o750, Recursive: true}
	require.NoError(t, permission.Ensure(t.Context(), tree))

	for _, i := range []int{0, count / 2, count - 1} {
		info, err := os.Stat(filepath.Join(tree, fmt.Sprintf("file%03d", i)))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o750), info.Mode().Perm())
	}
}

func TestPermissionEnsureRecursiveSkipsMissingRoot(t *testing.T) {
	permission := Permission{Path: "absent", Mode: 0o750, Recursive: true}
	require.NoError(t, permission.Ensure(t.Context(), t.TempDir()))
}

func TestPermissionEnsureRecursiveOnRegularFile(t *testing.T) {
	// Recursive on something that is not a directory falls back to the single-entry path
	// rather than failing to open it as a root.
	tree := t.TempDir()
	target := filepath.Join(tree, "file")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o600))

	permission := Permission{Path: "file", Mode: 0o640, Recursive: true}
	require.NoError(t, permission.Ensure(t.Context(), tree))

	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
}

func TestPermissionEnsureNamedPathInSubdirectory(t *testing.T) {
	tree := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tree, "sub"), 0o755))
	target := filepath.Join(tree, "sub", "system-probe.yaml")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o600))

	permission := Permission{Path: "sub/system-probe.yaml", Mode: 0o440}
	require.NoError(t, permission.Ensure(t.Context(), tree))

	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o440), info.Mode().Perm())
}

// openHandles returns the number of file descriptors the process holds, or -1 where that
// cannot be counted.
func openHandles(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return -1
	}
	return len(entries)
}

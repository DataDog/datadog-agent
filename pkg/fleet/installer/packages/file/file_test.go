// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package file

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

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

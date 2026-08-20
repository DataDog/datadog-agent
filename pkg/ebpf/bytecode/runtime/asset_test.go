// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// secureRuntimeDir must reject a cache directory that is not owned by root,
// since only root should be able to write or read the compiled objects stored
// there.
func TestSecureRuntimeDirRejectsNonRootOwned(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test must run as non-root: a root-created dir would legitimately pass")
	}

	dir := filepath.Join(t.TempDir(), "build")
	err := secureRuntimeDir(dir)
	require.Error(t, err, "a non-root-owned cache directory must be rejected")
}

// secureRuntimeDir must reject a directory whose path traverses a symlinked
// component.
func TestSecureRuntimeDirRejectsSymlinkComponent(t *testing.T) {
	base := t.TempDir()

	realDir := filepath.Join(base, "real")
	require.NoError(t, os.MkdirAll(realDir, 0700))

	link := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(realDir, link))

	err := secureRuntimeDir(filepath.Join(link, "build"))
	require.Error(t, err, "a cache directory reached through a symlink must be rejected")
}

// secureRuntimeDir must repair a non-root-owned component that sits below a
// root-owned sticky directory (as with a pre-existing datadog-agent directory
// under /var/tmp) by moving it aside and recreating the path as root, instead of
// refusing for the lifetime of the process. This requires root to create the
// non-root-owned component, so it runs only as root (as eBPF CI suites do).
func TestSecureRuntimeDirReclaimsNonRootComponent(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test must run as root to create a non-root-owned directory")
	}

	// Root-owned sticky parent standing in for /var/tmp.
	sticky := filepath.Join(t.TempDir(), "sticky")
	require.NoError(t, os.Mkdir(sticky, 0777))
	require.NoError(t, os.Chmod(sticky, 0777|os.ModeSticky))

	// A pre-existing datadog-agent directory owned by a non-root user.
	preexisting := filepath.Join(sticky, "datadog-agent")
	require.NoError(t, os.Mkdir(preexisting, 0777))
	require.NoError(t, syscall.Chown(preexisting, 1, 1))

	build := filepath.Join(preexisting, "system-probe", "build")
	require.NoError(t, secureRuntimeDir(build), "secureRuntimeDir should repair the non-root component and succeed")

	// The repaired component must now be a root-owned directory.
	info, err := os.Lstat(preexisting)
	require.NoError(t, err)
	require.True(t, info.IsDir())
	stat, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	require.Equal(t, uint32(0), stat.Uid, "repaired component must be owned by root")
}

// stickyParent returns a fresh root-owned sticky directory standing in for
// /var/tmp, and a build path beneath a datadog-agent component under it.
func stickyParent(t *testing.T) (sticky, buildPath string) {
	t.Helper()
	sticky = filepath.Join(t.TempDir(), "sticky")
	require.NoError(t, os.Mkdir(sticky, 0777))
	require.NoError(t, os.Chmod(sticky, 0777|os.ModeSticky))
	return sticky, filepath.Join(sticky, "datadog-agent", "system-probe", "build")
}

func dirUID(t *testing.T, p string) uint32 {
	t.Helper()
	info, err := os.Lstat(p)
	require.NoError(t, err)
	stat, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	return stat.Uid
}

func dirIno(t *testing.T, p string) uint64 {
	t.Helper()
	info, err := os.Lstat(p)
	require.NoError(t, err)
	stat, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	return stat.Ino
}

// hasReclaimedLeftover reports whether any component under root retains a
// moved-aside ".reclaimed-" directory, which must not survive a successful run.
func hasReclaimedLeftover(t *testing.T, root string) bool {
	t.Helper()
	found := false
	require.NoError(t, filepath.Walk(root, func(p string, _ os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if strings.Contains(p, ".reclaimed-") {
			found = true
		}
		return nil
	}))
	return found
}

// B1: an absent directory is created root-owned and 0700.
func TestSecureRuntimeDirCreatesRootOwnedWhenAbsent(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test must run as root")
	}
	_, build := stickyParent(t)

	require.NoError(t, secureRuntimeDir(build))
	require.Equal(t, uint32(0), dirUID(t, build), "created dir must be root-owned")
	info, err := os.Lstat(build)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0700), info.Mode().Perm(), "created dir must be 0700")
}

// B2: a normal upgrade over a pre-existing root-owned tree is accepted without
// any reclaim (the inode is preserved).
func TestSecureRuntimeDirAcceptsPreexistingRootDir(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test must run as root")
	}
	sticky, build := stickyParent(t)
	require.NoError(t, os.MkdirAll(build, 0755))
	da := filepath.Join(sticky, "datadog-agent")
	require.NoError(t, os.Chmod(da, 0755))
	before := dirIno(t, da)

	require.NoError(t, secureRuntimeDir(build))
	// A valid root-owned dir must be left untouched: same inode and same mode
	// (a reclaim would recreate it 0700).
	require.Equal(t, before, dirIno(t, da))
	info, err := os.Lstat(da)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0755), info.Mode().Perm(), "a valid dir must not be reclaimed to 0700")
	require.False(t, hasReclaimedLeftover(t, sticky))
}

// B4: a non-root-owned deep component (not the top datadog-agent one) is
// repaired too.
func TestSecureRuntimeDirRepairsDeepNonRootComponent(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test must run as root to create a non-root-owned directory")
	}
	sticky, build := stickyParent(t)
	sp := filepath.Join(sticky, "datadog-agent", "system-probe")
	require.NoError(t, os.MkdirAll(sp, 0755))
	require.NoError(t, syscall.Chown(sp, 1, 1))

	require.NoError(t, secureRuntimeDir(build))
	require.Equal(t, uint32(0), dirUID(t, sp), "deep non-root component must be repaired to root")
	require.False(t, hasReclaimedLeftover(t, sticky))
}

// B5: a group/other-writable, non-sticky directory below the sticky boundary is
// repaired (it violates the policy even though it is root-owned).
func TestSecureRuntimeDirRepairsWritableNonStickyDir(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test must run as root")
	}
	sticky, build := stickyParent(t)
	da := filepath.Join(sticky, "datadog-agent")
	require.NoError(t, os.Mkdir(da, 0700))
	require.NoError(t, os.Chmod(da, 0777)) // explicit chmod bypasses umask: root-owned, world-writable, no sticky

	require.NoError(t, secureRuntimeDir(build))
	require.Equal(t, uint32(0), dirUID(t, da))
	// A reclaim recreates the component 0700; if it were wrongly accepted as-is
	// it would still be 0777. (Inode numbers are unreliable here: ext4 recycles
	// them, so a recreated dir can reuse the freed number.)
	info, err := os.Lstat(da)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0700), info.Mode().Perm(), "writable non-sticky dir must be reclaimed to 0700")
}

// B9: running twice is idempotent - the second run finds a valid tree, repairs
// nothing, and leaves no moved-aside directories behind.
func TestSecureRuntimeDirIdempotent(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test must run as root to create a non-root-owned directory")
	}
	sticky, build := stickyParent(t)
	da := filepath.Join(sticky, "datadog-agent")
	require.NoError(t, os.Mkdir(da, 0777))
	require.NoError(t, syscall.Chown(da, 1, 1))

	require.NoError(t, secureRuntimeDir(build)) // repairs
	inoAfterFirst := dirIno(t, da)
	require.NoError(t, secureRuntimeDir(build)) // no-op
	require.Equal(t, inoAfterFirst, dirIno(t, da), "second run must not reclaim again")
	require.False(t, hasReclaimedLeftover(t, sticky), "no moved-aside dirs may remain")
}

// B8: a non-root component with NO sticky ancestor is refused, never repaired.
// Uses /root as a non-sticky root-only base (t.TempDir lives under sticky /tmp).
func TestSecureRuntimeDirRefusesNonRootWithoutStickyAncestor(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test must run as root to create the non-root component")
	}
	base, err := os.MkdirTemp("/root", "sp-rc-nosticky-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(base) })

	parent := filepath.Join(base, "parent")
	require.NoError(t, os.Mkdir(parent, 0755))
	require.NoError(t, syscall.Chown(parent, 1, 1))

	err = secureRuntimeDir(filepath.Join(parent, "build"))
	require.Error(t, err, "a non-root component with no sticky ancestor must be refused, not repaired")
	require.Equal(t, uint32(1), dirUID(t, parent), "the component must be left untouched, not reclaimed")
}

// A non-root component that sits below a sticky boundary but OUTSIDE the agent's
// dedicated (datadog-agent) subtree must be refused, never reclaimed. Reclaiming
// it would rename the shared parent aside and recursively delete it along with
// unrelated contents, so this guards that data-loss regression for a custom
// output_dir nested under a shared directory (e.g. /var/tmp/shared/datadog/build).
func TestSecureRuntimeDirRefusesSharedNonDedicatedAncestor(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test must run as root to create the non-root component")
	}

	// Root-owned sticky parent standing in for /var/tmp.
	sticky := filepath.Join(t.TempDir(), "sticky")
	require.NoError(t, os.Mkdir(sticky, 0777))
	require.NoError(t, os.Chmod(sticky, 0777|os.ModeSticky))

	// A shared, non-root-owned directory that is NOT named datadog-agent and
	// holds unrelated data.
	shared := filepath.Join(sticky, "shared")
	require.NoError(t, os.Mkdir(shared, 0777))
	require.NoError(t, syscall.Chown(shared, 1, 1))
	bystander := filepath.Join(shared, "unrelated.txt")
	require.NoError(t, os.WriteFile(bystander, []byte("keep me"), 0644))

	err := secureRuntimeDir(filepath.Join(shared, "datadog", "build"))
	require.Error(t, err, "a non-dedicated shared ancestor must be refused, not reclaimed")
	require.FileExists(t, bystander, "unrelated contents must be preserved")
	require.Equal(t, uint32(1), dirUID(t, shared), "the shared dir must be left untouched, not reclaimed")
	require.False(t, hasReclaimedLeftover(t, sticky))
}

// A group/other-writable leaf output directory inside the agent's own subtree is
// repaired to 0700, even when it carries the sticky bit (which is tolerated on
// ancestors but not on the leaf, where object files are written under predictable
// names).
func TestSecureRuntimeDirRepairsWritableStickyLeaf(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test must run as root")
	}
	_, build := stickyParent(t)
	require.NoError(t, os.MkdirAll(build, 0700))
	// Pre-existing leaf that is root-owned but sticky + world-writable.
	require.NoError(t, os.Chmod(build, 0777|os.ModeSticky))

	require.NoError(t, secureRuntimeDir(build))
	require.Equal(t, uint32(0), dirUID(t, build), "leaf must be root-owned")
	info, err := os.Lstat(build)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0700), info.Mode().Perm(), "writable leaf must be repaired to 0700")
	require.Equal(t, os.FileMode(0), info.Mode()&os.ModeSticky, "sticky bit must be gone after repair")
}

// A root-owned but group/other-writable leaf that is NOT inside the agent's
// dedicated subtree must be refused (not reclaimed): tightening or deleting a
// shared/system directory the agent does not own would be destructive, so it
// fails closed instead.
func TestSecureRuntimeDirRefusesWritableLeafOutsideSubtree(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test must run as root")
	}
	// t.TempDir lives under sticky /tmp, so there is a sticky ancestor, but no
	// component is named datadog-agent.
	base := filepath.Join(t.TempDir(), "shared")
	require.NoError(t, os.Mkdir(base, 0755))
	leaf := filepath.Join(base, "build")
	require.NoError(t, os.Mkdir(leaf, 0700))
	require.NoError(t, os.Chmod(leaf, 0777|os.ModeSticky)) // root-owned, sticky, world-writable

	err := secureRuntimeDir(leaf)
	require.Error(t, err, "a writable leaf outside the dedicated subtree must be refused")
	info, lerr := os.Lstat(leaf)
	require.NoError(t, lerr)
	require.Equal(t, os.FileMode(0777), info.Mode().Perm(), "the leaf must be left untouched, not tightened")
	require.NotEqual(t, os.FileMode(0), info.Mode()&os.ModeSticky, "the leaf must be left untouched, not reclaimed")
	require.False(t, hasReclaimedLeftover(t, base))
}

// verifyDirComponent is the pure policy behind the ancestor walk in
// secureRuntimeDir. Exercising it directly covers the accept/reject logic
// regardless of the euid the suite runs under (the filesystem-level test above
// skips when run as root, which is common for eBPF suites).
func TestVerifyDirComponent(t *testing.T) {
	const (
		rootUID    uint32 = 0
		nonRootUID uint32 = 1000
	)
	tests := []struct {
		name    string
		mode    os.FileMode
		uid     uint32
		isLeaf  bool
		wantErr bool
	}{
		{"root-owned 0700 dir", os.ModeDir | 0700, rootUID, false, false},
		{"root-owned 0755 dir", os.ModeDir | 0755, rootUID, false, false},
		{"root-owned sticky 1777 dir (/var/tmp)", os.ModeDir | os.ModeSticky | 0777, rootUID, false, false},
		{"symlink", os.ModeSymlink | 0777, rootUID, false, true},
		{"regular file", 0644, rootUID, false, true},
		{"non-root-owned dir", os.ModeDir | 0700, nonRootUID, false, true},
		{"group-writable dir without sticky", os.ModeDir | 0770, rootUID, false, true},
		{"world-writable dir without sticky", os.ModeDir | 0777, rootUID, false, true},
		// The leaf output directory is held to a stricter rule: no group/other
		// write bits even when the sticky bit is set.
		{"leaf root-owned 0700 dir", os.ModeDir | 0700, rootUID, true, false},
		{"leaf root-owned 0755 dir", os.ModeDir | 0755, rootUID, true, false},
		{"leaf sticky 1777 dir", os.ModeDir | os.ModeSticky | 0777, rootUID, true, true},
		{"leaf group-writable sticky dir", os.ModeDir | os.ModeSticky | 0770, rootUID, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyDirComponent("/some/path", tc.mode, tc.uid, tc.isLeaf)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

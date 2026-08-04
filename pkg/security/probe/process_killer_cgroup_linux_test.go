// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// dirInode returns the inode of the given directory, as the cgroup resolver would have recorded it.
func dirInode(t *testing.T, path string) uint64 {
	t.Helper()

	var stat syscall.Stat_t
	require.NoError(t, syscall.Stat(path, &stat))
	return stat.Ino
}

// fakeCgroup creates a directory tree mimicking a cgroup, with a writable cgroup.kill file,
// and returns the cgroup directory and its inode.
func fakeCgroup(t *testing.T, base string, cgroupID containerutils.CGroupID) (string, uint64) {
	t.Helper()

	dir := filepath.Join(base, string(cgroupID))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, cgroupKillFile), nil, 0o600))
	return dir, dirInode(t, dir)
}

func TestCgroupKillerWritesOneToCgroupKillFile(t *testing.T) {
	base := t.TempDir()
	dir, inode := fakeCgroup(t, base, "/kubepods/podabc")

	killer := &cgroupKiller{bases: []string{base}}
	require.NoError(t, killer.kill(cgroupKillTarget{id: "/kubepods/podabc", inode: inode}))

	content, err := os.ReadFile(filepath.Join(dir, cgroupKillFile))
	require.NoError(t, err)
	assert.Equal(t, "1", string(content), "the kernel only accepts \"1\" on cgroup.kill")
}

func TestCgroupKillerFallsThroughToNextBase(t *testing.T) {
	// The first base is the agent's own view of cgroupfs, which is read-only in the
	// Kubernetes manifests; the second one reaches the same cgroup through pid 1's root.
	unusableBase, usableBase := t.TempDir(), t.TempDir()
	dir, inode := fakeCgroup(t, usableBase, "/kubepods/podabc")

	killer := &cgroupKiller{bases: []string{unusableBase, usableBase}}
	require.NoError(t, killer.kill(cgroupKillTarget{id: "/kubepods/podabc", inode: inode}))

	content, err := os.ReadFile(filepath.Join(dir, cgroupKillFile))
	require.NoError(t, err)
	assert.Equal(t, "1", string(content))
}

func TestCgroupKillerErrorsWhenCgroupKillIsMissing(t *testing.T) {
	// Happens on kernels older than 5.14, and on the root cgroup. The caller must fall
	// back to killing each process individually.
	base := t.TempDir()
	dir := filepath.Join(base, "kubepods", "podabc")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	killer := &cgroupKiller{bases: []string{base}}
	assert.Error(t, killer.kill(cgroupKillTarget{id: "/kubepods/podabc", inode: dirInode(t, dir)}))
}

func TestCgroupKillerRefusesInodeMismatch(t *testing.T) {
	// The cgroup ID is a path resolved from kernel state; if it no longer points at the
	// cgroup CWS resolved, killing it would take down an unrelated workload.
	base := t.TempDir()
	dir, inode := fakeCgroup(t, base, "/kubepods/podabc")

	killer := &cgroupKiller{bases: []string{base}}
	assert.Error(t, killer.kill(cgroupKillTarget{id: "/kubepods/podabc", inode: inode + 1}))

	content, err := os.ReadFile(filepath.Join(dir, cgroupKillFile))
	require.NoError(t, err)
	assert.Empty(t, content, "nothing should have been written to cgroup.kill")
}

func TestCgroupKillerRefusesAgentOwnCgroup(t *testing.T) {
	base := t.TempDir()
	_, agentInode := fakeCgroup(t, base, "/kubepods/podagent/agent")
	_, podInode := fakeCgroup(t, base, "/kubepods/podagent")
	_, otherInode := fakeCgroup(t, base, "/kubepods/podother")

	killer := &cgroupKiller{bases: []string{base}, selfCGroupID: "/kubepods/podagent/agent"}

	// cgroup.kill also kills descendant cgroups, so an ancestor of our own cgroup is
	// just as fatal as our own cgroup.
	assert.Error(t, killer.kill(cgroupKillTarget{id: "/kubepods/podagent/agent", inode: agentInode}), "must refuse its own cgroup")
	assert.Error(t, killer.kill(cgroupKillTarget{id: "/kubepods/podagent", inode: podInode}), "must refuse an ancestor of its own cgroup")
	assert.NoError(t, killer.kill(cgroupKillTarget{id: "/kubepods/podother", inode: otherInode}), "unrelated cgroups must still be killable")
}

func TestCgroupKillerRejectsUnsafeCgroupIDs(t *testing.T) {
	base := t.TempDir()
	_, inode := fakeCgroup(t, base, "/kubepods/podabc")
	killer := &cgroupKiller{bases: []string{base}}

	for _, cgroupID := range []containerutils.CGroupID{
		"",                       // unresolved cgroup
		"/",                      // the root cgroup has no cgroup.kill, and killing it would kill the host
		"/kubepods/../../escape", // must not resolve outside of the cgroup mount point
	} {
		t.Run(string(cgroupID), func(t *testing.T) {
			assert.Error(t, killer.kill(cgroupKillTarget{id: cgroupID, inode: inode}))
		})
	}
}

func TestCgroupKillWriteBasesPrefersAgentViewThenHostRoot(t *testing.T) {
	bases := cgroupKillWriteBases("/host/sys/fs/cgroup", "/sys/fs/cgroup", "/host/proc")
	assert.Equal(t, []string{"/host/sys/fs/cgroup", "/host/proc/1/root/sys/fs/cgroup"}, bases)

	// On a host install both views are the same path, so there is nothing to fall back to.
	bases = cgroupKillWriteBases("/sys/fs/cgroup", "/sys/fs/cgroup", "/proc")
	assert.Equal(t, []string{"/sys/fs/cgroup", "/proc/1/root/sys/fs/cgroup"}, bases)

	// Without a cgroup2 mount point in the host mount namespace there is no fallback.
	bases = cgroupKillWriteBases("/host/sys/fs/cgroup", "", "/host/proc")
	assert.Equal(t, []string{"/host/sys/fs/cgroup"}, bases)
}

// TestCgroupKillerKillsRealCgroup exercises cgroup.kill against a real cgroup v2 hierarchy.
func TestCgroupKillerKillsRealCgroup(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root to create a cgroup and write cgroup.kill")
	}
	if !utils.IsPureCGroupV2Available() {
		t.Skip("requires a pure cgroup v2 hierarchy")
	}

	mountPoint, err := utils.GetCgroup2MountPoint()
	require.NoError(t, err)
	require.NotEmpty(t, mountPoint)

	// Nest the test cgroup under our own so we inherit whatever delegation is in place.
	selfID, err := selfCGroupID()
	require.NoError(t, err)

	cgroupID := containerutils.CGroupID(filepath.Join(string(selfID), "cws-cgroup-kill-test"))
	dir := filepath.Join(mountPoint, string(cgroupID))
	require.NoError(t, os.Mkdir(dir, 0o755))
	defer os.Remove(dir)

	cmd := exec.Command("/bin/sleep", "300")
	require.NoError(t, cmd.Start())
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "cgroup.procs"), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600))

	killer := &cgroupKiller{bases: []string{mountPoint}}
	require.NoError(t, killer.kill(cgroupKillTarget{id: cgroupID, inode: dirInode(t, dir)}))

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		require.Error(t, err, "the process should have been killed")
		exitErr, ok := err.(*exec.ExitError)
		require.True(t, ok)
		status, ok := exitErr.Sys().(syscall.WaitStatus)
		require.True(t, ok)
		assert.Equal(t, syscall.SIGKILL, status.Signal(), "cgroup.kill delivers SIGKILL")
	case <-time.After(5 * time.Second):
		t.Fatal("process was not killed by cgroup.kill")
	}
}

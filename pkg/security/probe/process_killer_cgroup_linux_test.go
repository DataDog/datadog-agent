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
	"strings"
	"syscall"
	"testing"

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

func TestCgroupKillerRefusesCgroupWithDescendants(t *testing.T) {
	// cgroup.kill also kills the processes of descendant cgroups, and those are not part of the
	// pid list the caller checked against the excluded binaries. Killing only what has been
	// checked means leaving these cgroups to the per-process path.
	base := t.TempDir()
	dir, inode := fakeCgroup(t, base, "/kubepods/podabc")
	fakeCgroup(t, base, "/kubepods/podabc/child")

	killer := &cgroupKiller{bases: []string{base}}
	assert.Error(t, killer.kill(cgroupKillTarget{id: "/kubepods/podabc", inode: inode}))

	content, err := os.ReadFile(filepath.Join(dir, cgroupKillFile))
	require.NoError(t, err)
	assert.Empty(t, content, "nothing should have been written to cgroup.kill")
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
// Everything the environment has to provide is a skip rather than a failure: containerized
// runners are usually confined to a cgroupfs they cannot create cgroups in.
func TestCgroupKillerKillsRealCgroup(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root to create a cgroup and write cgroup.kill")
	}
	if !utils.IsPureCGroupV2Available() {
		t.Skip("requires a pure cgroup v2 hierarchy")
	}

	mountPoint, err := utils.GetCgroup2MountPoint()
	if err != nil || mountPoint == "" {
		t.Skipf("requires a cgroup2 mount point: %v", err)
	}

	// Nest the test cgroup under our own when we can resolve it, so that we inherit whatever
	// delegation is in place. When we can't, the mount point is the closest thing we have.
	parent := mountPoint
	if selfID, err := selfCGroupID(); err == nil {
		parent = filepath.Join(mountPoint, string(selfID))
	}

	dir := filepath.Join(parent, "cws-cgroup-kill-test")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Skipf("requires a writable cgroupfs: %v", err)
	}
	defer os.Remove(dir)

	if _, err := os.Stat(filepath.Join(dir, cgroupKillFile)); err != nil {
		t.Skipf("requires cgroup.kill, added in Linux 5.14: %v", err)
	}

	cmd := exec.Command("/bin/sleep", "300")
	require.NoError(t, cmd.Start())
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	if err := os.WriteFile(filepath.Join(dir, "cgroup.procs"), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		t.Skipf("unable to move a process into the test cgroup: %v", err)
	}

	killer := &cgroupKiller{bases: []string{mountPoint}}
	cgroupID := containerutils.CGroupID(strings.TrimPrefix(dir, mountPoint))
	require.NoError(t, killer.kill(cgroupKillTarget{id: cgroupID, inode: dirInode(t, dir)}))

	// cgroup.kill has queued the SIGKILL by the time the write returns, so this only waits for
	// the process to be reaped. A hang is caught by the test timeout.
	err = cmd.Wait()
	require.Error(t, err, "the process should have been killed")
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	require.True(t, ok)
	assert.Equal(t, syscall.SIGKILL, status.Signal(), "cgroup.kill delivers SIGKILL")
}

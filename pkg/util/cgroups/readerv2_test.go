// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReaderV2(t *testing.T) {
	fakeFsPath := t.TempDir()
	paths := []string{
		"kubepods.slice/kubepods-besteffort.slice/kubepods-besteffort-podb3922967_14e1_4867_9388_461bac94b37e.slice/crio-2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc40.scope",
		"kubepods/kubepods-besteffort/kubepods-besteffort-podb3922967_14e1_4867_9388_461bac94b37e/2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc41",
		"system.slice/run-containerd-io.containerd.runtime.v2.task-k8s.io-2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc42-rootfs.mount",
		"kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podc704ef4c297ab11032b83ce52cbfc87b.slice/cri-containerd-2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc42.scope",
		"libpod_parent/libpod-6dc3fdffbf66b1239d55e98da9aaa759ea51ed35d04eb09d19ebd78963aa26c2/system.slice/var-lib-docker-containers-1575e8b4a92a9c340a657f3df4ddc0f6a6305c200879f3898b26368ad019b503-mounts-shm.mount",
		"libpod_parent/libpod-6dc3fdffbf66b1239d55e98da9aaa759ea51ed35d04eb09d19ebd78963aa26c2/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-poda2acd1bccd50fd7790183537181f658e.slice/docker-1575e8b4a92a9c340a657f3df4ddc0f6a6305c200879f3898b26368ad019b503.scope",
	}

	// Create mock directories for paths and corresponding inodes.
	for _, p := range paths {
		fullPath := filepath.Join(fakeFsPath, p)
		assert.NoErrorf(t, os.MkdirAll(fullPath, 0o750), "impossible to create temp directory '%s'", fullPath)
	}

	assert.NoError(t, os.WriteFile(filepath.Join(fakeFsPath, "cgroup.controllers"), []byte("cpu io memory"), 0o640))

	controllers := map[string]struct{}{
		"cpu":    {},
		"io":     {},
		"memory": {},
	}

	r, err := newReaderV2("", fakeFsPath, ContainerFilter, "")
	r.pidMapper = nil
	assert.NoError(t, err)
	assert.NotNil(t, r)

	cgroups, _, err := r.parseCgroups()
	assert.NoError(t, err)

	expected := map[string]Cgroup{
		"2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc40": newCgroupV2("2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc40", fakeFsPath, paths[0], controllers, r.pidMapper),
		"2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc41": newCgroupV2("2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc41", fakeFsPath, paths[1], controllers, r.pidMapper),
		"2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc42": newCgroupV2("2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc42", fakeFsPath, paths[3], controllers, r.pidMapper),
		"1575e8b4a92a9c340a657f3df4ddc0f6a6305c200879f3898b26368ad019b503": newCgroupV2("1575e8b4a92a9c340a657f3df4ddc0f6a6305c200879f3898b26368ad019b503", fakeFsPath, paths[5], controllers, r.pidMapper),
		"6dc3fdffbf66b1239d55e98da9aaa759ea51ed35d04eb09d19ebd78963aa26c2": newCgroupV2("6dc3fdffbf66b1239d55e98da9aaa759ea51ed35d04eb09d19ebd78963aa26c2", fakeFsPath, "libpod_parent/libpod-6dc3fdffbf66b1239d55e98da9aaa759ea51ed35d04eb09d19ebd78963aa26c2", controllers, r.pidMapper),
	}

	// Initialize Inodes
	for i := range cgroups {
		inode := cgroups[i].Inode()
		assert.NotEqual(t, uint64(0), inode)
	}
	for _, cgroup := range expected {
		inode := cgroup.Inode()
		assert.NotEqual(t, uint64(0), inode)
	}

	assert.Empty(t, cmp.Diff(expected, cgroups, cmp.AllowUnexported(cgroupV2{})))
}

// TestCRIOSubCgroupInodeResolution verifies that DogStatsD origin detection resolves
// a container when the client reports the inode of a CRI-O sub-cgroup directory
// (.scope/container/) rather than the parent .scope/ directory.
func TestCRIOSubCgroupInodeResolution(t *testing.T) {
	fakeFsPath := t.TempDir()

	containerID := "2327a2aec169e25cf05f2a901486b7463fdb513ae097fc0ae6a3ca94381ddc40"
	scopeRelPath := filepath.Join(
		"kubepods.slice/kubepods-besteffort.slice/kubepods-besteffort-podb3922967_14e1_4867_9388_461bac94b37e.slice",
		"crio-"+containerID+".scope",
	)
	containerPath := filepath.Join(fakeFsPath, scopeRelPath, "container")

	require.NoError(t, os.MkdirAll(containerPath, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(fakeFsPath, "cgroup.controllers"), []byte("cpu io memory"), 0o640))

	// containerInode is the inode of .scope/container/ — what DogStatsD clients report
	// from inside their cgroup namespace.
	containerInode := inodeForPath(containerPath)
	require.NotEqual(t, unknownInode, containerInode)

	// Sanity: the parent .scope/ inode is distinct.
	scopeInode := inodeForPath(filepath.Join(fakeFsPath, scopeRelPath))
	require.NotEqual(t, unknownInode, scopeInode)
	require.NotEqual(t, scopeInode, containerInode)

	impl, err := newReaderV2("", fakeFsPath, ContainerFilter, "")
	require.NoError(t, err)
	impl.pidMapper = nil

	reader := &Reader{impl: impl}
	require.NoError(t, reader.RefreshCgroups(0))

	// The parent container must be discoverable by its ID.
	expectedCg := reader.GetCgroup(containerID)
	require.NotNil(t, expectedCg, "container should be registered by ID")

	// The sub-cgroup inode must resolve to the same container.
	cg := reader.GetCgroupByInode(containerInode)
	require.NotNil(t, cg, "container/ sub-cgroup inode should resolve to the parent container")
	assert.Equal(t, expectedCg, cg)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && test

// Package testutil holds different utilities and stubs for testing
package testutil

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// FakeCgroup defines the data used to create a fake cgroup.
type FakeCgroup struct {
	// Name is the name of the cgroup (last component of the cgroup full path)
	Name string
	// Parent is the parent cgroup of this cgroup, nil for the root cgroup
	Parent *FakeCgroup
	// PIDs is the list of PIDs that are in this cgroup
	PIDs []int
	// VisibleInContainerNamespace is true if this cgroup is visible in the
	// container namespace. This will mount the host cgroup folder in the
	// container cgroup namespace
	VisibleInContainerNamespace bool
	// IsContainerRoot is true if this cgroup is the root cgroup of the
	// container. Only one cgroup can be the container root.
	IsContainerRoot bool
	// IsHostRoot is true if this cgroup is the root cgroup of the host. Only
	// one cgroup can be the host root. There must always be a host root cgroup.
	IsHostRoot bool
}

// FullName returns the full path of the cgroup, including the name of the cgroup
// and all its parents.
func (c *FakeCgroup) FullName() string {
	if c.Parent == nil {
		return "/" + c.Name
	}

	return filepath.Join(c.Parent.FullName(), c.Name)
}

// createFiles creates the cgroup directory in the namespace, and the bindmount
// if the cgroup needs to be visible in the container namespace.
func (c *FakeCgroup) createFiles(tb testing.TB, fs *FakeCgroupFilesystem) {
	fullPath := filepath.Join(fs.HostCgroupFsPath, c.FullName())
	require.NoError(tb, os.MkdirAll(fullPath, 0755), "cannot create cgroup directory at %s", fullPath)

	tb.Logf("cgroup %s: new cgroup folder: %s", c.Name, fullPath)

	cgroupProcs := filepath.Join(fullPath, "cgroup.procs")
	cgroupProcsFile, err := os.Create(cgroupProcs)
	require.NoError(tb, err, "cannot create cgroup.procs file at %s", cgroupProcs)
	defer cgroupProcsFile.Close()

	for _, pid := range c.PIDs {
		_, err := cgroupProcsFile.WriteString(strconv.Itoa(pid) + "\n")
		require.NoError(tb, err, "cannot write pid %d to cgroup.procs file at %s", pid, cgroupProcs)
	}

	if c.VisibleInContainerNamespace {
		containerCgroupPath := filepath.Join(fs.ContainerCgroupFsPath, c.FullName())
		require.NoError(tb, os.MkdirAll(containerCgroupPath, 0755), "cannot create cgroup directory at %s", containerCgroupPath)

		hostCgroupPath := filepath.Join(fs.HostCgroupFsPath, c.FullName())
		err := unix.Mount(hostCgroupPath, containerCgroupPath, "bind", unix.MS_BIND, "")
		// If we get permission denied when trying a bind mount inside our temporary directory,
		// it probably means we're running in a container and the bind mount is not allowed.
		if errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) {
			tb.Skip("Test requires privileges to bind mount our test directories")
		}
		require.NoError(tb, err, "cannot bind mount cgroup %s at %s", c.FullName(), containerCgroupPath)

		tb.Cleanup(func() {
			require.NoError(tb, unix.Unmount(containerCgroupPath, unix.MNT_DETACH))
		})

		// Sanity check that the inodes of the source and destination cgroup are the same
		var containerCgroupStat, hostCgroupPathStat unix.Stat_t
		require.NoError(tb, unix.Stat(containerCgroupPath, &containerCgroupStat))
		require.NoError(tb, unix.Stat(hostCgroupPath, &hostCgroupPathStat))
		require.Equal(tb, containerCgroupStat.Ino, hostCgroupPathStat.Ino, "the inodes should be the same, something is wrong with the bind mount")

		tb.Logf("cgroup %s: bindmount %s -> %s", c.Name, hostCgroupPath, containerCgroupPath)
	}
}

// FakeCgroupFilesystem is the result of calling CreateFakeCgroupFilesystem, contains
// the paths to the different parts of the created cgroup filesystem structure assuming
// a containerized environment that gets access to the host root filesystem.
type FakeCgroupFilesystem struct {
	// Root is the root of the temporary directory. This mocks the entire filesystem root (/)
	Root string
	// HostRootMountpoint is the mountpoint of the host root filesystem, relative to Root (/host)
	HostRootMountpoint string
	// HostRoot is the absolute path to the host root filesystem (Root + HostRootMountpoint)
	HostRoot string
	// ContainerCgroupFsPath is the path to the cgroup filesystem inside the container (Root + /sys/fs/cgroup)
	ContainerCgroupFsPath string
	// HostCgroupFsPath is the path to the cgroup filesystem inside the host (HostRoot + /sys/fs/cgroup)
	HostCgroupFsPath string
	// HostProc is the path to the proc filesystem inside the host (HostRoot + /proc)
	HostProc string
	// ContainerProc is the path to the proc filesystem inside the container (Root + /proc)
	ContainerProc string
}

// SetupTestEnvvars sets the appropriate environment variables to use the fake cgroup filesystem
// in the test, with proper cleanup.
func (fs *FakeCgroupFilesystem) SetupTestEnvvars(tb testing.TB) {
	kernel.WithFakeProcFS(tb, fs.HostProc)
}

// CreateFakeCgroupFilesystem creates a fake filesystem that contains the given cgroups
// It will create a new temporary directory, with cgroupfs and procfs folders below it.
// It will also create a structure similar to that mounted in containers, with /host
// simulating the host root filesystem.
func CreateFakeCgroupFilesystem(tb testing.TB, cgroups []FakeCgroup) *FakeCgroupFilesystem {
	var fs FakeCgroupFilesystem
	fs.Root = tb.TempDir()

	fs.HostRootMountpoint = "/host"
	fs.HostRoot = filepath.Join(fs.Root, fs.HostRootMountpoint)

	fs.HostProc = filepath.Join(fs.HostRoot, "proc")
	fs.ContainerProc = filepath.Join(fs.Root, "proc")

	fs.ContainerCgroupFsPath = createBaseCgroupfs(tb, fs.Root)
	fs.HostCgroupFsPath = createBaseCgroupfs(tb, fs.HostRoot)

	var hostRootCgroup, containerRootCgroup *FakeCgroup
	hasContainerCgroups := false

	// Detect the root cgroups
	for _, cgroup := range cgroups {
		if cgroup.IsHostRoot {
			hostRootCgroup = &cgroup
			require.Equal(tb, "/", hostRootCgroup.FullName(), "host root cgroup must be at root")
			require.Equal(tb, "", hostRootCgroup.Name, "host root cgroup must not have a name")
		}

		if cgroup.IsContainerRoot {
			containerRootCgroup = &cgroup
			require.True(tb, cgroup.VisibleInContainerNamespace, "container root cgroup must be visible in container namespace")
		}

		if cgroup.VisibleInContainerNamespace {
			hasContainerCgroups = true
		}
	}

	if hasContainerCgroups {
		require.NotNil(tb, containerRootCgroup, "container root cgroup must be set with cgroups that are visible in container namespace")
	}

	// Create all the structure
	for _, cgroup := range cgroups {
		cgroup.createFiles(tb, &fs)
		addCgroupPidFiles(tb, fs.HostProc, &cgroup, hostRootCgroup)

		if cgroup.VisibleInContainerNamespace {
			addCgroupPidFiles(tb, fs.ContainerProc, &cgroup, containerRootCgroup)
		}
	}

	return &fs
}

// createBaseCgroupfs creates the base cgroupfs directory at the given root
func createBaseCgroupfs(tb testing.TB, root string) string {
	cgroupfs := filepath.Join(root, "/sys/fs/cgroup")
	require.NoError(tb, os.MkdirAll(cgroupfs, 0755))

	return cgroupfs
}

// addCgroupPidFiles adds the cgroup file in the given procfs for the given cgroup.
// The root cgroup is used to calculate the relative path of the cgroup to the root.
func addCgroupPidFiles(tb testing.TB, procfs string, cgroup *FakeCgroup, rootCgroup *FakeCgroup) {
	rootFullPath := rootCgroup.FullName()
	cgroupFullPath := cgroup.FullName()
	cgroupRelativeToRoot, err := filepath.Rel(rootFullPath, cgroupFullPath)
	require.NoError(tb, err)

	for _, pid := range cgroup.PIDs {
		targetFiles := []string{
			filepath.Join(procfs, strconv.Itoa(pid), "task", strconv.Itoa(pid), "cgroup"),
			filepath.Join(procfs, strconv.Itoa(pid), "cgroup"),
		}
		contents := "0::/" + cgroupRelativeToRoot

		for _, targetFile := range targetFiles {
			tb.Logf("cgroup %s: %s written to %s", cgroup.Name, contents, targetFile)
			require.NoError(tb, os.MkdirAll(filepath.Dir(targetFile), 0755), "cannot create directory at %s", filepath.Dir(targetFile))
			require.NoError(tb, os.WriteFile(targetFile, []byte(contents), 0644), "cannot write cgroup.procs file at %s", targetFile)
		}
	}
}

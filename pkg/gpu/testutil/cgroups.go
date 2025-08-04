package testutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

type FakeCgroup struct {
	Name                        string
	Parent                      *FakeCgroup
	PIDs                        []int
	VisibleInContainerNamespace bool
	IsContainerRoot             bool
	IsHostRoot                  bool
}

func (c *FakeCgroup) FullName() string {
	if c.Parent == nil {
		return "/" + c.Name
	}

	return filepath.Join(c.Parent.FullName(), c.Name)
}

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

		// Sanity check that the inodes of containerCgroupFs and childCgroupFullPath are the same
		var containerCgroupStat, hostCgroupPathStat unix.Stat_t
		require.NoError(tb, unix.Stat(containerCgroupPath, &containerCgroupStat))
		require.NoError(tb, unix.Stat(hostCgroupPath, &hostCgroupPathStat))
		require.Equal(tb, containerCgroupStat.Ino, hostCgroupPathStat.Ino, "the inodes should be the same, something is wrong with the bind mount")

		tb.Logf("cgroup %s: bindmount %s -> %s", c.Name, hostCgroupPath, containerCgroupPath)
	}
}

type FakeCgroupFilesystem struct {
	Root                  string
	HostRoot              string
	HostRootMountpoint    string
	ContainerCgroupFsPath string
	HostCgroupFsPath      string
	HostProc              string
	ContainerProc         string
}

func (fs *FakeCgroupFilesystem) SetupTestEnvvars(tb testing.TB) {
	// Avoid memoization of ProcFSRoot, as we're not using the real procfs for utils.GetProcControlGroups
	kernel.ResetProcFSRoot()
	tb.Setenv("HOST_PROC", fs.HostProc)
	tb.Cleanup(func() {
		kernel.ResetProcFSRoot()
	})
}

func createBaseCgroupfs(tb testing.TB, root string) string {
	cgroupfs := filepath.Join(root, "/sys/fs/cgroup")
	require.NoError(tb, os.MkdirAll(cgroupfs, 0755))

	return cgroupfs
}

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
		contents := fmt.Sprintf("0::/%s", cgroupRelativeToRoot)

		for _, targetFile := range targetFiles {
			tb.Logf("cgroup %s: %s written to %s", cgroup.Name, contents, targetFile)
			require.NoError(tb, os.MkdirAll(filepath.Dir(targetFile), 0755), "cannot create directory at %s", filepath.Dir(targetFile))
			require.NoError(tb, os.WriteFile(targetFile, []byte(contents), 0644), "cannot write cgroup.procs file at %s", targetFile)
		}
	}
}

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

	for _, cgroup := range cgroups {
		cgroup.createFiles(tb, &fs)
		addCgroupPidFiles(tb, fs.HostProc, &cgroup, hostRootCgroup)

		if cgroup.VisibleInContainerNamespace {
			addCgroupPidFiles(tb, fs.ContainerProc, &cgroup, containerRootCgroup)
		}
	}

	return &fs
}

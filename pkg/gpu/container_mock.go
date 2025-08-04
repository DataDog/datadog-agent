//go:build linux

package gpu

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// mockContainerEnv represents a mock container environment for testing
type mockContainerEnv struct {
	containerRoot      string
	hostRootMountpoint string
	pid                int
	siblingProc        int
	childCgroupPath    string
	siblingCgroupPath  string
	cleanup            func()
}

// setupMockContainerEnv creates a mock container environment with fake cgroup hierarchy
// for testing cgroup-related functionality inside containers
func setupMockContainerEnv(t *testing.T) *mockContainerEnv {
	if os.Geteuid() != 0 {
		t.Skip("Test requires root privileges for the bind mount")
	}

	// For this test, instead of setting up the container completely as we do in the other tests,
	// we will just mock the cgroup hierarchy
	containerRoot := t.TempDir()

	// Create the procfs structure
	hostRootMountpoint := "/host"
	hostProc := filepath.Join(containerRoot, hostRootMountpoint, "proc")
	pid := 10
	siblingProc := 20

	// Avoid memoization of ProcFSRoot, as we're not using the real procfs for utils.GetProcControlGroups
	kernel.ResetProcFSRoot()
	t.Setenv("HOST_PROC", hostProc)
	cleanupKernel := func() {
		kernel.ResetProcFSRoot()
	}

	mainProcCgroupFile := filepath.Join(hostProc, strconv.Itoa(pid), "task", strconv.Itoa(pid), "cgroup")
	require.NoError(t, os.MkdirAll(filepath.Dir(mainProcCgroupFile), 0755))
	require.NoError(t, os.WriteFile(mainProcCgroupFile, []byte("0::/"), 0644))

	siblingCgroupName := fmt.Sprintf("test-sibling-cgroup-%s", utils.RandString(10))
	siblingProcCgroupFile := filepath.Join(hostProc, strconv.Itoa(siblingProc), "task", strconv.Itoa(siblingProc), "cgroup")
	require.NoError(t, os.MkdirAll(filepath.Dir(siblingProcCgroupFile), 0755))
	require.NoError(t, os.WriteFile(siblingProcCgroupFile, []byte(fmt.Sprintf("0::/../%s", siblingCgroupName)), 0644))

	// The container Cgroupfs is just a single directory (no child cgroups)
	containerCgroupFs := filepath.Join(containerRoot, "/sys/fs/cgroup")
	require.NoError(t, os.MkdirAll(containerCgroupFs, 0755))

	// The host Cgroupfs is a single directory with some cgroups
	hostCgroupFs := filepath.Join(containerRoot, hostRootMountpoint, "/sys/fs/cgroup")

	for i := 0; i < 10; i++ {
		cgroupName := fmt.Sprintf("test-cgroup-%d", i)
		cgroupPath := filepath.Join(hostCgroupFs, cgroupName, "cgroup.procs")
		require.NoError(t, os.MkdirAll(filepath.Dir(cgroupPath), 0755))
		require.NoError(t, os.WriteFile(cgroupPath, []byte(strconv.Itoa(pid)), 0644))
	}

	// Our target cgroup is a child cgroup too, so create a nested hierarchy
	parentCgroupName := fmt.Sprintf("test-parent-cgroup-%s", utils.RandString(10))

	// Create our child cgroup, using a bind mount so the inode is the same as the cgroup directory for the parent
	childCgroupName := fmt.Sprintf("test-child-cgroup-%s", utils.RandString(10))
	childCgroupPath := filepath.Join(parentCgroupName, childCgroupName)
	childCgroupFullPath := filepath.Join(hostCgroupFs, childCgroupPath)
	require.NoError(t, os.MkdirAll(childCgroupFullPath, 0755))
	err := unix.Mount(containerCgroupFs, childCgroupFullPath, "bind", unix.MS_BIND, "")
	// If we get permission denied when trying a bind mount inside our temporary directory,
	// it probably means we're running in a container and the bind mount is not allowed.
	if errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) {
		t.Skip("Test requires privileges to bind mount our test directories")
	}
	require.NoError(t, err)

	cleanupMount := func() {
		require.NoError(t, unix.Unmount(childCgroupFullPath, unix.MNT_DETACH))
	}

	// For sanity check, ensure here that the inodes of containerCgroupFs and childCgroupFullPath are the same
	var containerCgroupFsStat, childCgroupFullPathStat unix.Stat_t
	require.NoError(t, unix.Stat(containerCgroupFs, &containerCgroupFsStat))
	require.NoError(t, unix.Stat(childCgroupFullPath, &childCgroupFullPathStat))
	require.Equal(t, containerCgroupFsStat.Ino, childCgroupFullPathStat.Ino, "the inodes should be the same, something is wrong with the bind mount")

	// For the sibling cgroup we don't need the directory structure, we just need the cgroup name
	siblingCgroupPath := filepath.Join(parentCgroupName, siblingCgroupName)

	env := &mockContainerEnv{
		containerRoot:      containerRoot,
		hostRootMountpoint: hostRootMountpoint,
		pid:                pid,
		siblingProc:        siblingProc,
		childCgroupPath:    childCgroupPath,
		siblingCgroupPath:  siblingCgroupPath,
		cleanup: func() {
			cleanupMount()
			cleanupKernel()
		},
	}

	t.Cleanup(env.cleanup)
	return env
}

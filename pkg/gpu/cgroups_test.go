// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package gpu

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/require"

	containerdcgroups "github.com/containerd/cgroups/v3"
	"github.com/containerd/cgroups/v3/cgroup1"
	"github.com/containerd/cgroups/v3/cgroup2"

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/security/utils"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/link"
)

func isCgroupfsReadonly() bool {
	sysfsPath := filepath.Join("/sys/fs/cgroup", "test")
	err := os.MkdirAll(sysfsPath, 0755)
	if err != nil {
		return true
	}
	defer os.RemoveAll(sysfsPath)

	return false
}

func TestInsertDeviceAllowLine(t *testing.T) {
	tests := []struct {
		name          string
		lines         []string
		sectionHeader string
		newLine       string
		expected      []string
		expectError   bool
	}{
		{
			name: "insert after [Service] section with no existing DeviceAllow",
			lines: []string{
				"[Unit]",
				"Description=Test Service",
				"",
				"[Service]",
				"ExecStart=/bin/true",
				"",
				"[Install]",
				"WantedBy=multi-user.target",
			},
			sectionHeader: "[Service]",
			newLine:       "DeviceAllow=char-nvidia rwm",
			expected: []string{
				"[Unit]",
				"Description=Test Service",
				"",
				"[Service]",
				"DeviceAllow=char-nvidia rwm",
				"ExecStart=/bin/true",
				"",
				"[Install]",
				"WantedBy=multi-user.target",
			},
			expectError: false,
		},
		{
			name: "insert after existing DeviceAllow lines",
			lines: []string{
				"[Unit]",
				"Description=Test Service",
				"",
				"[Service]",
				"DeviceAllow=char-input rwm",
				"DeviceAllow=char-tty rwm",
				"ExecStart=/bin/true",
				"",
				"[Install]",
				"WantedBy=multi-user.target",
			},
			sectionHeader: "[Service]",
			newLine:       "DeviceAllow=char-nvidia rwm",
			expected: []string{
				"[Unit]",
				"Description=Test Service",
				"",
				"[Service]",
				"DeviceAllow=char-input rwm",
				"DeviceAllow=char-tty rwm",
				"DeviceAllow=char-nvidia rwm",
				"ExecStart=/bin/true",
				"",
				"[Install]",
				"WantedBy=multi-user.target",
			},
			expectError: false,
		},
		{
			name: "insert in section with no subsequent sections",
			lines: []string{
				"[Unit]",
				"Description=Test Service",
				"",
				"[Service]",
				"ExecStart=/bin/true",
				"Restart=always",
			},
			sectionHeader: "[Service]",
			newLine:       "DeviceAllow=char-nvidia rwm",
			expected: []string{
				"[Unit]",
				"Description=Test Service",
				"",
				"[Service]",
				"DeviceAllow=char-nvidia rwm",
				"ExecStart=/bin/true",
				"Restart=always",
			},
			expectError: false,
		},
		{
			name: "insert in section with empty lines in the middle",
			lines: []string{
				"[Unit]",
				"Description=Test Service",
				"",
				"[Service]",
				"DeviceAllow=char-input rwm",
				"",
				"DeviceAllow=char-tty rwm",
				"",
				"ExecStart=/bin/true",
				"",
				"[Install]",
				"WantedBy=multi-user.target",
			},
			sectionHeader: "[Service]",
			newLine:       "DeviceAllow=char-nvidia rwm",
			expected: []string{
				"[Unit]",
				"Description=Test Service",
				"",
				"[Service]",
				"DeviceAllow=char-input rwm",
				"",
				"DeviceAllow=char-tty rwm",
				"DeviceAllow=char-nvidia rwm",
				"",
				"ExecStart=/bin/true",
				"",
				"[Install]",
				"WantedBy=multi-user.target",
			},
			expectError: false,
		},
		{
			name: "insert in section with mixed content and DeviceAllow lines",
			lines: []string{
				"[Unit]",
				"Description=Test Service",
				"",
				"[Service]",
				"Type=simple",
				"DeviceAllow=char-input rwm",
				"ExecStart=/bin/true",
				"DeviceAllow=char-tty rwm",
				"Restart=always",
				"",
				"[Install]",
				"WantedBy=multi-user.target",
			},
			sectionHeader: "[Service]",
			newLine:       "DeviceAllow=char-nvidia rwm",
			expected: []string{
				"[Unit]",
				"Description=Test Service",
				"",
				"[Service]",
				"Type=simple",
				"DeviceAllow=char-input rwm",
				"ExecStart=/bin/true",
				"DeviceAllow=char-tty rwm",
				"DeviceAllow=char-nvidia rwm",
				"Restart=always",
				"",
				"[Install]",
				"WantedBy=multi-user.target",
			},
			expectError: false,
		},
		{
			name: "section not found",
			lines: []string{
				"[Unit]",
				"Description=Test Service",
				"",
				"[Service]",
				"ExecStart=/bin/true",
			},
			sectionHeader: "[Install]",
			newLine:       "WantedBy=multi-user.target",
			expected:      nil,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := insertDeviceAllowLine(tt.lines, tt.sectionHeader, tt.newLine)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildSafePath(t *testing.T) {
	tests := []struct {
		name      string
		rootfs    string
		basedir   string
		parts     []string
		expected  string
		expectErr bool
	}{
		{
			name:     "basic path construction",
			rootfs:   "/var/lib/docker",
			basedir:  "containers",
			parts:    []string{"abc123", "config.json"},
			expected: "/var/lib/docker/containers/abc123/config.json",
		},
		{
			name:      "path traversal attempt",
			rootfs:    "/var/lib/docker",
			basedir:   "containers",
			parts:     []string{"..", "etc", "passwd"},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildSafePath(tt.rootfs, tt.basedir, tt.parts...)

			if tt.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestDetachAllDeviceCgroupPrograms(t *testing.T) {
	// This test requires root privileges and a cgroupv2 system
	// Skip if not running as root or if cgroupv2 is not available
	if os.Geteuid() != 0 {
		t.Skip("Test requires root privileges")
	}

	if containerdcgroups.Mode() != containerdcgroups.Unified {
		t.Skip("Test requires cgroupv2")
	}

	if isCgroupfsReadonly() {
		t.Skip("Test requires a writable cgroupfs")
	}

	// We will be testing by reading /dev/null, so we need to make sure it's accessible
	// before we start the test
	devnull, err := os.Open("/dev/null")
	if err != nil {
		t.Skip("Test requires /dev/null to be accessible")
	} else {
		devnull.Close()
	}

	testCgroupName := "test-detach-device-programs-" + utils.RandString(10)
	testCgroupPath := filepath.Join("/sys/fs/cgroup", testCgroupName)
	moveSelfToCgroup(t, testCgroupName)

	// Test that /dev/null is still accessible after moving to cgroup (no BPF programs yet)
	f, err := os.Open("/dev/null")
	require.NoError(t, err)
	f.Close()

	// Create a BPF program that denies access to device with major number 1 (includes /dev/null)
	prog, err := ebpf.NewProgram(&ebpf.ProgramSpec{
		Type: ebpf.CGroupDevice,
		Instructions: asm.Instructions{
			// R1 contains pointer to the structure
			// Load major number (second uint32 at offset 4)
			asm.LoadMem(asm.R2, asm.R1, 4, asm.Word),
			// Check if this is device major 1 (which includes /dev/null)
			asm.LoadImm(asm.R3, 1, asm.DWord),
			asm.JEq.Reg(asm.R2, asm.R3, "deny"),

			// Allow access to other devices
			asm.LoadImm(asm.R0, 1, asm.DWord),
			asm.Return(),

			// Deny access to device major 1
			asm.LoadImm(asm.R0, 0, asm.DWord).WithSymbol("deny"),
			asm.Return(),
		},
		License: "GPL",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		prog.Close()
	})

	testCgroupDescr, err := os.Open(testCgroupPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		testCgroupDescr.Close()
	})

	// Important to use RawAttachProgram here, if we use the higher level AttachProgram
	// primitive it seems we cannot correctly detach the program later as we're still
	// holding references to it.
	err = link.RawAttachProgram(link.RawAttachProgramOptions{
		Target:  int(testCgroupDescr.Fd()),
		Attach:  ebpf.AttachCGroupDevice,
		Program: prog,
	})
	require.NoError(t, err)

	// /dev/null should now be inaccessible
	_, err = os.Open("/dev/null")
	require.Error(t, err, "expected /dev/null open to fail after attaching BPF program, but it succeeded")

	// Now detach all device programs
	err = detachAllDeviceCgroupPrograms("", testCgroupName)
	require.NoError(t, err)

	// /dev/null should be accessible again
	f, err = os.Open("/dev/null")
	require.NoError(t, err)
	f.Close()
}

func TestConfigureCgroupV1DeviceAllow(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("Test requires root privileges")
	}

	if containerdcgroups.Mode() != containerdcgroups.Legacy {
		t.Skip("Test requires cgroupv1")
	}

	if isCgroupfsReadonly() {
		t.Skip("Test requires a writable cgroupfs")
	}

	// We will be testing by reading /dev/null, so we need to make sure it's accessible
	// before we start the test
	devnull, err := os.Open("/dev/null")
	if err != nil {
		t.Skip("Test requires /dev/null to be accessible")
	} else {
		devnull.Close()
	}

	testCgroupName := "test-cgroup-device-allow-" + utils.RandString(10)
	moveSelfToCgroup(t, testCgroupName)

	// Test that /dev/null is still accessible after moving to cgroup
	f, err := os.Open("/dev/null")
	require.NoError(t, err)
	f.Close()

	// Update the device allow file to deny ourselves access to /dev/null
	devDenyFilePath := filepath.Join("/", cgroupv1DeviceControlDir, testCgroupName, "devices.deny")
	devDenyFile, err := os.OpenFile(devDenyFilePath, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	t.Cleanup(func() {
		devDenyFile.Close()
	})

	_, err = devDenyFile.WriteString("c 1:* rwm\n")
	require.NoError(t, err)

	// Test that /dev/null is now inaccessible
	_, err = os.Open("/dev/null")
	require.Error(t, err, "expected /dev/null open to fail after updating device allow file, but it succeeded")

	// Now configure the cgroup device allow
	require.NoError(t, configureCgroupV1DeviceAllow("", testCgroupName, 1))

	// Test that /dev/null is now accessible
	f, err = os.Open("/dev/null")
	require.NoError(t, err)
	f.Close()

}

func TestGetAbsoluteCgroupForProcess(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("Test requires root privileges")
	}

	if isCgroupfsReadonly() {
		t.Skip("Test requires a writable cgroupfs")
	}

	currentCgroup, err := getAbsoluteCgroupForProcess("", "/", uint32(os.Getpid()), uint32(os.Getpid()), containerdcgroups.Mode())
	require.NoError(t, err)
	require.NotEmpty(t, currentCgroup) // Cgroup could be anything, but it should not be empty

	testCgroupName := "test-get-cgroup-for-process-" + utils.RandString(10)
	moveSelfToCgroup(t, testCgroupName)

	currentCgroup, err = getAbsoluteCgroupForProcess("", "/", uint32(os.Getpid()), uint32(os.Getpid()), containerdcgroups.Mode())
	require.NoError(t, err)
	require.Equal(t, "/"+testCgroupName, currentCgroup)
}

func TestGetAbsoluteCgroupV1ForProcess(t *testing.T) {
	mainPid := 10
	siblingPid := 20

	// cgroupsv1 does not have relative cgroup paths, so we don't need to test inside of a container
	rootCgroup := testutil.FakeCgroup{
		Name:       "",
		IsHostRoot: true,
		PIDs:       []int{},
	}

	mainCgroup := testutil.FakeCgroup{
		Name:   "test-parent-cgroup-" + utils.RandString(10),
		PIDs:   []int{mainPid},
		Parent: &rootCgroup,
	}

	siblingCgroup := testutil.FakeCgroup{
		Name:   "test-sibling-cgroup-" + utils.RandString(10),
		PIDs:   []int{siblingPid},
		Parent: &rootCgroup,
	}

	cgroups := []testutil.FakeCgroup{
		rootCgroup,
		mainCgroup,
		siblingCgroup,
	}

	fs := testutil.CreateFakeCgroupFilesystem(t, cgroups)
	fs.SetupTestEnvvars(t)

	t.Run("SameProcess", func(t *testing.T) {
		cgroupPath, err := getAbsoluteCgroupForProcess(fs.Root, fs.HostRootMountpoint, uint32(mainPid), uint32(mainPid), containerdcgroups.Legacy)
		require.NoError(t, err)
		require.Equal(t, mainCgroup.FullName(), cgroupPath)
	})

	t.Run("SiblingProcess", func(t *testing.T) {
		cgroupPath, err := getAbsoluteCgroupForProcess(fs.Root, fs.HostRootMountpoint, uint32(mainPid), uint32(siblingPid), containerdcgroups.Legacy)
		require.NoError(t, err)
		require.Equal(t, siblingCgroup.FullName(), cgroupPath)
	})
}

func TestGetAbsoluteCgroupV2ForProcessInsideContainer(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("Test requires root privileges for the bind mount")
	}

	// For this test, instead of setting up the container completely as we do in the other tests,
	// we will just mock the cgroup hierarchy
	pid := 10
	siblingProc := 20

	rootCgroup := testutil.FakeCgroup{
		Name:       "",
		IsHostRoot: true,
		PIDs:       []int{},
	}

	parentCgroup := testutil.FakeCgroup{
		Name: "test-parent-cgroup-" + utils.RandString(10),
		PIDs: []int{},
	}

	childCgroup := testutil.FakeCgroup{
		Name:                        "test-child-cgroup-" + utils.RandString(10),
		Parent:                      &parentCgroup,
		PIDs:                        []int{pid},
		IsContainerRoot:             true,
		VisibleInContainerNamespace: true,
	}

	siblingCgroup := testutil.FakeCgroup{
		Name:                        "test-sibling-cgroup-" + utils.RandString(10),
		Parent:                      &parentCgroup,
		PIDs:                        []int{siblingProc},
		VisibleInContainerNamespace: true,
	}

	cgroups := []testutil.FakeCgroup{
		rootCgroup,
		parentCgroup,
		childCgroup,
		siblingCgroup,
	}

	// Add some sample cgroups to the hierarchy
	for i := 0; i < 10; i++ {
		cgroups = append(cgroups, testutil.FakeCgroup{
			Name: fmt.Sprintf("test-cgroup-%d", i),
			PIDs: []int{1000 + i},
		})
	}

	fs := testutil.CreateFakeCgroupFilesystem(t, cgroups)
	fs.SetupTestEnvvars(t)

	t.Run("SameProcess", func(t *testing.T) {
		cgroupPath, err := getAbsoluteCgroupForProcess(fs.Root, fs.HostRootMountpoint, uint32(pid), uint32(pid), containerdcgroups.Unified)
		require.NoError(t, err)
		require.Equal(t, childCgroup.FullName(), cgroupPath)
	})

	t.Run("SiblingProcess", func(t *testing.T) {
		cgroupPath, err := getAbsoluteCgroupForProcess(fs.Root, fs.HostRootMountpoint, uint32(pid), uint32(siblingProc), containerdcgroups.Unified)
		require.NoError(t, err)
		require.Equal(t, siblingCgroup.FullName(), cgroupPath)
	})
}

func moveSelfToCgroup(t *testing.T, cgroupName string) {
	if containerdcgroups.Mode() == containerdcgroups.Unified {
		prevCgroupPath, err := cgroup2.PidGroupPath(os.Getpid())
		require.NoError(t, err)

		prevCgroup, err := cgroup2.Load(prevCgroupPath)
		require.NoError(t, err)

		cgroup, err := cgroup2.NewManager("/sys/fs/cgroup", "/"+cgroupName, &cgroup2.Resources{})
		require.NoError(t, err, "failed to create cgroup")
		t.Cleanup(func() {
			cgroup.Delete()
		})

		require.NoError(t, cgroup.AddProc(uint64(os.Getpid())))
		t.Cleanup(func() {
			if err := prevCgroup.AddProc(uint64(os.Getpid())); err != nil {
				t.Logf("Failed to add process to root cgroup: %v", err)
			}
		})
	} else {
		prevCgroupPath := cgroup1.PidPath(os.Getpid())
		prevCgroup, err := cgroup1.Load(prevCgroupPath)
		if errors.Is(err, cgroup1.ErrCgroupDeleted) {
			// Jobs like tests_deb_*, tests_rpm_* run inside of containers, and
			// this step fails as the cgroup is not accesible. In that case, and considering we have KMT tests
			// for coverage, skip the test.
			t.Skip("cannot run cgroup tests in containerized test environment")
		}

		require.NoError(t, err)

		cgroup, err := cgroup1.New(cgroup1.StaticPath("/"+cgroupName), &specs.LinuxResources{})
		require.NoError(t, err, "failed to create cgroup")
		t.Cleanup(func() {
			cgroup.Delete()
		})

		proc := cgroup1.Process{Pid: os.Getpid()}
		require.NoError(t, cgroup.Add(proc))
		t.Cleanup(func() {
			if err := prevCgroup.Add(proc); err != nil {
				t.Logf("Failed to add process to root cgroup: %v", err)
			}
		})
	}
}

// createDeepCgroupStructure creates a directory structure with a lot of subdirectories
// and returns the number of directories created.
func createDeepCgroupStructure(depth int, numDirs int) []testutil.FakeCgroup {
	cgroups := []testutil.FakeCgroup{}

	for i := 0; i < numDirs; i++ {
		cgroups = append(cgroups, testutil.FakeCgroup{
			Name: fmt.Sprintf("test-cgroup-%d-%d", depth, i),
			PIDs: []int{},
		})

		if depth > 0 {
			cgroups = append(cgroups, createDeepCgroupStructure(depth-1, numDirs)...)
		}
	}

	return cgroups
}

func BenchmarkGetAbsoluteCgroupForProcess(b *testing.B) {
	// Create a cgroup hierarchy with a lot of subdirectories
	cgroups := createDeepCgroupStructure(4, 10)
	fs := testutil.CreateFakeCgroupFilesystem(b, cgroups)
	b.Logf("Created %d directories", len(cgroups))

	// Doesn't matter that the cgroup here is not found, we in fact
	// want the code to iterate though all the directories in the
	// cgroup directory.
	for b.Loop() {
		getAbsoluteCgroupForProcess(fs.Root, fs.HostRootMountpoint, uint32(os.Getpid()), uint32(os.Getpid()), containerdcgroups.Mode())
	}

	b.ReportMetric(float64(len(cgroups)), "dirs/op")
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && !bpf

// Tests in this file don't need eBPF or root: the !bpf constraint keeps them out of the kernel matrix tests
// (KMT), and the regular unit tests run them. Tests that do are in cgroups_privileged_test.go.

package gpu

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	containerdcgroups "github.com/containerd/cgroups/v3"

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

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

func TestMissingDeviceAllowLines(t *testing.T) {
	scope := []string{"[Unit]", "Description=x", "", "[Scope]", "Delegate=yes", "DeviceAllow=char-pts rwm", "", "[Install]", "DeviceAllow=char-nvidia rwm"}
	// None present in [Scope] (the one in [Install] does not count)
	require.Equal(t, nvidiaSystemdDeviceAllow, missingDeviceAllowLines(scope, "[Scope]", nvidiaSystemdDeviceAllow))

	withOne := append([]string{}, scope[:5]...)
	withOne = append(withOne, "DeviceAllow=char-nvidia rwm")
	withOne = append(withOne, scope[5:]...)
	require.Equal(t, "DeviceAllow=char-195 rwm\n", missingDeviceAllowLines(withOne, "[Scope]", nvidiaSystemdDeviceAllow))

	withBoth := append([]string{}, scope[:5]...)
	withBoth = append(withBoth, "  DeviceAllow=char-195 rwm", "DeviceAllow=char-nvidia rwm")
	withBoth = append(withBoth, scope[5:]...)
	require.Empty(t, missingDeviceAllowLines(withBoth, "[Scope]", nvidiaSystemdDeviceAllow))
}

func TestConfigureSystemDAllowIsIdempotent(t *testing.T) {
	root := t.TempDir()
	containerID := "cri-containerd-0123.scope"
	dir := filepath.Join(root, systemdTransientConfigPath, containerID+".d")
	require.NoError(t, os.MkdirAll(dir, 0755))
	path := filepath.Join(dir, systemdDeviceAllowFile)
	require.NoError(t, os.WriteFile(path, []byte("[Scope]\nDeviceAllow=char-pts rwm\n"), 0644))

	require.NoError(t, configureSystemDAllow(root, containerID))
	first, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "[Scope]\nDeviceAllow=char-pts rwm\nDeviceAllow=char-nvidia rwm\nDeviceAllow=char-195 rwm\n\n", string(first))

	// Re-applying the permissions, as done every cgroup_reapply_interval, does not grow the file
	for i := 0; i < 3; i++ {
		require.NoError(t, configureSystemDAllow(root, containerID))
	}
	again, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(first), string(again))
}

func TestMissingDeviceAllowLinesResetAndSpacing(t *testing.T) {
	// systemd writes the drop-in with a reset first
	systemdFormat := []string{"# This is a drop-in unit file extension, created via \"systemctl set-property\"", "# or an equivalent operation. Do not edit.", "[Scope]", "DeviceAllow=", "DeviceAllow=char-pts rwm"}
	require.Equal(t, nvidiaSystemdDeviceAllow, missingDeviceAllowLines(systemdFormat, "[Scope]", nvidiaSystemdDeviceAllow))

	// Rules before a reset are not in effect
	reset := []string{"[Scope]", "DeviceAllow=char-nvidia rwm", "DeviceAllow=char-195 rwm", "DeviceAllow =", "DeviceAllow=char-pts rwm"}
	require.Equal(t, nvidiaSystemdDeviceAllow, missingDeviceAllowLines(reset, "[Scope]", nvidiaSystemdDeviceAllow))

	// Spacing around "=" and between values does not matter
	spaced := []string{"[Scope]", "DeviceAllow = char-nvidia   rwm", "DeviceAllow=char-195 rwm"}
	require.Empty(t, missingDeviceAllowLines(spaced, "[Scope]", nvidiaSystemdDeviceAllow))
}

func TestConfigureSystemDAllowRestoresPartialAndResetRules(t *testing.T) {
	root := t.TempDir()
	containerID := "cri-containerd-0456.scope"
	dir := filepath.Join(root, systemdTransientConfigPath, containerID+".d")
	require.NoError(t, os.MkdirAll(dir, 0755))
	path := filepath.Join(dir, systemdDeviceAllowFile)

	// One of the two rules, then a systemd rewrite that reset them
	require.NoError(t, os.WriteFile(path, []byte("[Scope]\nDeviceAllow=char-nvidia rwm\nDeviceAllow=\nDeviceAllow=char-pts rwm\n"), 0644))
	require.NoError(t, configureSystemDAllow(root, containerID))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(string(content), "\n")
	require.Empty(t, missingDeviceAllowLines(lines, "[Scope]", nvidiaSystemdDeviceAllow), string(content))

	// Applying again does not change the file
	require.NoError(t, configureSystemDAllow(root, containerID))
	again, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(content), string(again))
}

func TestConfigureSystemDAllowAfterSpacedReset(t *testing.T) {
	root := t.TempDir()
	containerID := "cri-containerd-0789.scope"
	dir := filepath.Join(root, systemdTransientConfigPath, containerID+".d")
	require.NoError(t, os.MkdirAll(dir, 0755))
	path := filepath.Join(dir, systemdDeviceAllowFile)
	require.NoError(t, os.WriteFile(path, []byte("[Scope]\nDeviceAllow=char-pts rwm\nDeviceAllow =\nDeviceAllow = char-x rwm\n"), 0644))

	require.NoError(t, configureSystemDAllow(root, containerID))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	// Inserted after the last rule, so after the reset
	require.Equal(t, "[Scope]\nDeviceAllow=char-pts rwm\nDeviceAllow =\nDeviceAllow = char-x rwm\nDeviceAllow=char-nvidia rwm\nDeviceAllow=char-195 rwm\n\n", string(content))

	require.NoError(t, configureSystemDAllow(root, containerID))
	again, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(content), string(again))
}

func TestConfigureSystemDAllowWithSeveralScopeSections(t *testing.T) {
	root := t.TempDir()
	containerID := "cri-containerd-0abc.scope"
	dir := filepath.Join(root, systemdTransientConfigPath, containerID+".d")
	require.NoError(t, os.MkdirAll(dir, 0755))
	path := filepath.Join(dir, systemdDeviceAllowFile)
	require.NoError(t, os.WriteFile(path, []byte("[Scope]\nDeviceAllow=char-pts rwm\n[Unit]\nDescription=x\n[Scope]\nDeviceAllow=\nDeviceAllow=char-x rwm\n"), 0644))

	require.NoError(t, configureSystemDAllow(root, containerID))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	// Added to the last [Scope] section, after its reset
	require.Equal(t, "[Scope]\nDeviceAllow=char-pts rwm\n[Unit]\nDescription=x\n[Scope]\nDeviceAllow=\nDeviceAllow=char-x rwm\nDeviceAllow=char-nvidia rwm\nDeviceAllow=char-195 rwm\n\n", string(content))
	require.Empty(t, missingDeviceAllowLines(strings.Split(string(content), "\n"), "[Scope]", nvidiaSystemdDeviceAllow))

	require.NoError(t, configureSystemDAllow(root, containerID))
	again, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(content), string(again))
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

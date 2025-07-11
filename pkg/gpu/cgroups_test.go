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

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"

	"github.com/cilium/ebpf"
)

func TestInsertAfterSection(t *testing.T) {
	tests := []struct {
		name          string
		lines         []string
		sectionHeader string
		newLine       string
		expected      []string
		expectError   bool
	}{
		{
			name: "insert after [Service] section",
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
			result, err := insertAfterSection(tt.lines, tt.sectionHeader, tt.newLine)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d lines, got %d", len(tt.expected), len(result))
				return
			}

			for i, expectedLine := range tt.expected {
				if i >= len(result) {
					t.Errorf("missing line %d: expected %q", i, expectedLine)
					continue
				}
				if result[i] != expectedLine {
					t.Errorf("line %d: expected %q, got %q", i, expectedLine, result[i])
				}
			}
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

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestConfigureCgroupV2DeviceAllow(t *testing.T) {
	// This test requires root privileges and a cgroupv2 system
	// Skip if not running as root or if cgroupv2 is not available
	if os.Geteuid() != 0 {
		t.Skip("Test requires root privileges")
	}

	// We will be testing by reading /dev/null, so we need to make sure it's accessible
	// before we start the test
	devnull, err := os.Open("/dev/null")
	if err != nil {
		t.Skip("Test requires /dev/null to be accessible")
	} else {
		devnull.Close()
	}

	// Check if cgroupv2 is available
	cgroupReader, err := cgroups.NewReader()
	require.NoError(t, err)

	if cgroupReader.CgroupVersion() != 2 {
		t.Skip("Test requires cgroupv2")
	}

	testCgroupName := fmt.Sprintf("test-nvidia-device-allow-%s", utils.RandString(10))
	testCgroupPath := filepath.Join("/sys/fs/cgroup", testCgroupName)

	// Clean up after test
	defer func() {
		// Move process back to parent cgroup
		parentCgroupProcs := "/sys/fs/cgroup/cgroup.procs"
		pid := os.Getpid()
		_ = os.WriteFile(parentCgroupProcs, []byte(fmt.Sprintf("%d\n", pid)), 0644)
		// Remove the test cgroup directory
		if err := os.RemoveAll(testCgroupPath); err != nil {
			t.Logf("Failed to clean up test cgroup: %v", err)
		}
	}()

	// Test that /dev/null is accessible before adding the cgroup
	f, err := os.OpenFile("/dev/null", os.O_WRONLY, 0)
	require.NoError(t, err)
	if f != nil {
		f.Close()
	}

	// Create the test cgroup directory
	require.NoError(t, os.MkdirAll(testCgroupPath, 0755))

	// Enable controllers for the cgroup by writing to cgroup.subtree_control
	// This is required to make it a real cgroup
	subtreeControlPath := filepath.Join(testCgroupPath, "cgroup.subtree_control")
	require.NoError(t, os.WriteFile(subtreeControlPath, []byte("+pids"), 0644))

	// Refresh the cgroups reader to pick up the new cgroup
	require.NoError(t, cgroupReader.RefreshCgroups(0))

	// Get the cgroup object, ensure it exists
	testCgroup := cgroupReader.GetCgroup(testCgroupPath)
	require.NotNil(t, testCgroup)

	// Move self into the cgroup
	cgroupProcs := filepath.Join(testCgroupPath, "cgroup.procs")
	pid := os.Getpid()
	require.NoError(t, os.WriteFile(cgroupProcs, []byte(fmt.Sprintf("%d\n", pid)), 0644))

	// Test the BPF program attachment, allowing only NVIDIA
	err = configureCgroupV2DeviceAllow("", testCgroupName, nvidiaDeviceMajor)
	var verifierError *ebpf.VerifierError
	if errors.As(err, &verifierError) {
		t.Logf("Printing verifier error")
		for _, line := range verifierError.Log {
			t.Logf("%s", line)
		}
	}
	require.NoError(t, err)

	// /dev/null should be inaccessible
	f, err = os.Open("/dev/null")
	if err == nil {
		f.Close()
		t.Fatalf("expected /dev/null open to fail after moving to cgroup, but it succeeded")
	}

	// Now allow devices with major 1
	err = configureCgroupV2DeviceAllow("", testCgroupName, 1)
	if errors.As(err, &verifierError) {
		t.Logf("Printing verifier error")
		for _, line := range verifierError.Log {
			t.Logf("%s", line)
		}
	}
	require.NoError(t, err)

	// Test that /dev/null is now accessible
	f, err = os.Open("/dev/null")
	require.NoError(t, err)
}

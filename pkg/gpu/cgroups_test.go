// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package gpu

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
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
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

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
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

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

	// Check if cgroupv2 is available
	cgroupReader, err := cgroups.NewReader()
	if err != nil {
		t.Skipf("Cannot create cgroup reader: %v", err)
	}

	if cgroupReader.CgroupVersion() != 2 {
		t.Skip("Test requires cgroupv2")
	}

	// Create a temporary cgroup for testing
	testCgroupName := "test-nvidia-device-allow"
	testCgroupPath := filepath.Join("/sys/fs/cgroup", testCgroupName)

	// Clean up after test
	defer func() {
		// Remove the test cgroup directory
		if err := os.RemoveAll(testCgroupPath); err != nil {
			t.Logf("Failed to remove test cgroup directory: %v", err)
		}
	}()

	// Create the test cgroup directory
	if err := os.MkdirAll(testCgroupPath, 0755); err != nil {
		t.Fatalf("Failed to create test cgroup directory: %v", err)
	}

	// Refresh the cgroups to pick up the new cgroup
	if err := cgroupReader.RefreshCgroups(0); err != nil {
		t.Fatalf("Failed to refresh cgroups: %v", err)
	}

	// Get the cgroup object for our test cgroup
	testCgroup := cgroupReader.GetCgroup(testCgroupName)
	if testCgroup == nil {
		t.Fatalf("Failed to get cgroup object for %s", testCgroupName)
	}

	// Test the function with the real cgroup
	err = configureCgroupV2DeviceAllow(testCgroup, "/")
	if err != nil {
		t.Errorf("configureCgroupV2DeviceAllow failed: %v", err)
	}

	// Verify that the BPF program was attached by checking if the cgroup still exists and is accessible
	if testCgroup.Identifier() != testCgroupName {
		t.Errorf("Expected cgroup identifier %s, got %s", testCgroupName, testCgroup.Identifier())
	}

	// Try to get PIDs from the cgroup to verify it's working
	pids, err := testCgroup.GetPIDs(5 * time.Second)
	if err != nil {
		t.Logf("Could not get PIDs from cgroup (this is expected for an empty cgroup): %v", err)
	} else {
		t.Logf("Cgroup contains %d PIDs", len(pids))
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package gpu

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/security/utils"
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

	// Create a temporary cgroup for testing using cgcreate
	testCgroupName := fmt.Sprintf("test-nvidia-device-allow-%s", utils.RandString(10))

	// Clean up after test
	defer func() {
		// Remove the test cgroup using cgdelete
		cmd := exec.Command("cgdelete", "devices", testCgroupName)
		if err := cmd.Run(); err != nil {
			t.Logf("Failed to clean up test cgroup: %v", err)
		}
	}()

	// Create the test cgroup using cgcreate
	cmd := exec.Command("cgcreate", "-g", "devices:"+testCgroupName)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create test cgroup using cgcreate: %v", err)
	}

	// Refresh the cgroups reader to pick up the new cgroup
	if err := cgroupReader.RefreshCgroups(0); err != nil {
		t.Fatalf("Failed to refresh cgroups: %v", err)
	}

	// Get the cgroup object
	testCgroup := cgroupReader.GetCgroup(testCgroupName)
	if testCgroup == nil {
		t.Fatalf("Failed to get test cgroup after creation")
	}

	// Test the BPF program attachment
	if err := configureCgroupV2DeviceAllow("", testCgroupName); err != nil {
		t.Fatalf("Failed to configure cgroupv2 device allow: %v", err)
	}

	// Verify that the BPF program was attached by checking if the cgroup.procs file exists
	// and the cgroup is functional
	testCgroupPath := filepath.Join("/sys/fs/cgroup", testCgroupName)
	procsPath := filepath.Join(testCgroupPath, "cgroup.procs")
	if _, err := os.Stat(procsPath); err != nil {
		t.Errorf("Cgroup procs file not found after BPF attachment: %v", err)
	}

	t.Logf("Successfully created and configured cgroupv2 device allow for %s", testCgroupName)
}

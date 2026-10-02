// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package tags

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/stretchr/testify/assert"
)

func TestGetTags(t *testing.T) {
	// Create a temporary directory for our test
	tmpDir := t.TempDir()

	// Override procFSRoot to use our temporary directory
	originalProcFSRoot := kernel.ProcFSRoot
	kernel.ProcFSRoot = func() string { return tmpDir }
	defer func() { kernel.ProcFSRoot = originalProcFSRoot }()

	// Keep the AMD detection away from the host sysfs
	originalSysFSRoot := kernel.SysFSRoot
	sysDir := t.TempDir()
	kernel.SysFSRoot = func() string { return sysDir }
	defer func() { kernel.SysFSRoot = originalSysFSRoot }()

	tests := []struct {
		name     string
		setup    func() error
		wantTags []string
	}{
		{
			name: "no NVIDIA directory",
			setup: func() error {
				// Don't create anything, directory should not exist
				return nil
			},
			wantTags: nil,
		},
		{
			name: "NVIDIA directory exists but empty",
			setup: func() error {
				nvidiaPath := filepath.Join(tmpDir, "driver", "nvidia", "gpus")
				return os.MkdirAll(nvidiaPath, 0755)
			},
			wantTags: nil,
		},
		{
			name: "NVIDIA directory with one GPU",
			setup: func() error {
				nvidiaPath := filepath.Join(tmpDir, "driver", "nvidia", "gpus")
				if err := os.MkdirAll(nvidiaPath, 0755); err != nil {
					return err
				}
				// Create a dummy GPU entry
				return os.WriteFile(filepath.Join(nvidiaPath, "0"), []byte("dummy"), 0644)
			},
			wantTags: []string{"gpu_host:true"},
		},
		{
			name: "NVIDIA directory with multiple GPUs",
			setup: func() error {
				nvidiaPath := filepath.Join(tmpDir, "driver", "nvidia", "gpus")
				if err := os.MkdirAll(nvidiaPath, 0755); err != nil {
					return err
				}
				// Create multiple dummy GPU entries
				for i := 0; i < 2; i++ {
					if err := os.WriteFile(filepath.Join(nvidiaPath, strconv.Itoa(i)), []byte("dummy"), 0644); err != nil {
						return err
					}
				}
				return nil
			},
			wantTags: []string{"gpu_host:true"},
		},
		{
			name: "NVIDIA directory exists but not readable",
			setup: func() error {
				nvidiaPath := filepath.Join(tmpDir, "driver", "nvidia", "gpus")
				if err := os.MkdirAll(nvidiaPath, 0755); err != nil {
					return err
				}
				if err := os.Chmod(nvidiaPath, 0000); err != nil {
					return err
				}
				return nil
			},
			wantTags: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				// Clean up any existing files before each test
				if err := os.RemoveAll(filepath.Join(tmpDir, "driver")); err != nil {
					t.Fatalf("Failed to clean up test directory: %v", err)
				}
			}()

			if tt.setup != nil {
				if err := tt.setup(); err != nil {
					t.Fatalf("Setup failed: %v", err)
				}
			}
			gotTags := getTags()
			assert.Equal(t, tt.wantTags, gotTags)
		})
	}
}

func TestGetTagsAMD(t *testing.T) {
	originalProcFSRoot := kernel.ProcFSRoot
	procDir := t.TempDir()
	kernel.ProcFSRoot = func() string { return procDir }
	defer func() { kernel.ProcFSRoot = originalProcFSRoot }()

	originalSysFSRoot := kernel.SysFSRoot
	defer func() { kernel.SysFSRoot = originalSysFSRoot }()

	type card struct {
		name   string
		device string // PCI address, or a platform device name
		vendor string
		driver string // driver link target, empty for none
		uevent string // uevent content, empty for none
		noLink bool   // card without a device link
	}

	tests := []struct {
		name     string
		cards    []card
		noDRM    bool
		expected []string
	}{
		{name: "no DRM class", noDRM: true},
		{name: "no cards"},
		{
			name:     "AMD GPU bound to amdgpu",
			cards:    []card{{name: "card0", device: "0000:c1:00.0", vendor: "0x1002", driver: "amdgpu"}},
			expected: []string{"gpu_host:true"},
		},
		{
			name:     "AMD GPU bound through uevent",
			cards:    []card{{name: "card1", device: "0000:c1:00.0", vendor: "0x1002", uevent: "DRIVER=amdgpu\nPCI_ID=1002:75A3\n"}},
			expected: []string{"gpu_host:true"},
		},
		{
			name:  "AMD device bound to another driver",
			cards: []card{{name: "card0", device: "0000:c1:00.0", vendor: "0x1002", driver: "vfio-pci"}},
		},
		{
			name:  "AMD device without a driver",
			cards: []card{{name: "card0", device: "0000:c1:00.0", vendor: "0x1002"}},
		},
		{
			name:  "non-AMD GPU",
			cards: []card{{name: "card0", device: "0000:03:00.0", vendor: "0x1a03", driver: "ast"}},
		},
		{
			name:  "compute partition platform device",
			cards: []card{{name: "card1", device: "amdgpu_xcp_0", vendor: "0x1002", driver: "amdgpu"}},
		},
		{
			name:  "connector node",
			cards: []card{{name: "card0-DP-1", device: "0000:c1:00.0", vendor: "0x1002", driver: "amdgpu"}},
		},
		{
			name:  "card without device link",
			cards: []card{{name: "card0", noLink: true}},
		},
		{
			name: "AMD GPU next to a non-AMD card",
			cards: []card{
				{name: "card0", device: "0000:03:00.0", vendor: "0x1a03", driver: "ast"},
				{name: "card1", device: "0000:c1:00.0", vendor: "0x1002", driver: "amdgpu"},
			},
			expected: []string{"gpu_host:true"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sysDir := t.TempDir()
			kernel.SysFSRoot = func() string { return sysDir }
			if !tt.noDRM {
				assert.NoError(t, os.MkdirAll(filepath.Join(sysDir, "class", "drm"), 0o755))
			}
			for _, c := range tt.cards {
				cardDir := filepath.Join(sysDir, "class", "drm", c.name)
				assert.NoError(t, os.MkdirAll(cardDir, 0o755))
				if c.noLink {
					continue
				}
				deviceDir := filepath.Join(sysDir, "devices", "pci0000:00", c.device)
				assert.NoError(t, os.MkdirAll(deviceDir, 0o755))
				assert.NoError(t, os.WriteFile(filepath.Join(deviceDir, "vendor"), []byte(c.vendor+"\n"), 0o644))
				if c.driver != "" {
					driverDir := filepath.Join(sysDir, "bus", "pci", "drivers", c.driver)
					assert.NoError(t, os.MkdirAll(driverDir, 0o755))
					assert.NoError(t, os.Symlink(driverDir, filepath.Join(deviceDir, "driver")))
				}
				if c.uevent != "" {
					assert.NoError(t, os.WriteFile(filepath.Join(deviceDir, "uevent"), []byte(c.uevent), 0o644))
				}
				assert.NoError(t, os.Symlink(deviceDir, filepath.Join(cardDir, "device")))
			}

			assert.Equal(t, tt.expected, getTags())
		})
	}
}

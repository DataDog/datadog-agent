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

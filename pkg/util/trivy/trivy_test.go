// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && containerd

// Package trivy holds the scan components
package trivy

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/containerd/containerd/v2/core/mount"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/sbom"
)

// TestExtractLayersFromOverlayFSMounts checks if the function correctly extracts layer paths from Mount options.
func TestExtractLayersFromOverlayFSMounts(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mounts []mount.Mount
		want   []string
	}{
		{
			name:   "No mounts",
			mounts: []mount.Mount{},
		},
		{
			name:   "Single upperdir",
			mounts: []mount.Mount{{Options: []string{"someoption=somevalue", "upperdir=/path/to/upper"}}},
			want:   []string{"/path/to/upper"},
		},
		{
			name:   "Single lowerdir",
			mounts: []mount.Mount{{Options: []string{"someoption=somevalue", "lowerdir=/path/to/lower"}}},
			want:   []string{"/path/to/lower"},
		},
		{
			name:   "Multiple lowerdir",
			mounts: []mount.Mount{{Options: []string{"someoption=somevalue", "lowerdir=/path/to/lower1:/path/to/lower2"}}},
			want:   []string{"/path/to/lower1", "/path/to/lower2"},
		},
		{
			name:   "Multiple options",
			mounts: []mount.Mount{{Options: []string{"someoption=somevalue", "upperdir=/path/to/upper", "lowerdir=/path/to/lower1:/path/to/lower2"}}},
			want:   []string{"/path/to/upper", "/path/to/lower1", "/path/to/lower2"},
		},
		{
			name: "Multiple mounts",
			mounts: []mount.Mount{
				{Options: []string{"someoption=somevalue", "upperdir=/path/to/upper1"}},
				{Options: []string{"someoption=somevalue", "lowerdir=/path/to/lower1:/path/to/lower2"}},
			},
			want: []string{"/path/to/upper1", "/path/to/lower1", "/path/to/lower2"},
		},
		{
			// A single-layer image is exposed by containerd as a single bind mount
			// (no lowerdir/upperdir); its only layer is the mount source.
			name:   "Single-layer bind mount",
			mounts: []mount.Mount{{Type: "bind", Source: "/path/to/snapshots/132/fs", Options: []string{"ro", "rbind"}}},
			want:   []string{"/path/to/snapshots/132/fs"},
		},
		{
			// Overlay options take precedence; the mount source is not a layer path.
			name:   "Overlay mount source ignored",
			mounts: []mount.Mount{{Type: "overlay", Source: "overlay", Options: []string{"lowerdir=/path/to/lower"}}},
			want:   []string{"/path/to/lower"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractLayersFromOverlayFSMounts(tt.mounts))
		})
	}
}

// TestScanFilesystemImageCreated covers the use_mount path, where the caller
// supplies the build time of the image it mounted.
func TestScanFilesystemImageCreated(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "etc"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "etc", "os-release"), []byte("ID=alpine\nVERSION_ID=3.20.0\n"), 0o644))

	tests := []struct {
		name    string
		created time.Time
		want    []string
	}{
		{
			name:    "offset is normalized to UTC",
			created: time.Date(2024, time.November, 7, 5, 26, 15, 123456789, time.FixedZone("UTC+5", 5*60*60)),
			want:    []string{"2024-11-07T00:26:15.123456789Z"},
		},
		{
			name: "config omits created",
		},
	}
	c := NewCollectorForCLI()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := c.scanFilesystem(t.Context(), root, sbom.ScanOptions{Analyzers: []string{OSAnalyzers}}, false, tt.created)
			require.NoError(t, err)

			var got []string
			for _, p := range report.ToCycloneDX().GetMetadata().GetComponent().GetProperties() {
				if p.GetName() == imageCreatedPropertyKey {
					got = append(got, p.GetValue())
				}
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

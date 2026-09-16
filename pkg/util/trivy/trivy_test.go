// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && containerd

// Package trivy holds the scan components
package trivy

import (
	"testing"

	"github.com/containerd/containerd/v2/core/mount"
	"github.com/stretchr/testify/assert"
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

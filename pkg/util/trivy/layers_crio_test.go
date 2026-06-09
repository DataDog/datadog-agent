// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && crio

package trivy

import (
	"testing"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCRIOLayerPaths(t *testing.T) {
	// Distinct value namespaces for the path-encoded DiffID and the
	// imgMeta layer DiffID so a positional swap is detectable -- the
	// same-name fixtures of the original test let an off-by-one go silently.
	imgMeta := &workloadmeta.ContainerImageMetadata{
		Layers: []workloadmeta.ContainerImageLayer{
			{DiffID: "sha256:digest_base"},
			{DiffID: ""}, // empty-history marker; must be skipped
			{DiffID: "sha256:digest_middle"},
			{DiffID: "sha256:digest_top"},
		},
	}
	// GetCRIOImageLayers prepends each path while iterating
	// imgMeta.Layers, so its output is top-to-base.
	lowerDirs := []string{
		"/var/lib/containers/storage/overlay/diff_top/diff",
		"/var/lib/containers/storage/overlay/diff_middle/diff",
		"/var/lib/containers/storage/overlay/diff_base/diff",
	}

	t.Run("happy_path_pairs_in_image_config_order", func(t *testing.T) {
		got, err := buildCRIOLayerPaths(imgMeta, lowerDirs)
		require.NoError(t, err)
		want := []struct{ diffID, digest, path string }{
			{"sha256:diff_base", "sha256:digest_base", "/var/lib/containers/storage/overlay/diff_base/diff"},
			{"sha256:diff_middle", "sha256:digest_middle", "/var/lib/containers/storage/overlay/diff_middle/diff"},
			{"sha256:diff_top", "sha256:digest_top", "/var/lib/containers/storage/overlay/diff_top/diff"},
		}
		require.Len(t, got, len(want))
		for i, w := range want {
			assert.Equal(t, w.diffID, got[i].DiffID)
			assert.Equal(t, w.digest, got[i].Digest)
			assert.Equal(t, w.path, got[i].Path)
		}
	})

	t.Run("count_mismatch_extra_lower_dir", func(t *testing.T) {
		extra := append([]string{}, lowerDirs...)
		extra = append(extra, "/var/lib/containers/storage/overlay/diff_spurious/diff")
		_, err := buildCRIOLayerPaths(imgMeta, extra)
		require.ErrorIs(t, err, errLayerCountMismatch)
	})

	t.Run("count_mismatch_short_lower_dir", func(t *testing.T) {
		_, err := buildCRIOLayerPaths(imgMeta, lowerDirs[:1])
		require.ErrorIs(t, err, errLayerCountMismatch)
	})
}

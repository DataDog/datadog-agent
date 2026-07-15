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
	// The layer DiffIDs (digest_*) and the lowerDir basenames (diff_*) use
	// distinct value namespaces so a layer paired with the wrong path is
	// detectable rather than silently masked by same-name fixtures.
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

	// manifestDigests are the per-layer compressed-blob digests from the image
	// manifest, in image-config (base-to-top) order, so they pair with the
	// non-empty layers. A third namespace (mdigest_*) keeps them distinct from
	// the diff_ids and the lowerDir basenames.
	manifestDigests := []string{"sha256:mdigest_base", "sha256:mdigest_middle", "sha256:mdigest_top"}

	t.Run("happy_path_pairs_in_image_config_order", func(t *testing.T) {
		got, err := buildCRIOLayerPaths(imgMeta, lowerDirs, manifestDigests)
		require.NoError(t, err)
		want := []struct{ diffID, digest, path string }{
			{"sha256:digest_base", "sha256:mdigest_base", "/var/lib/containers/storage/overlay/diff_base/diff"},
			{"sha256:digest_middle", "sha256:mdigest_middle", "/var/lib/containers/storage/overlay/diff_middle/diff"},
			{"sha256:digest_top", "sha256:mdigest_top", "/var/lib/containers/storage/overlay/diff_top/diff"},
		}
		require.Len(t, got, len(want))
		for i, w := range want {
			assert.Equal(t, w.diffID, got[i].DiffID)
			assert.Equal(t, w.digest, got[i].Digest)
			assert.Equal(t, w.path, got[i].Path)
		}
	})

	t.Run("nil_manifest_digests_leave_digest_empty", func(t *testing.T) {
		got, err := buildCRIOLayerPaths(imgMeta, lowerDirs, nil)
		require.NoError(t, err)
		require.Len(t, got, len(lowerDirs))
		for i := range got {
			assert.NotEmpty(t, got[i].DiffID)
			assert.Empty(t, got[i].Digest, "no manifest digests means no LayerDigest")
		}
	})

	t.Run("misaligned_manifest_digests_leave_digest_empty", func(t *testing.T) {
		// Two digests for three layers: the counts disagree, so any positional
		// pairing would be wrong. Drop the digests rather than mispair them.
		got, err := buildCRIOLayerPaths(imgMeta, lowerDirs, manifestDigests[:2])
		require.NoError(t, err)
		require.Len(t, got, len(lowerDirs))
		for i := range got {
			assert.Empty(t, got[i].Digest, "misaligned manifest digests means no LayerDigest")
		}
	})

	t.Run("count_mismatch_extra_lower_dir", func(t *testing.T) {
		extra := append([]string{}, lowerDirs...)
		extra = append(extra, "/var/lib/containers/storage/overlay/diff_spurious/diff")
		_, err := buildCRIOLayerPaths(imgMeta, extra, manifestDigests)
		require.ErrorIs(t, err, errLayerCountMismatch)
	})

	t.Run("count_mismatch_short_lower_dir", func(t *testing.T) {
		_, err := buildCRIOLayerPaths(imgMeta, lowerDirs[:1], manifestDigests)
		require.ErrorIs(t, err, errLayerCountMismatch)
	})
}

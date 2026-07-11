// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && docker

package trivy

import (
	"testing"

	dimage "github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dockerInspect builds a synthetic docker InspectResponse modelling an
// overlay2-backed image. lower is a colon-joined chain of paths
// (top-down except the topmost, per docker daemon convention), upper
// is the topmost path. diffIDs is in image-config (bottom-up) order.
func dockerInspect(lower, upper string, diffIDs []string) dimage.InspectResponse {
	return dimage.InspectResponse{
		GraphDriver: &storage.DriverData{
			Name: "overlay2",
			Data: map[string]string{
				"LowerDir": lower,
				"UpperDir": upper,
			},
		},
		RootFS: dimage.RootFS{Layers: diffIDs},
	}
}

func TestBuildDockerLayerPaths(t *testing.T) {
	diffIDs := []string{
		"sha256:base",
		"sha256:mid1",
		"sha256:mid2",
		"sha256:top",
	}

	t.Run("happy_path_pairs_diffid_with_path", func(t *testing.T) {
		// LowerDir is top-down without the topmost: mid2 -> mid1 -> base.
		// UpperDir is the topmost (top).
		// After reversal in buildDockerLayerPaths the slice is:
		//   [base, mid1, mid2, top]
		inspect := dockerInspect("/dock/mid2:/dock/mid1:/dock/base", "/dock/top", diffIDs)
		got, err := buildDockerLayerPaths(inspect)
		require.NoError(t, err)

		wantPaths := []string{"/dock/base", "/dock/mid1", "/dock/mid2", "/dock/top"}
		require.Len(t, diffIDs, len(got))
		for i := range got {
			assert.Equal(t, diffIDs[i], got[i].DiffID)
			assert.Equal(t, wantPaths[i], got[i].Path)
			// Docker daemon does not expose per-layer manifest digests
			// reliably; the builder leaves Digest empty rather than
			// risk pairing a wrong one.
			assert.Empty(t, got[i].Digest)
		}
	})

	t.Run("count_mismatch_too_few_paths", func(t *testing.T) {
		inspect := dockerInspect("/dock/mid1:/dock/base", "/dock/top", diffIDs) // 3 paths, 4 diff_ids
		_, err := buildDockerLayerPaths(inspect)
		require.ErrorIs(t, err, errLayerCountMismatch)
	})

	t.Run("count_mismatch_too_many_paths", func(t *testing.T) {
		inspect := dockerInspect("/dock/spurious:/dock/mid2:/dock/mid1:/dock/base", "/dock/top", diffIDs) // 5 paths
		_, err := buildDockerLayerPaths(inspect)
		require.ErrorIs(t, err, errLayerCountMismatch)
	})

	t.Run("only_upperdir_single_layer", func(t *testing.T) {
		inspect := dockerInspect("", "/dock/only", []string{"sha256:only"})
		got, err := buildDockerLayerPaths(inspect)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "/dock/only", got[0].Path)
		assert.Equal(t, "sha256:only", got[0].DiffID)
	})
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package trivy

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/stretchr/testify/require"
)

func TestMemoryCache(t *testing.T) {
	cache := newMemoryCache()
	ctx := t.Context()

	// Everything is missing on an empty cache.
	_, err := cache.GetArtifact(ctx, "artifact")
	require.Error(t, err)
	_, err = cache.GetBlob(ctx, "blob1")
	require.Error(t, err)

	missingArtifact, missingBlobs, err := cache.MissingBlobs(ctx, "artifact", []string{"blob1", "blob2"})
	require.NoError(t, err)
	require.True(t, missingArtifact)
	require.Equal(t, []string{"blob1", "blob2"}, missingBlobs)

	// Store an artifact and a blob, then read them back.
	artifactInfo := newTestArtifactInfo()
	require.NoError(t, cache.PutArtifact(ctx, "artifact", artifactInfo))
	blobInfo := newTestBlobInfo()
	require.NoError(t, cache.PutBlob(ctx, "blob1", blobInfo))

	gotArtifact, err := cache.GetArtifact(ctx, "artifact")
	require.NoError(t, err)
	require.Equal(t, artifactInfo, gotArtifact)

	gotBlob, err := cache.GetBlob(ctx, "blob1")
	require.NoError(t, err)
	require.Equal(t, blobInfo, gotBlob)

	// Now only blob2 is missing and the artifact is present.
	missingArtifact, missingBlobs, err = cache.MissingBlobs(ctx, "artifact", []string{"blob1", "blob2"})
	require.NoError(t, err)
	require.False(t, missingArtifact)
	require.Equal(t, []string{"blob2"}, missingBlobs)

	// DeleteBlobs removes a blob.
	require.NoError(t, cache.DeleteBlobs(ctx, []string{"blob1"}))
	_, err = cache.GetBlob(ctx, "blob1")
	require.Error(t, err)

	// Clear empties the cache.
	require.NoError(t, cache.Clear(ctx))
	_, err = cache.GetArtifact(ctx, "artifact")
	require.Error(t, err)
}

// TestMemoryCacheConcurrent exercises the cache from many goroutines, as a fast
// scan does when trivy analyzes image layers in parallel. Run with -race.
func TestMemoryCacheConcurrent(t *testing.T) {
	cache := newMemoryCache()
	ctx := t.Context()

	const goroutines = 16
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				id := fmt.Sprintf("blob-%d-%d", g, i)
				require.NoError(t, cache.PutBlob(ctx, id, newTestBlobInfo()))
				require.NoError(t, cache.PutArtifact(ctx, fmt.Sprintf("art-%d", g), newTestArtifactInfo()))
				_, _, _ = cache.MissingBlobs(ctx, fmt.Sprintf("art-%d", g), []string{id, "missing"})
				_, _ = cache.GetBlob(ctx, id)
			}
		}(g)
	}
	wg.Wait()
}

func newTestArtifactInfo() types.ArtifactInfo {
	return types.ArtifactInfo{
		SchemaVersion: 1,
		Architecture:  "amd64",
		Created:       time.Date(2023, 2, 28, 0, 0, 0, 0, time.UTC),
		DockerVersion: "18.06.1-ce",
		OS:            "linux",
		HistoryPackages: []types.Package{
			{
				Name:    "musl",
				Version: "1.2.3",
			},
		},
	}
}

func newTestBlobInfo() types.BlobInfo {
	return types.BlobInfo{
		SchemaVersion: 1,
		OS: types.OS{
			Family: "alpine",
			Name:   "3.17",
		},
	}
}

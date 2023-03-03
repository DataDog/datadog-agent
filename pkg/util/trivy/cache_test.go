// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy
// +build trivy

package trivy

import (
	"strconv"
	"testing"
	"time"

	"github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/stretchr/testify/require"
)

// TTL needs to be long enough so that keys don't expire in the middle of a test
var testCacheTTL = 1 * time.Hour

func TestBadgerCache_Artifacts(t *testing.T) {
	cache, err := NewBadgerCache(t.TempDir(), testCacheTTL)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cache.Close())
	}()

	_, err = cache.GetArtifact("non-existing-ID")
	require.Error(t, err)

	artifactID := "some_ID"
	artifactInfo := newTestArtifactInfo()

	err = cache.PutArtifact(artifactID, artifactInfo)
	require.NoError(t, err)

	storedArtifact, err := cache.GetArtifact(artifactID)
	require.NoError(t, err)
	require.Equal(t, artifactInfo, storedArtifact)
}

func TestBadgerCache_Blobs(t *testing.T) {
	cache, err := NewBadgerCache(t.TempDir(), testCacheTTL)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cache.Close())
	}()

	_, err = cache.GetBlob("non-existing-ID")
	require.Error(t, err)

	blobID := "some_ID"
	blobInfo := newTestBlobInfo()

	err = cache.PutBlob(blobID, blobInfo)
	require.NoError(t, err)

	storedBlobInfo, err := cache.GetBlob(blobID)
	require.NoError(t, err)
	require.Equal(t, blobInfo, storedBlobInfo)
}

func TestBadgerCache_DeleteBlobs(t *testing.T) {
	cache, err := NewBadgerCache(t.TempDir(), testCacheTTL)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cache.Close())
	}()

	// Store 3 blobs with IDs "0", "1", "2"
	for blobID, osName := range []string{"3.15", "3.16", "3.17"} {
		blobInfo := types.BlobInfo{
			SchemaVersion: 1,
			OS: types.OS{
				Family: "alpine",
				Name:   osName,
			},
		}

		err := cache.PutBlob(strconv.Itoa(blobID), blobInfo)
		require.NoError(t, err)
	}

	// Delete 2 blobs
	err = cache.DeleteBlobs([]string{"0", "1"})
	require.NoError(t, err)

	// Check that the deleted blobs are no longer there, but the other one is
	_, err = cache.GetBlob("0")
	require.Error(t, err)
	_, err = cache.GetBlob("1")
	require.Error(t, err)
	_, err = cache.GetBlob("2")
	require.NoError(t, err)
}

func TestBadgerCache_MissingBlobs(t *testing.T) {
	cache, err := NewBadgerCache(t.TempDir(), testCacheTTL)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cache.Close())
	}()

	existingArtifactID := "1"
	existingBlobID := "2"

	err = cache.PutArtifact(existingArtifactID, newTestArtifactInfo())
	require.NoError(t, err)

	err = cache.PutBlob(existingBlobID, newTestBlobInfo())
	require.NoError(t, err)

	nonExistingBlobIDs := []string{"non-existing-1", "non-existing-2"}
	inputBlobIDs := append([]string{existingBlobID}, nonExistingBlobIDs...)

	// Artifact exists. Some blobs missing.
	missingArtifact, missingBlobIDs, err := cache.MissingBlobs(existingArtifactID, inputBlobIDs)
	require.False(t, missingArtifact)
	require.Equal(t, nonExistingBlobIDs, missingBlobIDs)
	require.NoError(t, err)

	// Artifact does not exist. Some blobs missing.
	missingArtifact, missingBlobIDs, err = cache.MissingBlobs("non-existing-ID", inputBlobIDs)
	require.True(t, missingArtifact)
	require.Equal(t, nonExistingBlobIDs, missingBlobIDs)
	require.NoError(t, err)
}

func TestBadgerCache_Clear(t *testing.T) {
	cache, err := NewBadgerCache(t.TempDir(), testCacheTTL)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cache.Close())
	}()

	artifactID := "some_ID"

	err = cache.PutArtifact(artifactID, newTestArtifactInfo())
	require.NoError(t, err)

	err = cache.Clear()
	require.NoError(t, err)

	_, err = cache.GetArtifact(artifactID)
	require.Error(t, err)
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

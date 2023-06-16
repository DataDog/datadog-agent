// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package trivy

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/stretchr/testify/require"
)

var (
	defaultCacheSize = 100
	defaultDiskSize  = 1000000
)

func TestCustomBoltCache_Artifacts(t *testing.T) {
	cache, _, err := NewCustomBoltCache(t.TempDir(), defaultCacheSize, defaultDiskSize)
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

func TestCustomBoltCache_Blobs(t *testing.T) {
	cache, _, err := NewCustomBoltCache(t.TempDir(), defaultCacheSize, defaultDiskSize)
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

func TestCustomBoltCache_DeleteBlobs(t *testing.T) {
	cache, _, err := NewCustomBoltCache(t.TempDir(), defaultCacheSize, defaultDiskSize)
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

func TestCustomBoltCache_MissingBlobs(t *testing.T) {
	cache, _, err := NewCustomBoltCache(t.TempDir(), defaultCacheSize, defaultDiskSize)
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

func TestCustomBoltCache_Clear(t *testing.T) {
	cache, _, err := NewCustomBoltCache(t.TempDir(), defaultCacheSize, defaultDiskSize)
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

func TestCustomBoltCache_CurrentObjectSize(t *testing.T) {
	cache, _, err := NewCustomBoltCache(t.TempDir(), defaultCacheSize, defaultDiskSize)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cache.Close())
	}()

	serializedArtifactInfo, err := json.Marshal(newTestArtifactInfo())
	require.NoError(t, err)

	// Store two artifacts
	artifactIDs := []string{"some_ID", "some_other_ID"}
	for _, id := range artifactIDs {
		err = cache.PutArtifact(id, newTestArtifactInfo())
		require.NoError(t, err)
	}

	// Check that the currentCachedObjectTotalSize is equal to the size of the two artifacts
	persistentCache := cache.(*TrivyCache).Cache.(*PersistentCache)
	require.Equal(t, len(serializedArtifactInfo)*len(artifactIDs), persistentCache.GetCurrentCachedObjectTotalSize())

	// Remove one artifact and check that currentCachedObjectTotalSize is the size of 1 artifact
	err = persistentCache.Remove([]string{"some_ID"})
	require.NoError(t, err)
	require.Equal(t, len(serializedArtifactInfo)*(len(artifactIDs)-1), persistentCache.GetCurrentCachedObjectTotalSize())

	// Remove the already removed artifact and the last one, check that currentCachedObjectTotalSize is 0
	err = persistentCache.Remove(artifactIDs)
	require.NoError(t, err)
	require.Equal(t, 0, persistentCache.GetCurrentCachedObjectTotalSize())
}

func TestCustomBoltCache_Eviction(t *testing.T) {
	// Set the maximum cache entries to 2
	cache, _, err := NewCustomBoltCache(t.TempDir(), 2, defaultDiskSize)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cache.Close())
	}()

	// store 3 artifacts with different sizes
	artifactIDs := []string{"key1", "key2", "key3"}
	artifactSize := make(map[string]int)
	for i, id := range artifactIDs {

		artifact := newTestArtifactInfo()
		artifact.Architecture = strings.Repeat("A", i*7)

		serializedArtifactInfo, err := json.Marshal(artifact)
		require.NoError(t, err)
		artifactSize[id] = len(serializedArtifactInfo)

		err = cache.PutArtifact(id, artifact)
		require.NoError(t, err)
	}

	// Make sure only the artifact 2 and 3 are stored and currentCachedObjectTotalSize is correctly updated
	persistentCache := cache.(*TrivyCache).Cache.(*PersistentCache)
	require.Equal(t, artifactSize["key2"]+artifactSize["key3"], persistentCache.GetCurrentCachedObjectTotalSize())

	_, err = cache.GetArtifact("key2")
	require.NoError(t, err)

	_, err = cache.GetArtifact("key3")
	require.NoError(t, err)

	_, err = cache.GetArtifact("key1")
	require.Error(t, err)
}

func TestCustomBoltCache_DiskSizeLimit(t *testing.T) {
	// Set the max disk size to the size of one item
	artifact := newTestArtifactInfo()
	artifact.Architecture = "architecture1"
	serializedArtifactInfo, err := json.Marshal(artifact)
	require.NoError(t, err)

	cache, _, err := NewCustomBoltCache(t.TempDir(), defaultCacheSize, len(serializedArtifactInfo))
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cache.Close())
	}()
	// Store two items
	err = cache.PutArtifact("key1", artifact)
	require.NoError(t, err)

	artifact.Architecture = "architecture2"
	err = cache.PutArtifact("key2", artifact)
	require.NoError(t, err)

	// Verify that only the second item is stored and currentCachedObjectTotalSize is correctly updated
	retrievedArtifact, err := cache.GetArtifact("key2")
	require.NoError(t, err)
	require.Equal(t, artifact, retrievedArtifact)

	_, err = cache.GetArtifact("key1")
	require.Error(t, err)

	persistentCache := cache.(*TrivyCache).Cache.(*PersistentCache)
	require.Equal(t, len(serializedArtifactInfo), persistentCache.GetCurrentCachedObjectTotalSize())
}

func TestCustomBoltCache_GarbageCollector(t *testing.T) {
	// Create a workload meta global store containing two images with a distinct artifactID/blobs and a shared blob
	globalStore := workloadmeta.CreateGlobalStore(workloadmeta.NodeAgentCatalog)
	globalStore.Start(context.TODO())
	image1 := &workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainerImageMetadata,
			ID:   "image1",
		},
	}

	image2 := &workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainerImageMetadata,
			ID:   "image2",
		},
	}

	// Test with no SBOM
	image3 := &workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainerImageMetadata,
			ID:   "image3",
		},
	}

	globalStore.Reset([]workloadmeta.Entity{image1, image2, image3}, workloadmeta.SourceAll)

	cache, cacheCleaner, err := NewCustomBoltCache(t.TempDir(), defaultCacheSize, defaultDiskSize)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cache.Close())
	}()

	// link image1 to artifact key1, an owned blob and a shared blob
	cacheCleaner.setKeysForEntity("image1", []string{"key1", "blob1", "sharedBlob"})
	// link image2 to artifact key2, an owned blob and a shared blob
	cacheCleaner.setKeysForEntity("image2", []string{"key2", "blob2", "sharedBlob"})

	// Create a goroutine that calls cacheCleaner.Clean every 500ms
	go func() {
		cleanTicker := time.NewTicker(500 * time.Millisecond)
		for range cleanTicker.C {
			cacheCleaner.Clean()
		}
	}()

	// Store the artifacts of both images, the exclusive blobs and the shared blob
	err = cache.PutArtifact("key1", newTestArtifactInfo())
	require.NoError(t, err)

	err = cache.PutArtifact("key2", newTestArtifactInfo())
	require.NoError(t, err)

	err = cache.PutBlob("sharedBlob", newTestBlobInfo())
	require.NoError(t, err)

	err = cache.PutBlob("blob1", newTestBlobInfo())
	require.NoError(t, err)

	err = cache.PutBlob("blob2", newTestBlobInfo())
	require.NoError(t, err)

	// Wait for the garbage collector to be called
	time.Sleep(time.Second)

	// Check that no cache object was removed
	artifact, err := cache.GetArtifact("key1")
	require.NoError(t, err)
	require.Equal(t, newTestArtifactInfo(), artifact)

	artifact, err = cache.GetArtifact("key2")
	require.NoError(t, err)
	require.Equal(t, newTestArtifactInfo(), artifact)

	blob, err := cache.GetBlob("sharedBlob")
	require.NoError(t, err)
	require.Equal(t, newTestBlobInfo(), blob)

	blob, err = cache.GetBlob("blob1")
	require.NoError(t, err)
	require.Equal(t, newTestBlobInfo(), blob)

	blob, err = cache.GetBlob("blob2")
	require.NoError(t, err)
	require.Equal(t, newTestBlobInfo(), blob)

	// Remove the second image from the workloadmeta
	globalStore.Reset([]workloadmeta.Entity{image1}, workloadmeta.SourceAll)

	// Wait for the garbage collector to clean up the unused artifact
	time.Sleep(time.Second)

	// Check that only artifact "key2" and "blob2" were removed
	_, err = cache.GetArtifact("key2")
	require.Error(t, err)

	_, err = cache.GetBlob("blob2")
	require.Error(t, err)

	artifact, err = cache.GetArtifact("key1")
	require.NoError(t, err)
	require.Equal(t, newTestArtifactInfo(), artifact)

	blob, err = cache.GetBlob("sharedBlob")
	require.NoError(t, err)
	require.Equal(t, newTestBlobInfo(), blob)

	blob, err = cache.GetBlob("blob1")
	require.NoError(t, err)
	require.Equal(t, newTestBlobInfo(), blob)

	// Check that the currentCachedObjectTotalSize is correct
	serializedArtifactInfo, err := json.Marshal(newTestArtifactInfo())
	require.NoError(t, err)

	serializedBlobInfo, err := json.Marshal(newTestBlobInfo())
	require.NoError(t, err)

	persistentCache := cache.(*TrivyCache).Cache.(*PersistentCache)
	require.Equal(t, 2*len(serializedBlobInfo)+len(serializedArtifactInfo), persistentCache.GetCurrentCachedObjectTotalSize())
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

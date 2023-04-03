// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy
// +build trivy

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
	defaultCacheSize  = 100
	defaultDiskSize   = 1000000
	defaultGcInterval = 1000 * time.Minute
)

func TestBoltCache_Artifacts(t *testing.T) {
	cache, err := NewCustomBoltCache(t.TempDir(), defaultCacheSize, defaultDiskSize, defaultGcInterval)
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

func TestBoltCache_Blobs(t *testing.T) {
	cache, err := NewCustomBoltCache(t.TempDir(), defaultCacheSize, defaultDiskSize, defaultGcInterval)
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

func TestBoltCache_DeleteBlobs(t *testing.T) {
	cache, err := NewCustomBoltCache(t.TempDir(), defaultCacheSize, defaultDiskSize, defaultGcInterval)
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

func TestBoltCache_MissingBlobs(t *testing.T) {
	cache, err := NewCustomBoltCache(t.TempDir(), defaultCacheSize, defaultDiskSize, defaultGcInterval)
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

func TestBoltCache_Clear(t *testing.T) {
	cache, err := NewCustomBoltCache(t.TempDir(), defaultCacheSize, defaultDiskSize, defaultGcInterval)
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

func TestBoltCache_CurrentObjectSize(t *testing.T) {
	cache, err := NewCustomBoltCache(t.TempDir(), defaultCacheSize, defaultDiskSize, defaultGcInterval)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cache.Close())
	}()

	serializedArtifactInfo, err := json.Marshal(newTestArtifactInfo())
	require.NoError(t, err)

	artifactIDs := []string{"some_ID", "some_other_ID"}
	for _, id := range artifactIDs {
		err = cache.PutArtifact(id, newTestArtifactInfo())
		require.NoError(t, err)
	}

	persistentCache := cache.(*TrivyCache).Cache.(*PersistentCache)
	require.Equal(t, len(serializedArtifactInfo)*len(artifactIDs), persistentCache.GetCurrentCachedObjectSize())

	err = persistentCache.Remove([]string{"some_ID"})
	require.NoError(t, err)
	require.Equal(t, len(serializedArtifactInfo)*(len(artifactIDs)-1), persistentCache.GetCurrentCachedObjectSize())

	err = persistentCache.Remove(artifactIDs)
	require.NoError(t, err)
	require.Equal(t, 0, persistentCache.GetCurrentCachedObjectSize())
}

func TestBoltCache_Eviction(t *testing.T) {
	cache, err := NewCustomBoltCache(t.TempDir(), 2, defaultDiskSize, defaultGcInterval)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cache.Close())
	}()
	artifactIDs := []string{"key1", "key2", "key3"}
	artifactSize := make(map[string]int)

	// Make artifacts of different size and record their size
	for i, id := range artifactIDs {

		artifact := newTestArtifactInfo()
		artifact.Architecture = strings.Repeat("A", i*7)

		serializedArtifactInfo, err := json.Marshal(artifact)
		require.NoError(t, err)
		artifactSize[id] = len(serializedArtifactInfo)

		err = cache.PutArtifact(id, artifact)
		require.NoError(t, err)
	}

	persistentCache := cache.(*TrivyCache).Cache.(*PersistentCache)
	require.Equal(t, artifactSize["key2"]+artifactSize["key3"], persistentCache.GetCurrentCachedObjectSize())
}

func TestBoltCache_DiskSizeLimit(t *testing.T) {
	artifact := newTestArtifactInfo()
	artifact.Architecture = "architecture1"
	serializedArtifactInfo, err := json.Marshal(artifact)
	require.NoError(t, err)

	cache, err := NewCustomBoltCache(t.TempDir(), defaultCacheSize, len(serializedArtifactInfo), defaultGcInterval)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cache.Close())
	}()

	err = cache.PutArtifact("key1", artifact)
	require.NoError(t, err)

	artifact.Architecture = "architecture2"
	err = cache.PutArtifact("key2", artifact)
	require.NoError(t, err)

	retrievedArtifact, err := cache.GetArtifact("key2")
	require.NoError(t, err)
	require.Equal(t, artifact, retrievedArtifact)

	_, err = cache.GetArtifact("key1")
	require.Error(t, err)

	persistentCache := cache.(*TrivyCache).Cache.(*PersistentCache)
	require.Equal(t, len(serializedArtifactInfo), persistentCache.GetCurrentCachedObjectSize())
}

func TestBoltCache_GarbageCollector(t *testing.T) {
	globalStore := workloadmeta.CreateGlobalStore(workloadmeta.NodeAgentCatalog)
	globalStore.Start(context.TODO())
	image1 := &workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainerImageMetadata,
			ID:   "image1",
		},
		SBOM: &workloadmeta.SBOM{
			ArtifactID: "key1",
			BlobIDs:    []string{"blob"},
		},
	}

	image2 := &workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainerImageMetadata,
			ID:   "image2",
		},
		SBOM: &workloadmeta.SBOM{
			ArtifactID: "key2",
			BlobIDs:    []string{"blob"},
		},
	}
	globalStore.Reset([]workloadmeta.Entity{image1, image2}, workloadmeta.SourceAll)

	cache, err := NewCustomBoltCache(t.TempDir(), defaultCacheSize, defaultDiskSize, 1*time.Second)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cache.Close())
	}()
	err = cache.PutArtifact("key1", newTestArtifactInfo())
	require.NoError(t, err)

	err = cache.PutArtifact("key2", newTestArtifactInfo())
	require.NoError(t, err)

	err = cache.PutBlob("blob", newTestBlobInfo())
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	artifact, err := cache.GetArtifact("key1")
	require.NoError(t, err)
	require.Equal(t, newTestArtifactInfo(), artifact)

	artifact, err = cache.GetArtifact("key2")
	require.NoError(t, err)
	require.Equal(t, newTestArtifactInfo(), artifact)

	blob, err := cache.GetBlob("blob")
	require.NoError(t, err)
	require.Equal(t, newTestBlobInfo(), blob)

	globalStore.Reset([]workloadmeta.Entity{image1}, workloadmeta.SourceAll)

	time.Sleep(2 * time.Second)

	artifact, err = cache.GetArtifact("key1")
	require.NoError(t, err)
	require.Equal(t, newTestArtifactInfo(), artifact)

	_, err = cache.GetArtifact("key2")
	require.Error(t, err)

	blob, err = cache.GetBlob("blob")
	require.NoError(t, err)
	require.Equal(t, newTestBlobInfo(), blob)

	serializedArtifactInfo, err := json.Marshal(newTestArtifactInfo())
	require.NoError(t, err)

	serializedBlobInfo, err := json.Marshal(newTestBlobInfo())
	require.NoError(t, err)

	persistentCache := cache.(*TrivyCache).Cache.(*PersistentCache)
	require.Equal(t, len(serializedBlobInfo)+len(serializedArtifactInfo), persistentCache.GetCurrentCachedObjectSize())
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

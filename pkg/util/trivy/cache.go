// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

// Package trivy holds trivy related files
package trivy

import (
	"errors"

	"github.com/aquasecurity/trivy/pkg/cache"
	"github.com/aquasecurity/trivy/pkg/fanal/types"
)

// CacheWithCleaner implements trivy's cache interface and adds a Clean method.
type CacheWithCleaner interface {
	cache.Cache
	// clean removes unused cached entries from the cache.
	clean() error
	setKeysForEntity(entity string, cachedKeys []string)
}

func newMemoryCache() *memoryCache {
	return &memoryCache{
		blobs:     make(map[string]types.BlobInfo),
		artifacts: make(map[string]types.ArtifactInfo),
	}
}

type memoryCache struct {
	blobs      map[string]types.BlobInfo
	artifacts  map[string]types.ArtifactInfo
	lastBlobID string
}

func (c *memoryCache) MissingBlobs(artifactID string, blobIDs []string) (missingArtifact bool, missingBlobIDs []string, err error) {
	for _, blobID := range blobIDs {
		if _, err := c.GetBlob(blobID); err != nil {
			missingBlobIDs = append(missingBlobIDs, blobID)
		}
	}

	if _, err := c.GetArtifact(artifactID); err != nil {
		missingArtifact = true
	}

	return
}

func (c *memoryCache) PutArtifact(artifactID string, artifactInfo types.ArtifactInfo) error {
	c.artifacts[artifactID] = artifactInfo
	return nil
}

func (c *memoryCache) PutBlob(blobID string, blobInfo types.BlobInfo) error {
	c.blobs[blobID] = blobInfo
	c.lastBlobID = blobID
	return nil
}

func (c *memoryCache) DeleteBlobs(blobIDs []string) error {
	for _, id := range blobIDs {
		delete(c.blobs, id)
	}
	return nil
}

func (c *memoryCache) GetArtifact(artifactID string) (types.ArtifactInfo, error) {
	art, ok := c.artifacts[artifactID]
	if !ok {
		return types.ArtifactInfo{}, errors.New("not found")
	}
	return art, nil
}

func (c *memoryCache) GetBlob(blobID string) (types.BlobInfo, error) {
	b, ok := c.blobs[blobID]
	if !ok {
		return types.BlobInfo{}, errors.New("not found")
	}
	return b, nil
}

func (c *memoryCache) Close() (err error) {
	c.artifacts = nil
	c.blobs = nil
	return nil
}

func (c *memoryCache) Clear() (err error) {
	return c.Close()
}
func (c *memoryCache) clean() error                      { return nil }
func (c *memoryCache) setKeysForEntity(string, []string) {}

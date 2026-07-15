// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

// Package trivy holds trivy related files
package trivy

import (
	"context"
	"errors"
	"sync"

	"github.com/aquasecurity/trivy/pkg/fanal/types"
)

// newMemoryCache returns an in-memory implementation of trivy's cache.Cache.
// It is created per scan and freed when the scan returns, so it holds a single
// image or filesystem worth of analysis results and needs no eviction.
func newMemoryCache() *memoryCache {
	return &memoryCache{
		blobs:     make(map[string]types.BlobInfo),
		artifacts: make(map[string]types.ArtifactInfo),
	}
}

// memoryCache is safe for concurrent use: with a fast scan trivy analyzes image
// layers in parallel and stores each result from its own goroutine.
type memoryCache struct {
	mu         sync.Mutex
	blobs      map[string]types.BlobInfo
	artifacts  map[string]types.ArtifactInfo
	lastBlobID string
}

func (c *memoryCache) MissingBlobs(_ context.Context, artifactID string, blobIDs []string) (missingArtifact bool, missingBlobIDs []string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, blobID := range blobIDs {
		if _, ok := c.blobs[blobID]; !ok {
			missingBlobIDs = append(missingBlobIDs, blobID)
		}
	}
	_, ok := c.artifacts[artifactID]
	return !ok, missingBlobIDs, nil
}

func (c *memoryCache) PutArtifact(_ context.Context, artifactID string, artifactInfo types.ArtifactInfo) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.artifacts[artifactID] = artifactInfo
	return nil
}

func (c *memoryCache) PutBlob(_ context.Context, blobID string, blobInfo types.BlobInfo) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.blobs[blobID] = blobInfo
	c.lastBlobID = blobID
	return nil
}

func (c *memoryCache) DeleteBlobs(_ context.Context, blobIDs []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range blobIDs {
		delete(c.blobs, id)
	}
	return nil
}

func (c *memoryCache) GetArtifact(_ context.Context, artifactID string) (types.ArtifactInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	art, ok := c.artifacts[artifactID]
	if !ok {
		return types.ArtifactInfo{}, errors.New("not found")
	}
	return art, nil
}

func (c *memoryCache) GetBlob(_ context.Context, blobID string) (types.BlobInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.blobs[blobID]
	if !ok {
		return types.BlobInfo{}, errors.New("not found")
	}
	return b, nil
}

func (c *memoryCache) Close() (err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.artifacts = nil
	c.blobs = nil
	return nil
}

func (c *memoryCache) Clear(_ context.Context) (err error) {
	return c.Close()
}

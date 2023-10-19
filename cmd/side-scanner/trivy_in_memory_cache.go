package main

import (
	"errors"

	"github.com/aquasecurity/trivy/pkg/fanal/types"
)

func newMemoryCache() *memoryCache {
	return &memoryCache{}
}

type memoryCache struct {
}

func (c *memoryCache) MissingBlobs(artifactID string, blobIDs []string) (missingArtifact bool, missingBlobIDs []string, err error) {
	return true, blobIDs, nil
}

func (c *memoryCache) PutArtifact(artifactID string, artifactInfo types.ArtifactInfo) (err error) {
	return nil
}

func (c *memoryCache) PutBlob(blobID string, blobInfo types.BlobInfo) (err error) {
	return nil
}

func (c *memoryCache) DeleteBlobs(blobIDs []string) error {
	return nil
}

func (c *memoryCache) GetArtifact(artifactID string) (artifactInfo types.ArtifactInfo, err error) {
	return types.ArtifactInfo{}, errors.New("not found")
}

func (c *memoryCache) GetBlob(blobID string) (blobInfo types.BlobInfo, err error) {
	return types.BlobInfo{}, errors.New("not found")
}

func (c *memoryCache) Close() (err error) {
	return nil
}

func (c *memoryCache) Clear() (err error) {
	return nil
}

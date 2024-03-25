// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package scanners

import (
	"errors"

	"github.com/aquasecurity/trivy/pkg/fanal/types"
)

func newMemoryCache() *memoryCache {
	return &memoryCache{}
}

type memoryCache struct {
	blobInfo     *types.BlobInfo
	blobID       string
	artifactInfo *types.ArtifactInfo
	artifactID   string
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

func (c *memoryCache) PutArtifact(artifactID string, artifactInfo types.ArtifactInfo) (err error) {
	c.artifactInfo = &artifactInfo
	c.artifactID = artifactID
	return nil
}

func (c *memoryCache) PutBlob(blobID string, blobInfo types.BlobInfo) (err error) {
	c.blobInfo = &blobInfo
	c.blobID = blobID
	return nil
}

func (c *memoryCache) DeleteBlobs(blobIDs []string) error {
	if c.blobInfo != nil {
		for _, blobID := range blobIDs {
			if blobID == c.blobID {
				c.blobInfo = nil
			}
		}
	}
	return nil
}

func (c *memoryCache) GetArtifact(artifactID string) (artifactInfo types.ArtifactInfo, err error) {
	if c.artifactInfo != nil && c.artifactID == artifactID {
		return *c.artifactInfo, nil
	}
	return types.ArtifactInfo{}, nil
}

func (c *memoryCache) GetBlob(blobID string) (blobInfo types.BlobInfo, err error) {
	if c.blobInfo != nil && c.blobID == blobID {
		return *c.blobInfo, nil
	}
	return types.BlobInfo{}, errors.New("not found")
}

func (c *memoryCache) Close() (err error) {
	c.artifactInfo = nil
	c.blobInfo = nil
	return nil
}

func (c *memoryCache) Clear() (err error) {
	return c.Close()
}

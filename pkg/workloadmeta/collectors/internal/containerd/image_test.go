// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKnownImages(t *testing.T) {
	images := newKnownImages()

	imageID := "sha256:92ca512c4cbb8a0909f903979610d5ee300ee26ce2898040c99606d1de46fce1"
	repoTag := "gcr.io/datadoghq/agent:7.42.0"
	repoDigest := "gcr.io/datadoghq/agent@sha256:3a19076bfee70900a600b8e3ee2cc30d5101d1d3d2b33654f1a316e596eaa4e0"

	// Add a reference. The image name is an ID.
	images.addReference(imageID, imageID)
	gotID, found := images.getImageID(imageID)
	assert.True(t, found)
	assert.Equal(t, imageID, gotID)
	assert.Empty(t, images.getRepoTags(imageID))
	assert.Empty(t, images.getRepoDigests(imageID))
	assert.NotEmpty(t, images.getAReference(imageID))

	// Add another reference to the same image. The name is a repo tag.
	images.addReference(repoTag, imageID)
	gotID, found = images.getImageID(repoTag)
	assert.True(t, found)
	assert.Equal(t, imageID, gotID)
	assert.Equal(t, []string{repoTag}, images.getRepoTags(imageID))
	assert.Empty(t, images.getRepoDigests(imageID))
	assert.NotEmpty(t, images.getAReference(imageID))

	// Add another reference to the same image. The name is a repo digest.
	images.addReference(repoDigest, imageID)
	gotID, found = images.getImageID(repoDigest)
	assert.True(t, found)
	assert.Equal(t, imageID, gotID)
	assert.Equal(t, []string{repoTag}, images.getRepoTags(imageID))
	assert.Equal(t, []string{repoDigest}, images.getRepoDigests(imageID))
	assert.NotEmpty(t, images.getAReference(imageID))

	// Delete the reference that is an ID.
	images.deleteReference(imageID, imageID)
	_, found = images.getImageID(imageID)
	assert.False(t, found)
	assert.Equal(t, []string{repoTag}, images.getRepoTags(imageID))
	assert.Equal(t, []string{repoDigest}, images.getRepoDigests(imageID))
	assert.NotEmpty(t, images.getAReference(imageID))

	// Delete the reference that is a repo tag.
	images.deleteReference(repoTag, imageID)
	_, found = images.getImageID(repoTag)
	assert.False(t, found)
	assert.Empty(t, images.getRepoTags(imageID))
	assert.Equal(t, []string{repoDigest}, images.getRepoDigests(imageID))
	assert.NotEmpty(t, images.getAReference(imageID))

	// Delete the reference that is a repo digest.
	images.deleteReference(repoDigest, imageID)
	_, found = images.getImageID(repoDigest)
	assert.False(t, found)
	assert.Empty(t, images.getRepoTags(imageID))
	assert.Empty(t, images.getRepoDigests(imageID))
	assert.Empty(t, images.getAReference(imageID))
}

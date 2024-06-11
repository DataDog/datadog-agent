// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"testing"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
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

func TestGetLayersWithHistory(t *testing.T) {
	// Create a new image
	imageConfig := ocispec.Image{
		History: []ocispec.History{
			{
				EmptyLayer: false,
				Comment:    "not-empty1",
			},
			{
				EmptyLayer: true,
				Comment:    "empty1",
			},
			{
				EmptyLayer: true,
				Comment:    "empty2",
			},
			{
				EmptyLayer: false,
				Comment:    "not-empty2",
			},
			{
				EmptyLayer: true,
				Comment:    "empty3",
			},
			{
				EmptyLayer: false,
				Comment:    "not-empty3",
			},
			{
				EmptyLayer: true,
				Comment:    "empty4",
			},
			{
				EmptyLayer: true,
				Comment:    "empty5",
			},
		},
	}

	manifest := ocispec.Manifest{
		Layers: []ocispec.Descriptor{
			{
				MediaType: ocispec.MediaTypeImageLayer,
				Digest:    digest.FromString("foo"),
				Size:      1,
			},
			{
				MediaType: ocispec.MediaTypeImageLayer,
				Digest:    digest.FromString("bar"),
				Size:      2,
			},
			{
				MediaType: ocispec.MediaTypeImageLayer,
				Digest:    digest.FromString("baz"),
				Size:      3,
			},
			{
				MediaType: ocispec.MediaTypeImageLayer,
				Digest:    digest.FromString("bow"),
				Size:      4,
			},
		},
	}

	layers := getLayersWithHistory(imageConfig, manifest)
	assert.Equal(t, []workloadmeta.ContainerImageLayer{
		{
			MediaType: ocispec.MediaTypeImageLayer,
			Digest:    digest.FromString("foo").String(),
			SizeBytes: 1,
			History: &ocispec.History{
				Comment: "not-empty1",
			},
		},
		{
			MediaType: ocispec.MediaTypeImageLayer,
			Digest:    digest.FromString("bar").String(),
			SizeBytes: 2,
			History: &ocispec.History{
				Comment: "not-empty2",
			},
		},
		{
			MediaType: ocispec.MediaTypeImageLayer,
			Digest:    digest.FromString("baz").String(),
			SizeBytes: 3,
			History: &ocispec.History{
				Comment: "not-empty3",
			},
		},
		{
			MediaType: ocispec.MediaTypeImageLayer,
			Digest:    digest.FromString("bow").String(),
			SizeBytes: 4,
		},
	}, layers)
}

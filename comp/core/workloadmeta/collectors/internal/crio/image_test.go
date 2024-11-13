// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build crio

package crio

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// TestGenerateImageEventFromContainer tests that a valid image event is generated from container image data.
func TestGenerateImageEventFromContainer(t *testing.T) {
	client := &mockCRIOClient{
		mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec, verbose bool) (*v1.ImageStatusResponse, error) {
			return &v1.ImageStatusResponse{
				Image: &v1.Image{
					Id:          "image123",
					RepoTags:    []string{"myrepo/myimage:latest"},
					RepoDigests: []string{"myrepo/myimage@sha256:123abc"},
					Size_:       123456789,
				},
				Info: map[string]string{
					"info": `{
						"labels": {"label1": "value1", "label2": "value2"},
						"imageSpec": {
							"os": "linux",
							"architecture": "amd64",
							"variant": "v8",
							"rootfs": {
								"diff_ids": [
									"sha256:layer1digest",
									"sha256:layer2digest"
								]
							},
							"history": [
								{
									"created": "2023-01-01T00:00:00Z",
									"created_by": "command1",
									"author": "author1",
									"comment": "Layer 1 comment",
									"empty_layer": false
								},
								{
									"created": "2023-01-02T00:00:00Z",
									"created_by": "command2",
									"author": "author2",
									"comment": "Layer 2 comment",
									"empty_layer": false
								}
							]
						}
					}`,
				},
			}, nil
		},
	}

	store := &mockWorkloadmetaStore{}
	crioCollector := collector{
		client: client,
		store:  store,
	}

	// Mock container data
	container := &v1.Container{
		Id:           "container1",
		Image:        &v1.ImageSpec{Image: "myrepo/myimage:latest"},
		PodSandboxId: "pod1",
	}

	// Generate the image event
	event := crioCollector.generateImageEventFromContainer(context.Background(), container)
	image := event.Entity.(*workloadmeta.ContainerImageMetadata)

	// Assert basic fields
	assert.Equal(t, "image123", image.EntityID.ID)
	assert.Equal(t, "myrepo/myimage:latest", image.EntityMeta.Name)
	assert.Equal(t, "myrepo/myimage@sha256:123abc", image.RepoDigests[0])
	assert.Equal(t, int64(123456789), image.SizeBytes)

	// Assert parsed metadata
	assert.Equal(t, "linux", image.OS)
	assert.Equal(t, "amd64", image.Architecture)
	assert.Equal(t, "v8", image.Variant)
	assert.Equal(t, map[string]string{"label1": "value1", "label2": "value2"}, image.Labels)

	// Assert layers and their metadata
	assert.Len(t, image.Layers, 2)
	assert.Equal(t, "sha256:layer1digest", image.Layers[0].Digest)
	assert.Equal(t, "sha256:layer2digest", image.Layers[1].Digest)

	// Assert layer history for each layer
	assert.NotNil(t, image.Layers[0].History)
	assert.Equal(t, "command1", image.Layers[0].History.CreatedBy)
	assert.Equal(t, "author1", image.Layers[0].History.Author)
	assert.Equal(t, "Layer 1 comment", image.Layers[0].History.Comment)

	assert.NotNil(t, image.Layers[1].History)
	assert.Equal(t, "command2", image.Layers[1].History.CreatedBy)
	assert.Equal(t, "author2", image.Layers[1].History.Author)
	assert.Equal(t, "Layer 2 comment", image.Layers[1].History.Comment)
}

// TestGenerateImageEventLayerInfoError verifies behavior when layer metadata is missing.
func TestGenerateImageEventLayerInfoError(t *testing.T) {
	client := &mockCRIOClient{
		mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec, verbose bool) (*v1.ImageStatusResponse, error) {
			return &v1.ImageStatusResponse{
				Image: &v1.Image{
					Id:          "image123",
					RepoTags:    []string{"repo/image:tag"},
					RepoDigests: []string{"repo/image@sha256:dummyhash"},
					Size_:       123456789,
				},
			}, nil
		},
	}

	store := &mockWorkloadmetaStore{}
	crioCollector := collector{
		client: client,
		store:  store,
	}

	container := &v1.Container{
		Id:           "container1",
		Image:        &v1.ImageSpec{Image: "repo/image:tag"},
		PodSandboxId: "pod1",
	}

	event := crioCollector.generateImageEventFromContainer(context.Background(), container)
	image := event.Entity.(*workloadmeta.ContainerImageMetadata)

	assert.Equal(t, "repo/image:tag", image.EntityMeta.Name)
	assert.Empty(t, image.Layers)
}

// TestGenerateImageEventNoRepoTags tests behavior when image has no repository tags.
func TestGenerateImageEventNoRepoTags(t *testing.T) {
	client := &mockCRIOClient{
		mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec, verbose bool) (*v1.ImageStatusResponse, error) {
			return &v1.ImageStatusResponse{
				Image: &v1.Image{
					Id: "image123",
				},
			}, nil
		},
	}

	store := &mockWorkloadmetaStore{}
	crioCollector := collector{
		client: client,
		store:  store,
	}

	container := &v1.Container{
		Id:           "container1",
		Image:        &v1.ImageSpec{Image: "repo/image:tag"},
		PodSandboxId: "pod1",
	}

	event := crioCollector.generateImageEventFromContainer(context.Background(), container)
	image := event.Entity.(*workloadmeta.ContainerImageMetadata)

	assert.Empty(t, image.RepoTags)
	assert.Equal(t, "image123", image.EntityID.ID)
}

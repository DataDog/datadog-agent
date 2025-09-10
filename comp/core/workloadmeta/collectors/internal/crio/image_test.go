// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build crio

package crio

import (
	"context"
	"errors"
	"testing"
	"time"

	imgspecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestParseDigests(t *testing.T) {
	tests := []struct {
		name        string
		imageRefs   []string
		expected    string
		expectError bool
	}{
		{
			name:        "Valid digest",
			imageRefs:   []string{"myrepo/myimage@sha256:abc123def456"},
			expected:    "sha256:abc123def456",
			expectError: false,
		},
		{
			name:        "Multiple digests - uses first",
			imageRefs:   []string{"myrepo/myimage@sha256:abc123", "myrepo/myimage@sha256:def456"},
			expected:    "sha256:abc123",
			expectError: false,
		},
		{
			name:        "Empty digest list",
			imageRefs:   []string{},
			expected:    "",
			expectError: true,
		},
		{
			name:        "No @ symbol",
			imageRefs:   []string{"myrepo/myimage:latest"},
			expected:    "",
			expectError: true,
		},
		{
			name:        "@ symbol but no digest",
			imageRefs:   []string{"myrepo/myimage@"},
			expected:    "", // Returns empty string, not an error
			expectError: false,
		},
		{
			name:        "Complex repo with digest",
			imageRefs:   []string{"registry.example.com/org/myrepo/myimage@sha256:1234567890abcdef"},
			expected:    "sha256:1234567890abcdef",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseDigests(tt.imageRefs)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestConvertImageToEvent(t *testing.T) {
	tests := []struct {
		name      string
		image     *v1.Image
		info      map[string]string
		namespace string
		expected  *workloadmeta.CollectorEvent
	}{
		{
			name: "Complete image with digest",
			image: &v1.Image{
				Id:          "image123",
				RepoTags:    []string{"myrepo/myimage:latest"},
				RepoDigests: []string{"myrepo/myimage@sha256:abc123"},
				Spec: &v1.ImageSpec{
					Annotations: map[string]string{
						"annotation1": "value1",
					},
				},
			},
			info: map[string]string{
				"info": `{
					"labels": {"label1": "value1"},
					"imageSpec": {
						"os": "linux",
						"architecture": "amd64",
						"variant": "v1",
						"rootfs": {
							"diff_ids": ["sha256:layer1", "sha256:layer2"]
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
			namespace: "default",
			expected: &workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceRuntime,
				Entity: &workloadmeta.ContainerImageMetadata{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainerImageMetadata,
						ID:   "sha256:abc123", // Should use digest, not raw ID
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "myrepo/myimage:latest",
						Namespace: "default",
						Annotations: map[string]string{
							"annotation1": "value1",
						},
						Labels: map[string]string{
							"label1": "value1",
						},
					},
					RepoTags:     []string{"myrepo/myimage:latest"},
					RepoDigests:  []string{"myrepo/myimage@sha256:abc123"},
					OS:           "linux",
					Architecture: "amd64",
					Variant:      "v1",
					Layers: []workloadmeta.ContainerImageLayer{
						{
							Digest: "sha256:layer1",
							History: &imgspecs.History{
								Created:    parseTime("2023-01-01T00:00:00Z"),
								CreatedBy:  "command1",
								Author:     "author1",
								Comment:    "Layer 1 comment",
								EmptyLayer: false,
							},
						},
						{
							Digest: "sha256:layer2",
							History: &imgspecs.History{
								Created:    parseTime("2023-01-02T00:00:00Z"),
								CreatedBy:  "command2",
								Author:     "author2",
								Comment:    "Layer 2 comment",
								EmptyLayer: false,
							},
						},
					},
				},
			},
		},
		{
			name: "Image without digest falls back to raw ID",
			image: &v1.Image{
				Id:          "image123",
				RepoTags:    []string{"myrepo/myimage:latest"},
				RepoDigests: []string{}, // No digest available
			},
			info:      map[string]string{},
			namespace: "default",
			expected: &workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceRuntime,
				Entity: &workloadmeta.ContainerImageMetadata{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainerImageMetadata,
						ID:   "image123", // Should use raw ID when no digest
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "myrepo/myimage:latest",
						Namespace: "default",
					},
					RepoTags:    []string{"myrepo/myimage:latest"},
					RepoDigests: []string{},
				},
			},
		},
		{
			name: "Image without repo tags",
			image: &v1.Image{
				Id:          "image123",
				RepoTags:    []string{},
				RepoDigests: []string{"myrepo/myimage@sha256:abc123"},
			},
			info:      map[string]string{},
			namespace: "default",
			expected: &workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceRuntime,
				Entity: &workloadmeta.ContainerImageMetadata{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainerImageMetadata,
						ID:   "sha256:abc123",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "", // No name when no repo tags
						Namespace: "default",
					},
					RepoTags:    []string{},
					RepoDigests: []string{"myrepo/myimage@sha256:abc123"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := &collector{}

			result := collector.convertImageToEvent(tt.image, tt.info, tt.namespace)

			assert.Equal(t, tt.expected.Type, result.Type)
			assert.Equal(t, tt.expected.Source, result.Source)

			expectedImg := tt.expected.Entity.(*workloadmeta.ContainerImageMetadata)
			actualImg := result.Entity.(*workloadmeta.ContainerImageMetadata)

			assert.Equal(t, expectedImg.EntityID, actualImg.EntityID)
			assert.Equal(t, expectedImg.EntityMeta.Name, actualImg.EntityMeta.Name)
			assert.Equal(t, expectedImg.EntityMeta.Namespace, actualImg.EntityMeta.Namespace)
			assert.Equal(t, expectedImg.EntityMeta.Annotations, actualImg.EntityMeta.Annotations)
			assert.Equal(t, expectedImg.EntityMeta.Labels, actualImg.EntityMeta.Labels)
			assert.Equal(t, expectedImg.RepoTags, actualImg.RepoTags)
			assert.Equal(t, expectedImg.RepoDigests, actualImg.RepoDigests)
			assert.Equal(t, expectedImg.OS, actualImg.OS)
			assert.Equal(t, expectedImg.Architecture, actualImg.Architecture)
			assert.Equal(t, expectedImg.Variant, actualImg.Variant)

			// Check layers
			require.Equal(t, len(expectedImg.Layers), len(actualImg.Layers))
			for i, expectedLayer := range expectedImg.Layers {
				actualLayer := actualImg.Layers[i]
				assert.Equal(t, expectedLayer.Digest, actualLayer.Digest)
				if expectedLayer.History != nil && actualLayer.History != nil {
					assert.Equal(t, expectedLayer.History.CreatedBy, actualLayer.History.CreatedBy)
					assert.Equal(t, expectedLayer.History.Author, actualLayer.History.Author)
					assert.Equal(t, expectedLayer.History.Comment, actualLayer.History.Comment)
					assert.Equal(t, expectedLayer.History.EmptyLayer, actualLayer.History.EmptyLayer)
				}
			}
		})
	}
}

func TestGenerateImageEventsFromImageList(t *testing.T) {
	tests := []struct {
		name                  string
		mockListImages        func(ctx context.Context) ([]*v1.Image, error)
		mockGetContainerImage func(ctx context.Context, imageSpec *v1.ImageSpec, verbose bool) (*v1.ImageStatusResponse, error)
		existingImages        map[string]*workloadmeta.ContainerImageMetadata
		expectedEvents        int
		expectedError         bool
	}{
		{
			name: "All images are new - should create events for all",
			mockListImages: func(_ context.Context) ([]*v1.Image, error) {
				return []*v1.Image{
					{
						Id:          "image1",
						RepoTags:    []string{"repo/image1:latest"},
						RepoDigests: []string{"repo/image1@sha256:hash1"},
					},
					{
						Id:          "image2",
						RepoTags:    []string{"repo/image2:latest"},
						RepoDigests: []string{"repo/image2@sha256:hash2"},
					},
				}, nil
			},
			mockGetContainerImage: func(_ context.Context, imageSpec *v1.ImageSpec, _ bool) (*v1.ImageStatusResponse, error) {
				return &v1.ImageStatusResponse{
					Image: &v1.Image{
						Id:          imageSpec.Image,
						RepoTags:    []string{"repo/image:latest"},
						RepoDigests: []string{"repo/image@sha256:hash"},
					},
				}, nil
			},
			existingImages: map[string]*workloadmeta.ContainerImageMetadata{},
			expectedEvents: 2,
			expectedError:  false,
		},
		{
			name: "Some images exist - should skip existing ones",
			mockListImages: func(_ context.Context) ([]*v1.Image, error) {
				return []*v1.Image{
					{
						Id:          "image1",
						RepoTags:    []string{"repo/image1:latest"},
						RepoDigests: []string{"repo/image1@sha256:hash1"},
					},
					{
						Id:          "image2",
						RepoTags:    []string{"repo/image2:latest"},
						RepoDigests: []string{"repo/image2@sha256:hash2"},
					},
				}, nil
			},
			mockGetContainerImage: func(_ context.Context, imageSpec *v1.ImageSpec, _ bool) (*v1.ImageStatusResponse, error) {
				return &v1.ImageStatusResponse{
					Image: &v1.Image{
						Id:          imageSpec.Image,
						RepoTags:    []string{"repo/image:latest"},
						RepoDigests: []string{"repo/image@sha256:hash"},
					},
				}, nil
			},
			existingImages: map[string]*workloadmeta.ContainerImageMetadata{
				"sha256:hash1": {
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: "sha256:hash1"},
				},
			},
			expectedEvents: 1, // Only image2 creates an event (image1 is skipped entirely)
			expectedError:  false,
		},
		{
			name: "ListImages fails",
			mockListImages: func(_ context.Context) ([]*v1.Image, error) {
				return nil, errors.New("failed to list images")
			},
			mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec, _ bool) (*v1.ImageStatusResponse, error) {
				return nil, errors.New("should not be called")
			},
			existingImages: map[string]*workloadmeta.ContainerImageMetadata{},
			expectedEvents: 0,
			expectedError:  true,
		},
		{
			name: "GetContainerImage fails for some images",
			mockListImages: func(_ context.Context) ([]*v1.Image, error) {
				return []*v1.Image{
					{
						Id:          "image1",
						RepoTags:    []string{"repo/image1:latest"},
						RepoDigests: []string{"repo/image1@sha256:hash1"},
					},
					{
						Id:          "image2",
						RepoTags:    []string{"repo/image2:latest"},
						RepoDigests: []string{"repo/image2@sha256:hash2"},
					},
				}, nil
			},
			mockGetContainerImage: func(_ context.Context, imageSpec *v1.ImageSpec, _ bool) (*v1.ImageStatusResponse, error) {
				if imageSpec.Image == "image1" {
					return &v1.ImageStatusResponse{
						Image: &v1.Image{
							Id:          imageSpec.Image,
							RepoTags:    []string{"repo/image1:latest"},
							RepoDigests: []string{"repo/image1@sha256:hash1"},
						},
					}, nil
				}
				return nil, errors.New("failed to get image status")
			},
			existingImages: map[string]*workloadmeta.ContainerImageMetadata{},
			expectedEvents: 1, // Only image1 should succeed
			expectedError:  false,
		},
		{
			name: "ID format mismatch - image stored with digest ID but lookup uses raw ID",
			mockListImages: func(_ context.Context) ([]*v1.Image, error) {
				return []*v1.Image{
					{
						Id:          "rawid123",
						RepoTags:    []string{"repo/image1:latest"},
						RepoDigests: []string{"repo/image1@sha256:digestid456"},
					},
				}, nil
			},
			mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec, _ bool) (*v1.ImageStatusResponse, error) {
				return &v1.ImageStatusResponse{
					Image: &v1.Image{
						Id:          "rawid123",
						RepoTags:    []string{"repo/image1:latest"},
						RepoDigests: []string{"repo/image1@sha256:digestid456"},
					},
				}, nil
			},
			existingImages: map[string]*workloadmeta.ContainerImageMetadata{
				"sha256:digestid456": {
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: "sha256:digestid456"},
				},
			},
			expectedEvents: 0, // No event created - image exists and is skipped
			expectedError:  false,
		},
		{
			name: "ID format mismatch - image stored with raw ID but lookup tries digest first",
			mockListImages: func(_ context.Context) ([]*v1.Image, error) {
				return []*v1.Image{
					{
						Id:          "rawid123",
						RepoTags:    []string{"repo/image1:latest"},
						RepoDigests: []string{"repo/image1@sha256:digestid456"},
					},
				}, nil
			},
			mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec, _ bool) (*v1.ImageStatusResponse, error) {
				return &v1.ImageStatusResponse{
					Image: &v1.Image{
						Id:          "rawid123",
						RepoTags:    []string{"repo/image1:latest"},
						RepoDigests: []string{"repo/image1@sha256:digestid456"},
					},
				}, nil
			},
			existingImages: map[string]*workloadmeta.ContainerImageMetadata{
				"rawid123": {
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: "rawid123"},
				},
			},
			expectedEvents: 0, // No event created - image exists and is skipped
			expectedError:  false,
		},
		{
			name: "Dangling image with empty digests - stored with raw ID",
			mockListImages: func(_ context.Context) ([]*v1.Image, error) {
				return []*v1.Image{
					{
						Id:          "7f7fbb837cb1da28",
						RepoTags:    []string{}, // Empty tags (dangling image)
						RepoDigests: []string{"docker.io/frankspano569/agent@sha256:bf78d892b7739f7c"},
					},
				}, nil
			},
			mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec, _ bool) (*v1.ImageStatusResponse, error) {
				return &v1.ImageStatusResponse{
					Image: &v1.Image{
						Id:          "7f7fbb837cb1da28",
						RepoTags:    []string{}, // Empty tags in response
						RepoDigests: []string{}, // Empty digests in response (key difference)
					},
				}, nil
			},
			existingImages: map[string]*workloadmeta.ContainerImageMetadata{
				"7f7fbb837cb1da28": { // Image was stored with raw ID due to empty response digests
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: "7f7fbb837cb1da28"},
				},
			},
			expectedEvents: 0, // No event created - image exists and is skipped
			expectedError:  false,
		},
		{
			name: "Multiple ID format scenarios in same batch",
			mockListImages: func(_ context.Context) ([]*v1.Image, error) {
				return []*v1.Image{
					{
						Id:          "rawid1",
						RepoTags:    []string{"repo/image1:latest"},
						RepoDigests: []string{"repo/image1@sha256:digestid1"},
					},
					{
						Id:          "rawid2",
						RepoTags:    []string{}, // Dangling image
						RepoDigests: []string{"repo/image2@sha256:digestid2"},
					},
					{
						Id:          "rawid3",
						RepoTags:    []string{"repo/image3:latest"},
						RepoDigests: []string{"repo/image3@sha256:digestid3"},
					},
				}, nil
			},
			mockGetContainerImage: func(_ context.Context, imageSpec *v1.ImageSpec, _ bool) (*v1.ImageStatusResponse, error) {
				return &v1.ImageStatusResponse{
					Image: &v1.Image{
						Id:          imageSpec.Image,
						RepoTags:    []string{"repo/image:latest"},
						RepoDigests: []string{"repo/image@sha256:hash"},
					},
				}, nil
			},
			existingImages: map[string]*workloadmeta.ContainerImageMetadata{
				"sha256:digestid1": { // Image1 stored with digest ID
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: "sha256:digestid1"},
				},
				"rawid2": { // Image2 stored with raw ID (dangling)
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: "rawid2"},
				},
				// Image3 is new
			},
			expectedEvents: 1, // Only image3 creates an event (image1 & image2 are skipped)
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockCRIOClient{
				mockListImages:        tt.mockListImages,
				mockGetContainerImage: tt.mockGetContainerImage,
			}
			store := &mockWorkloadmetaStore{
				existingImages: tt.existingImages,
			}
			collector := &collector{
				client: client,
				store:  store,
			}

			events, imageIDs, err := collector.generateImageEventsFromImageList(context.Background())

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedEvents, len(events))
				// imageIDs should include all current images (events are only for new images, but imageIDs tracks all)
				assert.GreaterOrEqual(t, len(imageIDs), len(events), "imageIDs should include at least as many images as events")
			}
		})
	}
}

func TestGenerateUnsetImageEvent(t *testing.T) {
	entityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainerImageMetadata,
		ID:   "sha256:abc123",
	}

	event := generateUnsetImageEvent(entityID)

	assert.Equal(t, workloadmeta.EventTypeUnset, event.Type)
	assert.Equal(t, workloadmeta.SourceRuntime, event.Source)
	assert.Equal(t, entityID, event.Entity.GetID())
}

func TestParseImageInfo(t *testing.T) {
	tests := []struct {
		name     string
		info     map[string]string
		expected imageInfo
	}{
		{
			name: "Complete image info",
			info: map[string]string{
				"info": `{
					"labels": {"label1": "value1", "label2": "value2"},
					"imageSpec": {
						"os": "linux",
						"architecture": "amd64",
						"variant": "v1",
						"rootfs": {
							"diff_ids": ["sha256:layer1", "sha256:layer2"]
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
			expected: imageInfo{
				os:      "linux",
				arch:    "amd64",
				variant: "v1",
				labels: map[string]string{
					"label1": "value1",
					"label2": "value2",
				},
				layers: []workloadmeta.ContainerImageLayer{
					{
						Digest: "sha256:layer1",
						History: &imgspecs.History{
							Created:    parseTime("2023-01-01T00:00:00Z"),
							CreatedBy:  "command1",
							Author:     "author1",
							Comment:    "Layer 1 comment",
							EmptyLayer: false,
						},
					},
					{
						Digest: "sha256:layer2",
						History: &imgspecs.History{
							Created:    parseTime("2023-01-02T00:00:00Z"),
							CreatedBy:  "command2",
							Author:     "author2",
							Comment:    "Layer 2 comment",
							EmptyLayer: false,
						},
					},
				},
			},
		},
		{
			name: "Empty info",
			info: map[string]string{},
			expected: imageInfo{
				layers: []workloadmeta.ContainerImageLayer{},
			},
		},
		{
			name: "Invalid JSON",
			info: map[string]string{
				"info": "invalid json",
			},
			expected: imageInfo{
				layers: []workloadmeta.ContainerImageLayer{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseImageInfo(tt.info, "/tmp/test", "testimage")

			assert.Equal(t, tt.expected.os, result.os)
			assert.Equal(t, tt.expected.arch, result.arch)
			assert.Equal(t, tt.expected.variant, result.variant)
			assert.Equal(t, tt.expected.labels, result.labels)

			require.Equal(t, len(tt.expected.layers), len(result.layers))
			for i, expectedLayer := range tt.expected.layers {
				actualLayer := result.layers[i]
				assert.Equal(t, expectedLayer.Digest, actualLayer.Digest)
				if expectedLayer.History != nil && actualLayer.History != nil {
					assert.Equal(t, expectedLayer.History.CreatedBy, actualLayer.History.CreatedBy)
					assert.Equal(t, expectedLayer.History.Author, actualLayer.History.Author)
					assert.Equal(t, expectedLayer.History.Comment, actualLayer.History.Comment)
					assert.Equal(t, expectedLayer.History.EmptyLayer, actualLayer.History.EmptyLayer)
				}
			}
		})
	}
}

// Helper function to parse time for tests
func parseTime(timeStr string) *time.Time {
	t, _ := time.Parse(time.RFC3339, timeStr)
	return &t
}

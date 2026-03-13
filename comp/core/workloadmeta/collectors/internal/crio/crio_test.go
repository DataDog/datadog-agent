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
	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestPull(t *testing.T) {
	config := configmock.New(t)
	config.SetWithoutSource("container_image.enabled", false)

	createTime := time.Now().Add(-10 * time.Minute).UnixNano()
	startTime := time.Now().Add(-5 * time.Minute).UnixNano()
	finishTime := time.Now().UnixNano()

	tests := []struct {
		name                   string
		mockGetAllContainers   func(ctx context.Context) ([]*v1.Container, error)
		mockGetPodStatus       func(ctx context.Context, podID string) (*v1.PodSandboxStatus, error)
		mockGetContainerStatus func(ctx context.Context, containerID string) (*v1.ContainerStatusResponse, error)
		mockGetContainerImage  func(ctx context.Context, imageSpec *v1.ImageSpec, verbose bool) (*v1.ImageStatusResponse, error)
		expectedEvents         []workloadmeta.CollectorEvent
		expectedError          bool
	}{
		{
			name: "Valid container and image data",
			mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
				return []*v1.Container{
					{
						Id:           "container1",
						Image:        &v1.ImageSpec{Image: "image123"}, // container returns image ID here, container status returns image tag
						ImageRef:     "myrepo/myimage@sha256:123abc",
						PodSandboxId: "pod1",
						Metadata:     &v1.ContainerMetadata{Name: "container1"},
					},
				}, nil
			},
			mockGetPodStatus: func(_ context.Context, _ string) (*v1.PodSandboxStatus, error) {
				return &v1.PodSandboxStatus{Metadata: &v1.PodSandboxMetadata{Namespace: "default"}}, nil
			},
			mockGetContainerStatus: func(_ context.Context, _ string) (*v1.ContainerStatusResponse, error) {
				return &v1.ContainerStatusResponse{
					Status: &v1.ContainerStatus{
						Metadata:   &v1.ContainerMetadata{Name: "container1"},
						State:      v1.ContainerState_CONTAINER_RUNNING,
						CreatedAt:  createTime,
						StartedAt:  startTime,
						FinishedAt: finishTime,
						Image:      &v1.ImageSpec{Image: "myrepo/myimage:latest"},
						ImageRef:   "myrepo/myimage@sha256:123abc",
						ImageId:    "my_image_id",
						Resources: &v1.ContainerResources{
							Linux: &v1.LinuxContainerResources{
								CpuQuota:           50000,
								CpuPeriod:          100000,
								MemoryLimitInBytes: 104857600,
							},
						},
					},
				}, nil
			},
			mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec, _ bool) (*v1.ImageStatusResponse, error) {
				return &v1.ImageStatusResponse{
					Image: &v1.Image{
						Id:          "image123",
						RepoTags:    []string{"myrepo/myimage:latest"},
						RepoDigests: []string{"myrepo/myimage@sha256:123abc"},
					},
				}, nil
			},
			expectedEvents: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.Container{
						EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "container1"},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "container1",
							Namespace: "default",
						},
						Image: workloadmeta.ContainerImage{
							Name:       "myrepo/myimage",
							ShortName:  "myimage",
							RawName:    "myrepo/myimage:latest",
							ID:         "my_image_id",
							Tag:        "latest",
							RepoDigest: "sha256:123abc",
						},
						Resources: workloadmeta.ContainerResources{
							CPULimit:    pointer.Ptr(0.5),
							MemoryLimit: pointer.Ptr(uint64(104857600)),
						},
						Runtime: workloadmeta.ContainerRuntimeCRIO,
						State: workloadmeta.ContainerState{
							Status:     workloadmeta.ContainerStatusRunning,
							Running:    true,
							CreatedAt:  time.Unix(0, createTime).UTC(),
							StartedAt:  time.Unix(0, startTime).UTC(),
							FinishedAt: time.Unix(0, finishTime).UTC(),
							ExitCode:   pointer.Ptr(int64(0)),
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name: "Missing resources in container but available in Info",
			mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
				return []*v1.Container{
					{Id: "container1", PodSandboxId: "pod1"},
				}, nil
			},
			mockGetContainerStatus: func(_ context.Context, _ string) (*v1.ContainerStatusResponse, error) {
				return &v1.ContainerStatusResponse{
					Status: &v1.ContainerStatus{
						Metadata:   &v1.ContainerMetadata{Name: "container1"},
						State:      v1.ContainerState_CONTAINER_RUNNING,
						CreatedAt:  createTime,
						StartedAt:  startTime,
						FinishedAt: finishTime,
					},
					Info: map[string]string{
						"info": `{
							"pid": 12345,
							"runtimeSpec": {
								"hostname": "container-host",
								"linux": {
									"cgroupsPath": "/crio/crio-45e0df1c6e04fda693f5ef2654363c1ff5667bee7f8a9042ff5c629d48fbcbc4",
									"resources": {
										"cpu": {
											"quota": 50000,
											"period": 100000
										},
										"memory": {
											"memoryLimitInBytes": 104857600
										}
									}
								}
							}
						}`,
					},
				}, nil
			},
			expectedEvents: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.Container{
						EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "container1"},
						Runtime:  workloadmeta.ContainerRuntimeCRIO,
						State: workloadmeta.ContainerState{
							Status:     workloadmeta.ContainerStatusRunning,
							Running:    true,
							CreatedAt:  time.Unix(0, createTime).UTC(),
							StartedAt:  time.Unix(0, startTime).UTC(),
							FinishedAt: time.Unix(0, finishTime).UTC(),
							ExitCode:   pointer.Ptr(int64(0)),
						},
						Resources: workloadmeta.ContainerResources{
							CPULimit:    pointer.Ptr(0.5),
							MemoryLimit: pointer.Ptr(uint64(104857600)),
						},
						PID:        12345,
						Hostname:   "container-host",
						CgroupPath: "/crio/crio-45e0df1c6e04fda693f5ef2654363c1ff5667bee7f8a9042ff5c629d48fbcbc4",
					},
				},
			},
			expectedError: false,
		},
		{
			name: "Container with missing metadata",
			mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
				return []*v1.Container{
					{Id: "container1", PodSandboxId: "pod1", Metadata: nil},
				}, nil
			},
			mockGetContainerStatus: func(_ context.Context, _ string) (*v1.ContainerStatusResponse, error) {
				return &v1.ContainerStatusResponse{
					Status: &v1.ContainerStatus{
						State: v1.ContainerState_CONTAINER_RUNNING,
					},
				}, nil
			},
			expectedEvents: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.Container{
						EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "container1"},
						Runtime:  workloadmeta.ContainerRuntimeCRIO,
						State: workloadmeta.ContainerState{
							Running:    true,
							Status:     workloadmeta.ContainerStatusRunning,
							CreatedAt:  time.Unix(0, 0).UTC(),
							StartedAt:  time.Unix(0, 0).UTC(),
							FinishedAt: time.Unix(0, 0).UTC(),
							ExitCode:   pointer.Ptr(int64(0)),
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name: "Error retrieving container status",
			mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
				return []*v1.Container{
					{Id: "container1", PodSandboxId: "pod1"},
				}, nil
			},
			mockGetContainerStatus: func(_ context.Context, _ string) (*v1.ContainerStatusResponse, error) {
				return nil, errors.New("container status error")
			},
			expectedEvents: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.Container{
						EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "container1"},
						Runtime:  workloadmeta.ContainerRuntimeCRIO,
						State: workloadmeta.ContainerState{
							Status: workloadmeta.ContainerStatusUnknown,
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name: "No containers returned",
			mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
				return []*v1.Container{}, nil
			},
			expectedEvents: nil,
			expectedError:  false,
		},
		{
			name: "Error retrieving containers",
			mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
				return nil, errors.New("failed to retrieve containers")
			},
			expectedEvents: nil,
			expectedError:  true,
		},
		{
			name: "All resource limits are zero",
			mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
				return []*v1.Container{
					{
						Id:           "container1",
						Image:        &v1.ImageSpec{Image: "image123"}, // container returns image ID here, container status returns image tag
						ImageRef:     "myrepo/myimage@sha256:123abc",
						PodSandboxId: "pod1",
						Metadata:     &v1.ContainerMetadata{Name: "container1"},
					},
				}, nil
			},
			mockGetPodStatus: func(_ context.Context, _ string) (*v1.PodSandboxStatus, error) {
				return &v1.PodSandboxStatus{Metadata: &v1.PodSandboxMetadata{Namespace: "default"}}, nil
			},
			mockGetContainerStatus: func(_ context.Context, _ string) (*v1.ContainerStatusResponse, error) {
				return &v1.ContainerStatusResponse{
					Status: &v1.ContainerStatus{
						Metadata:   &v1.ContainerMetadata{Name: "container1"},
						State:      v1.ContainerState_CONTAINER_RUNNING,
						CreatedAt:  createTime,
						StartedAt:  startTime,
						FinishedAt: finishTime,
						Image:      &v1.ImageSpec{Image: "myrepo/myimage:latest"},
						ImageRef:   "myrepo/myimage@sha256:123abc",
						ImageId:    "my_image_id",
						Resources: &v1.ContainerResources{
							Linux: &v1.LinuxContainerResources{
								CpuQuota:           0,
								CpuPeriod:          0,
								MemoryLimitInBytes: 0,
							},
						},
					},
				}, nil
			},
			expectedEvents: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: &workloadmeta.Container{
						EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "container1"},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "container1",
							Namespace: "default",
						},
						Image: workloadmeta.ContainerImage{
							Name:       "myrepo/myimage",
							ShortName:  "myimage",
							RawName:    "myrepo/myimage:latest",
							ID:         "my_image_id",
							Tag:        "latest",
							RepoDigest: "sha256:123abc",
						},
						Resources: workloadmeta.ContainerResources{
							CPULimit:    nil, // No CPU limit
							MemoryLimit: nil, // No memory limit
						},
						Runtime: workloadmeta.ContainerRuntimeCRIO,
						State: workloadmeta.ContainerState{
							Status:     workloadmeta.ContainerStatusRunning,
							Running:    true,
							CreatedAt:  time.Unix(0, createTime).UTC(),
							StartedAt:  time.Unix(0, startTime).UTC(),
							FinishedAt: time.Unix(0, finishTime).UTC(),
							ExitCode:   pointer.Ptr(int64(0)),
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name: "Error retrieving container",
			mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
				return nil, errors.New("failed to retrieve containers")
			},
			expectedEvents: nil,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockCRIOClient{
				mockGetAllContainers:   tt.mockGetAllContainers,
				mockGetPodStatus:       tt.mockGetPodStatus,
				mockGetContainerStatus: tt.mockGetContainerStatus,
				mockGetContainerImage:  tt.mockGetContainerImage,
			}

			store := &mockWorkloadmetaStore{}
			crioCollector := collector{
				client: client,
				store:  store,
			}

			err := crioCollector.Pull(context.Background())

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedEvents, store.notifiedEvents)
		})
	}
}

func TestGenerateImageEventFromContainer(t *testing.T) {
	time1, _ := time.Parse(time.RFC3339, "2023-01-01T00:00:00Z")
	time2, _ := time.Parse(time.RFC3339, "2023-01-02T00:00:00Z")
	tests := []struct {
		name                string
		mockGetContainerImg func(context.Context, *v1.ImageSpec, bool) (*v1.ImageStatusResponse, error)
		container           *v1.Container
		expectedEvent       *workloadmeta.CollectorEvent
		expectError         bool
	}{
		{
			name: "Valid image metadata with history and layers",
			mockGetContainerImg: func(_ context.Context, _ *v1.ImageSpec, _ bool) (*v1.ImageStatusResponse, error) {
				return &v1.ImageStatusResponse{
					Image: &v1.Image{
						Id:          "image123",
						RepoTags:    []string{"myrepo/myimage:latest"},
						RepoDigests: []string{"myrepo/myimage@sha256:123abc"},
					},
					Info: map[string]string{
						"info": `{
							"labels": {"label1": "value1", "label2": "value2"},
							"imageSpec": {
								"os": "linux",
								"architecture": "amd64",
								"variant": "v8",
								"rootfs": {
									"diff_ids": ["sha256:layer1digest", "sha256:layer2digest"]
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
			container: &v1.Container{
				Id:           "container1",
				Image:        &v1.ImageSpec{Image: "myrepo/myimage:latest"},
				PodSandboxId: "pod1",
			},
			expectedEvent: &workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceRuntime,
				Entity: &workloadmeta.ContainerImageMetadata{
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: "sha256:123abc"},
					EntityMeta: workloadmeta.EntityMeta{
						Name:   "myrepo/myimage:latest",
						Labels: map[string]string{"label1": "value1", "label2": "value2"},
					},
					RepoTags:     []string{"myrepo/myimage:latest"},
					RepoDigests:  []string{"myrepo/myimage@sha256:123abc"},
					OS:           "linux",
					Architecture: "amd64",
					Variant:      "v8",
					Layers: []workloadmeta.ContainerImageLayer{
						{
							Digest:    "sha256:layer1digest",
							History:   &imgspecs.History{Created: &time1, CreatedBy: "command1", Author: "author1", Comment: "Layer 1 comment"},
							SizeBytes: 0,
						},
						{
							Digest:    "sha256:layer2digest",
							History:   &imgspecs.History{Created: &time2, CreatedBy: "command2", Author: "author2", Comment: "Layer 2 comment"},
							SizeBytes: 0,
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Image has no repo tags or digest",
			mockGetContainerImg: func(_ context.Context, _ *v1.ImageSpec, _ bool) (*v1.ImageStatusResponse, error) {
				return &v1.ImageStatusResponse{
					Image: &v1.Image{
						Id: "image123",
					},
				}, nil
			},
			container: &v1.Container{
				Id:           "container1",
				Image:        &v1.ImageSpec{Image: "repo/image:tag"},
				PodSandboxId: "pod1",
			},
			expectedEvent: &workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceRuntime,
				Entity: &workloadmeta.ContainerImageMetadata{
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: "image123"},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "",
					},
					RepoTags:    nil,
					RepoDigests: nil,
				},
			},
			expectError: false,
		},
		{
			name: "Error retrieving image metadata",
			mockGetContainerImg: func(_ context.Context, _ *v1.ImageSpec, _ bool) (*v1.ImageStatusResponse, error) {
				return nil, errors.New("failed to retrieve image metadata")
			},
			container: &v1.Container{
				Id:           "container1",
				Image:        &v1.ImageSpec{Image: "repo/image:tag"},
				PodSandboxId: "pod1",
			},
			expectedEvent: nil,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockCRIOClient{
				mockGetContainerImage: tt.mockGetContainerImg,
			}
			store := &mockWorkloadmetaStore{}
			crioCollector := collector{
				client: client,
				store:  store,
			}

			event, err := crioCollector.generateImageEventFromContainer(context.Background(), tt.container)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedEvent, event)
			}
		})
	}
}

func TestOptimizedImageCollection(t *testing.T) {
	config := configmock.New(t)
	config.SetWithoutSource("container_image.enabled", true)

	tests := []struct {
		name                     string
		mockListImages           func(ctx context.Context) ([]*v1.Image, error)
		mockGetContainerImage    func(ctx context.Context, imageSpec *v1.ImageSpec, verbose bool) (*v1.ImageStatusResponse, error)
		existingImages           map[string]*workloadmeta.ContainerImageMetadata
		expectedImageStatusCalls int
		expectedEventCount       int
		expectError              bool
	}{
		{
			name: "Skip existing images optimization",
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
			mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec, _ bool) (*v1.ImageStatusResponse, error) {
				return &v1.ImageStatusResponse{
					Image: &v1.Image{
						Id:          "image2",
						RepoTags:    []string{"repo/image2:latest"},
						RepoDigests: []string{"repo/image2@sha256:hash2"},
					},
				}, nil
			},
			existingImages: map[string]*workloadmeta.ContainerImageMetadata{
				"sha256:hash1": {
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: "sha256:hash1"},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "repo/image1:latest",
					},
				},
			},
			expectedImageStatusCalls: 1, // Only image2 should trigger GetContainerImage
			expectedEventCount:       1, // Only image2 generates a new event (image1 is skipped entirely)
			expectError:              false,
		},
		{
			name: "Handle mutable tags correctly",
			mockListImages: func(_ context.Context) ([]*v1.Image, error) {
				return []*v1.Image{
					{
						Id:          "image1",
						RepoTags:    []string{"repo/image:latest"},
						RepoDigests: []string{"repo/image@sha256:newhash"},
					},
				}, nil
			},
			mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec, _ bool) (*v1.ImageStatusResponse, error) {
				return &v1.ImageStatusResponse{
					Image: &v1.Image{
						Id:          "image1",
						RepoTags:    []string{"repo/image:latest"},
						RepoDigests: []string{"repo/image@sha256:newhash"},
					},
				}, nil
			},
			existingImages: map[string]*workloadmeta.ContainerImageMetadata{
				"sha256:oldhash": {
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: "sha256:oldhash"},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "repo/image:latest",
					},
				},
			},
			expectedImageStatusCalls: 1, // New hash should trigger GetContainerImage
			expectedEventCount:       1, // New hash should generate an event
			expectError:              false,
		},
		{
			name: "Handle ListImages error gracefully",
			mockListImages: func(_ context.Context) ([]*v1.Image, error) {
				return nil, errors.New("failed to list images")
			},
			mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec, _ bool) (*v1.ImageStatusResponse, error) {
				return &v1.ImageStatusResponse{
					Image: &v1.Image{
						Id:          "image1",
						RepoTags:    []string{"repo/image:latest"},
						RepoDigests: []string{"repo/image@sha256:hash1"},
					},
				}, nil
			},
			existingImages:           nil,
			expectedImageStatusCalls: 0,
			expectedEventCount:       0,
			expectError:              true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageStatusCallCount := 0
			client := &mockCRIOClient{
				mockListImages: tt.mockListImages,
				mockGetContainerImage: func(ctx context.Context, imageSpec *v1.ImageSpec, verbose bool) (*v1.ImageStatusResponse, error) {
					imageStatusCallCount++
					return tt.mockGetContainerImage(ctx, imageSpec, verbose)
				},
			}
			store := &mockWorkloadmetaStore{
				existingImages: tt.existingImages,
			}
			crioCollector := collector{
				client: client,
				store:  store,
			}

			events, imageIDs, err := crioCollector.generateImageEventsFromImageList(context.Background())

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedEventCount, len(events))
				assert.Equal(t, tt.expectedImageStatusCalls, imageStatusCallCount, "Unexpected number of GetContainerImage calls")
				// imageIDs should include all current images (both new events + existing skipped)
				assert.GreaterOrEqual(t, len(imageIDs), len(events), "imageIDs should include at least as many images as events")
			}
		})
	}
}

func TestPullWithImageCollectionEnabled(t *testing.T) {
	config := configmock.New(t)
	// Enable image collection to test the optimized image collection path
	config.SetWithoutSource("container_image.enabled", true)

	createTime := time.Now().Add(-10 * time.Minute).UnixNano()
	startTime := time.Now().Add(-5 * time.Minute).UnixNano()

	tests := []struct {
		name                     string
		mockGetAllContainers     func(ctx context.Context) ([]*v1.Container, error)
		mockGetPodStatus         func(ctx context.Context, podID string) (*v1.PodSandboxStatus, error)
		mockGetContainerStatus   func(ctx context.Context, containerID string) (*v1.ContainerStatusResponse, error)
		mockListImages           func(ctx context.Context) ([]*v1.Image, error)
		mockGetContainerImage    func(ctx context.Context, imageSpec *v1.ImageSpec, verbose bool) (*v1.ImageStatusResponse, error)
		existingImages           map[string]*workloadmeta.ContainerImageMetadata
		expectedContainerEvents  int
		expectedImageEvents      int
		expectedImageStatusCalls int
		expectedError            bool
	}{
		{
			name: "Pull with image collection enabled - new images",
			mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
				return []*v1.Container{
					{
						Id:           "container1",
						Image:        &v1.ImageSpec{Image: "image123"},
						ImageRef:     "myrepo/myimage@sha256:123abc",
						PodSandboxId: "pod1",
						Metadata:     &v1.ContainerMetadata{Name: "container1"},
					},
				}, nil
			},
			mockGetPodStatus: func(_ context.Context, _ string) (*v1.PodSandboxStatus, error) {
				return &v1.PodSandboxStatus{Metadata: &v1.PodSandboxMetadata{Namespace: "default"}}, nil
			},
			mockGetContainerStatus: func(_ context.Context, _ string) (*v1.ContainerStatusResponse, error) {
				return &v1.ContainerStatusResponse{
					Status: &v1.ContainerStatus{
						Metadata:  &v1.ContainerMetadata{Name: "container1"},
						State:     v1.ContainerState_CONTAINER_RUNNING,
						CreatedAt: createTime,
						StartedAt: startTime,
						Image:     &v1.ImageSpec{Image: "myrepo/myimage:latest"},
						ImageRef:  "myrepo/myimage@sha256:123abc",
					},
				}, nil
			},
			mockListImages: func(_ context.Context) ([]*v1.Image, error) {
				return []*v1.Image{
					{
						Id:          "image123",
						RepoTags:    []string{"myrepo/myimage:latest"},
						RepoDigests: []string{"myrepo/myimage@sha256:123abc"},
					},
				}, nil
			},
			mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec, _ bool) (*v1.ImageStatusResponse, error) {
				return &v1.ImageStatusResponse{
					Image: &v1.Image{
						Id:          "image123",
						RepoTags:    []string{"myrepo/myimage:latest"},
						RepoDigests: []string{"myrepo/myimage@sha256:123abc"},
					},
					Info: map[string]string{
						"info": `{
							"labels": {"test": "label"},
							"imageSpec": {
								"os": "linux",
								"architecture": "amd64"
							}
						}`,
					},
				}, nil
			},
			existingImages:           nil, // No existing images
			expectedContainerEvents:  1,   // 1 container event
			expectedImageEvents:      1,   // 1 new image event
			expectedImageStatusCalls: 1,   // 1 GetContainerImage call for new image
			expectedError:            false,
		},
		{
			name: "Pull with image collection enabled - existing images skipped",
			mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
				return []*v1.Container{
					{
						Id:           "container1",
						Image:        &v1.ImageSpec{Image: "image123"},
						ImageRef:     "myrepo/myimage@sha256:123abc",
						PodSandboxId: "pod1",
						Metadata:     &v1.ContainerMetadata{Name: "container1"},
					},
				}, nil
			},
			mockGetPodStatus: func(_ context.Context, _ string) (*v1.PodSandboxStatus, error) {
				return &v1.PodSandboxStatus{Metadata: &v1.PodSandboxMetadata{Namespace: "default"}}, nil
			},
			mockGetContainerStatus: func(_ context.Context, _ string) (*v1.ContainerStatusResponse, error) {
				return &v1.ContainerStatusResponse{
					Status: &v1.ContainerStatus{
						Metadata:  &v1.ContainerMetadata{Name: "container1"},
						State:     v1.ContainerState_CONTAINER_RUNNING,
						CreatedAt: createTime,
						StartedAt: startTime,
						Image:     &v1.ImageSpec{Image: "myrepo/myimage:latest"},
						ImageRef:  "myrepo/myimage@sha256:123abc",
					},
				}, nil
			},
			mockListImages: func(_ context.Context) ([]*v1.Image, error) {
				return []*v1.Image{
					{
						Id:          "image123",
						RepoTags:    []string{"myrepo/myimage:latest"},
						RepoDigests: []string{"myrepo/myimage@sha256:123abc"},
					},
				}, nil
			},
			mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec, _ bool) (*v1.ImageStatusResponse, error) {
				// This should not be called since image already exists
				t.Errorf("GetContainerImage should not be called for existing images")
				return nil, errors.New("unexpected call")
			},
			existingImages: map[string]*workloadmeta.ContainerImageMetadata{
				"sha256:123abc": {
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: "sha256:123abc"},
					EntityMeta: workloadmeta.EntityMeta{
						Name: "myrepo/myimage:latest",
					},
				},
			},
			expectedContainerEvents:  1, // 1 container event
			expectedImageEvents:      0, // No image events - image exists and is skipped
			expectedImageStatusCalls: 0, // No GetContainerImage calls - image exists
			expectedError:            false,
		},
		{
			name: "Pull with image collection enabled - ListImages fails, should return error",
			mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
				return []*v1.Container{
					{
						Id:           "container1",
						Image:        &v1.ImageSpec{Image: "image123"},
						ImageRef:     "myrepo/myimage@sha256:123abc",
						PodSandboxId: "pod1",
						Metadata:     &v1.ContainerMetadata{Name: "container1"},
					},
				}, nil
			},
			mockGetPodStatus: func(_ context.Context, _ string) (*v1.PodSandboxStatus, error) {
				return &v1.PodSandboxStatus{Metadata: &v1.PodSandboxMetadata{Namespace: "default"}}, nil
			},
			mockGetContainerStatus: func(_ context.Context, _ string) (*v1.ContainerStatusResponse, error) {
				return &v1.ContainerStatusResponse{
					Status: &v1.ContainerStatus{
						Metadata:  &v1.ContainerMetadata{Name: "container1"},
						State:     v1.ContainerState_CONTAINER_RUNNING,
						CreatedAt: createTime,
						StartedAt: startTime,
						Image:     &v1.ImageSpec{Image: "myrepo/myimage:latest"},
						ImageRef:  "myrepo/myimage@sha256:123abc",
					},
				}, nil
			},
			mockListImages: func(_ context.Context) ([]*v1.Image, error) {
				return nil, errors.New("ListImages failed")
			},
			mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec, _ bool) (*v1.ImageStatusResponse, error) {
				return &v1.ImageStatusResponse{
					Image: &v1.Image{
						Id:          "image123",
						RepoTags:    []string{"myrepo/myimage:latest"},
						RepoDigests: []string{"myrepo/myimage@sha256:123abc"},
					},
				}, nil
			},
			existingImages:           nil,
			expectedContainerEvents:  0,    // No container events due to early error
			expectedImageEvents:      0,    // No image events due to ListImages failure
			expectedImageStatusCalls: 0,    // No GetContainerImage calls due to early error
			expectedError:            true, // Should return error when ListImages fails
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageStatusCallCount := 0
			client := &mockCRIOClient{
				mockGetAllContainers:   tt.mockGetAllContainers,
				mockGetPodStatus:       tt.mockGetPodStatus,
				mockGetContainerStatus: tt.mockGetContainerStatus,
				mockListImages:         tt.mockListImages,
				mockGetContainerImage: func(ctx context.Context, imageSpec *v1.ImageSpec, verbose bool) (*v1.ImageStatusResponse, error) {
					imageStatusCallCount++
					return tt.mockGetContainerImage(ctx, imageSpec, verbose)
				},
			}

			store := &mockWorkloadmetaStore{
				existingImages: tt.existingImages,
			}
			crioCollector := collector{
				client:     client,
				store:      store,
				seenImages: make(map[workloadmeta.EntityID]struct{}),
			}

			err := crioCollector.Pull(context.Background())

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Verify the expected number of events were notified
			// The store should receive both container and image events
			totalEvents := len(store.notifiedEvents)
			expectedTotalEvents := tt.expectedContainerEvents + tt.expectedImageEvents
			assert.Equal(t, expectedTotalEvents, totalEvents, "Total events should match expected container + image events")

			// Count container vs image events
			containerEventCount := 0
			imageEventCount := 0
			for _, event := range store.notifiedEvents {
				switch event.Entity.(type) {
				case *workloadmeta.Container:
					containerEventCount++
				case *workloadmeta.ContainerImageMetadata:
					imageEventCount++
				}
			}

			assert.Equal(t, tt.expectedContainerEvents, containerEventCount, "Container event count mismatch")
			assert.Equal(t, tt.expectedImageEvents, imageEventCount, "Image event count mismatch")
			assert.Equal(t, tt.expectedImageStatusCalls, imageStatusCallCount, "GetContainerImage call count mismatch")
		})
	}
}

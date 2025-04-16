// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build crio

package crio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	imgspecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestPull(t *testing.T) {

	const envVarName = "DD_CONTAINER_IMAGE_ENABLED"
	originalValue := os.Getenv(envVarName)
	defer os.Setenv(envVarName, originalValue)

	os.Setenv(envVarName, "false")

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
							ID:         "sha256:123abc",
							Tag:        "latest",
							RepoDigest: "myrepo/myimage@sha256:123abc",
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
							ID:         "sha256:123abc",
							Tag:        "latest",
							RepoDigest: "myrepo/myimage@sha256:123abc",
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
				return nil, fmt.Errorf("failed to retrieve image metadata")
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

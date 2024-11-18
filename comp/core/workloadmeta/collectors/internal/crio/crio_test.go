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

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// Helper functions to create pointer values for testing
func floatPtr(f float64) *float64 {
	return &f
}

func uintPtr(u uint64) *uint64 {
	return &u
}

// fakeWorkloadmetaStore is a mock implementation of the workloadmeta store.
type fakeWorkloadmetaStore struct {
	workloadmeta.Component
	notifiedEvents []workloadmeta.CollectorEvent
}

func (store *fakeWorkloadmetaStore) Notify(events []workloadmeta.CollectorEvent) {
	store.notifiedEvents = append(store.notifiedEvents, events...)
}

// fakeCRIOClient simulates the CRI-O client for testing purposes.
type fakeCRIOClient struct {
	mockGetAllContainers   func(ctx context.Context) ([]*v1.Container, error)
	mockGetContainerStatus func(ctx context.Context, containerID string) (*v1.ContainerStatusResponse, error)
	mockGetPodStatus       func(ctx context.Context, podID string) (*v1.PodSandboxStatus, error)
	mockGetContainerImage  func(ctx context.Context, imageSpec *v1.ImageSpec) (*v1.Image, error)
	mockRuntimeMetadata    func(ctx context.Context) (*v1.VersionResponse, error)
}

func (f *fakeCRIOClient) GetAllContainers(ctx context.Context) ([]*v1.Container, error) {
	if f.mockGetAllContainers != nil {
		return f.mockGetAllContainers(ctx)
	}
	return []*v1.Container{}, nil
}

func (f *fakeCRIOClient) GetContainerStatus(ctx context.Context, containerID string) (*v1.ContainerStatusResponse, error) {
	if f.mockGetContainerStatus != nil {
		return f.mockGetContainerStatus(ctx, containerID)
	}
	return &v1.ContainerStatusResponse{}, nil
}

func (f *fakeCRIOClient) GetPodStatus(ctx context.Context, podID string) (*v1.PodSandboxStatus, error) {
	if f.mockGetPodStatus != nil {
		return f.mockGetPodStatus(ctx, podID)
	}
	return &v1.PodSandboxStatus{}, nil
}

func (f *fakeCRIOClient) GetContainerImage(ctx context.Context, imageSpec *v1.ImageSpec) (*v1.Image, error) {
	if f.mockGetContainerImage != nil {
		return f.mockGetContainerImage(ctx, imageSpec)
	}
	return &v1.Image{}, nil
}

func (f *fakeCRIOClient) RuntimeMetadata(ctx context.Context) (*v1.VersionResponse, error) {
	if f.mockRuntimeMetadata != nil {
		return f.mockRuntimeMetadata(ctx)
	}
	return &v1.VersionResponse{RuntimeName: "cri-o", RuntimeVersion: "v1.30.0"}, nil
}

func (f *fakeCRIOClient) Close() error {
	return nil
}

// TestPull verifies that Pull populates container data correctly with PID, Hostname, and CgroupPath.
func TestPull(t *testing.T) {
	client := &fakeCRIOClient{
		mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
			return []*v1.Container{
				{Id: "container1", PodSandboxId: "pod1", Metadata: &v1.ContainerMetadata{Name: "container1"}, Image: &v1.ImageSpec{Image: "myrepo/myimage:latest"}},
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
					CreatedAt: time.Now().Add(-10 * time.Minute).UnixNano(),
					Resources: &v1.ContainerResources{
						Linux: &v1.LinuxContainerResources{
							CpuQuota:           50000,
							CpuPeriod:          100000,
							MemoryLimitInBytes: 104857600,
						},
					},
				},
				Info: map[string]string{
					"info": `{
						"pid": 12345,
						"runtimeSpec": {
							"hostname": "container-host",
							"linux": {
								"cgroupsPath": "/crio/crio-45e0df1c6e04fda693f5ef2654363c1ff5667bee7f8a9042ff5c629d48fbcbc4"
							}
						}
					}`,
				},
			}, nil
		},
		mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec) (*v1.Image, error) {
			return &v1.Image{
				Id:          "image123",
				RepoTags:    []string{"myrepo/myimage:latest"},
				RepoDigests: []string{"myrepo/myimage@sha256:123abc"},
			}, nil
		},
	}

	store := &fakeWorkloadmetaStore{}
	crioCollector := collector{
		client: client,
		store:  store,
	}

	err := crioCollector.Pull(context.Background())
	assert.NoError(t, err)
	assert.NotEmpty(t, store.notifiedEvents)
	event := store.notifiedEvents[0]
	container := event.Entity.(*workloadmeta.Container)

	assert.Equal(t, "container1", container.EntityMeta.Name)
	assert.Equal(t, "default", container.EntityMeta.Namespace)
	assert.Equal(t, "container1", container.EntityID.ID)
	assert.Equal(t, floatPtr(0.5), container.Resources.CPULimit)
	assert.Equal(t, uintPtr(104857600), container.Resources.MemoryLimit)
	assert.Equal(t, "myrepo/myimage:latest", container.Image.RawName)
	assert.Equal(t, 12345, container.PID)
	assert.Equal(t, "container-host", container.Hostname)
	assert.Equal(t, "/crio/crio-45e0df1c6e04fda693f5ef2654363c1ff5667bee7f8a9042ff5c629d48fbcbc4", container.CgroupPath)
}

// TestPullContainerStatusError verifies that Pull handles errors when retrieving container status.
func TestPullContainerStatusError(t *testing.T) {
	client := &fakeCRIOClient{
		mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
			return []*v1.Container{
				{Id: "container1", PodSandboxId: "pod1"},
			}, nil
		},
		mockGetContainerStatus: func(_ context.Context, _ string) (*v1.ContainerStatusResponse, error) {
			return nil, errors.New("container status error")
		},
	}

	store := &fakeWorkloadmetaStore{}
	crioCollector := collector{
		client: client,
		store:  store,
	}

	err := crioCollector.Pull(context.Background())
	assert.NoError(t, err)
	assert.Len(t, store.notifiedEvents, 1)
	event := store.notifiedEvents[0]
	container := event.Entity.(*workloadmeta.Container)

	assert.Equal(t, workloadmeta.ContainerStatusUnknown, container.State.Status)
	assert.Empty(t, container.Resources.CPULimit)
	assert.Empty(t, container.Resources.MemoryLimit)
	assert.Equal(t, 0, container.PID)         // Default PID
	assert.Equal(t, "", container.Hostname)   // Default Hostname
	assert.Equal(t, "", container.CgroupPath) // Default CgroupPath
}

// TestPullNoPodNamespace verifies that Pull handles cases with a missing pod namespace.
func TestPullNoPodNamespace(t *testing.T) {
	client := &fakeCRIOClient{
		mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
			return []*v1.Container{
				{Id: "container1", PodSandboxId: "nonexistent-pod"},
			}, nil
		},
		mockGetPodStatus: func(_ context.Context, _ string) (*v1.PodSandboxStatus, error) {
			return nil, errors.New("pod not found")
		},
		mockGetContainerStatus: func(_ context.Context, _ string) (*v1.ContainerStatusResponse, error) {
			return &v1.ContainerStatusResponse{
				Status: &v1.ContainerStatus{
					Metadata:  &v1.ContainerMetadata{Name: "container1"},
					State:     v1.ContainerState_CONTAINER_RUNNING,
					CreatedAt: time.Now().Add(-10 * time.Minute).UnixNano(),
				},
				Info: map[string]string{
					"info": `{
						"pid": 12345,
						"runtimeSpec": {
							"hostname": "container-host",
							"linux": {
								"cgroupsPath": "/crio/crio-45e0df1c6e04fda693f5ef2654363c1ff5667bee7f8a9042ff5c629d48fbcbc4"
							}
						}
					}`,
				},
			}, nil
		},
	}

	store := &fakeWorkloadmetaStore{}
	crioCollector := collector{
		client: client,
		store:  store,
	}

	err := crioCollector.Pull(context.Background())
	assert.NoError(t, err)
	container := store.notifiedEvents[0].Entity.(*workloadmeta.Container)

	assert.Equal(t, "", container.EntityMeta.Namespace) // Namespace should be empty
	assert.Equal(t, 12345, container.PID)
	assert.Equal(t, "container-host", container.Hostname)
	assert.Equal(t, "/crio/crio-45e0df1c6e04fda693f5ef2654363c1ff5667bee7f8a9042ff5c629d48fbcbc4", container.CgroupPath)
}

// TestPullContainerImageError verifies error handling when retrieving container image fails.
func TestPullContainerImageError(t *testing.T) {
	client := &fakeCRIOClient{
		mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
			return []*v1.Container{
				{Id: "container1", PodSandboxId: "pod1"},
			}, nil
		},
		mockGetContainerStatus: func(_ context.Context, _ string) (*v1.ContainerStatusResponse, error) {
			return &v1.ContainerStatusResponse{
				Status: &v1.ContainerStatus{
					Metadata:  &v1.ContainerMetadata{Name: "container1"},
					State:     v1.ContainerState_CONTAINER_RUNNING,
					CreatedAt: time.Now().Add(-10 * time.Minute).UnixNano(),
				},
				Info: map[string]string{
					"info": `{
						"pid": 12345,
						"runtimeSpec": {
							"hostname": "container-host",
							"linux": {
								"cgroupsPath": "/crio/crio-45e0df1c6e04fda693f5ef2654363c1ff5667bee7f8a9042ff5c629d48fbcbc4"
							}
						}
					}`,
				},
			}, nil
		},
		mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec) (*v1.Image, error) {
			return nil, errors.New("image retrieval error")
		},
	}

	store := &fakeWorkloadmetaStore{}
	crioCollector := collector{
		client: client,
		store:  store,
	}

	err := crioCollector.Pull(context.Background())
	assert.NoError(t, err)
	container := store.notifiedEvents[0].Entity.(*workloadmeta.Container)

	assert.Empty(t, container.Image.ID)
	assert.Empty(t, container.Image.RawName)
	assert.Equal(t, 12345, container.PID)
	assert.Equal(t, "container-host", container.Hostname)
	assert.Equal(t, "/crio/crio-45e0df1c6e04fda693f5ef2654363c1ff5667bee7f8a9042ff5c629d48fbcbc4", container.CgroupPath)
}

// TestPullNoContainers verifies that Pull handles an empty container list gracefully.
func TestPullNoContainers(t *testing.T) {
	client := &fakeCRIOClient{
		mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
			return []*v1.Container{}, nil
		},
	}

	store := &fakeWorkloadmetaStore{}
	crioCollector := collector{
		client: client,
		store:  store,
	}

	err := crioCollector.Pull(context.Background())
	assert.NoError(t, err)
	assert.Empty(t, store.notifiedEvents) // Should have no events
}

// TestPullContainerRetrievalError verifies that Pull handles an error when retrieving containers.
func TestPullContainerRetrievalError(t *testing.T) {
	client := &fakeCRIOClient{
		mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
			return nil, errors.New("failed to retrieve containers")
		},
	}

	store := &fakeWorkloadmetaStore{}
	crioCollector := collector{
		client: client,
		store:  store,
	}

	err := crioCollector.Pull(context.Background())
	assert.Error(t, err)
	assert.Empty(t, store.notifiedEvents) // No events should be generated
}

// TestPullContainerMissingMetadata verifies that Pull handles containers with missing metadata.
func TestPullContainerMissingMetadata(t *testing.T) {
	client := &fakeCRIOClient{
		mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
			return []*v1.Container{
				{Id: "container1", PodSandboxId: "pod1", Metadata: nil}, // Missing metadata
			}, nil
		},
		mockGetContainerStatus: func(_ context.Context, _ string) (*v1.ContainerStatusResponse, error) {
			return &v1.ContainerStatusResponse{
				Status: &v1.ContainerStatus{
					State: v1.ContainerState_CONTAINER_RUNNING,
				},
				Info: map[string]string{
					"info": `{
						"pid": 12345,
						"runtimeSpec": {
							"hostname": "container-host",
							"linux": {
								"cgroupsPath": "/crio/crio-45e0df1c6e04fda693f5ef2654363c1ff5667bee7f8a9042ff5c629d48fbcbc4"
							}
						}
					}`,
				},
			}, nil
		},
	}

	store := &fakeWorkloadmetaStore{}
	crioCollector := collector{
		client: client,
		store:  store,
	}

	err := crioCollector.Pull(context.Background())
	assert.NoError(t, err)
	container := store.notifiedEvents[0].Entity.(*workloadmeta.Container)

	assert.Equal(t, "", container.EntityMeta.Name) // Default to unknown name
	assert.Equal(t, 12345, container.PID)
	assert.Equal(t, "container-host", container.Hostname)
	assert.Equal(t, "/crio/crio-45e0df1c6e04fda693f5ef2654363c1ff5667bee7f8a9042ff5c629d48fbcbc4", container.CgroupPath)
}

// TestPullContainerDefaultResourceLimits verifies that Pull handles containers with default resource limits.
func TestPullContainerDefaultResourceLimits(t *testing.T) {
	client := &fakeCRIOClient{
		mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
			return []*v1.Container{
				{Id: "container1", PodSandboxId: "pod1"},
			}, nil
		},
		mockGetContainerStatus: func(_ context.Context, _ string) (*v1.ContainerStatusResponse, error) {
			return &v1.ContainerStatusResponse{
				Status: &v1.ContainerStatus{
					Metadata: &v1.ContainerMetadata{Name: "container1"},
					Resources: &v1.ContainerResources{
						Linux: &v1.LinuxContainerResources{
							CpuQuota: 0, CpuPeriod: 0, MemoryLimitInBytes: 0,
						},
					},
				},
				Info: map[string]string{
					"info": `{
						"pid": 12345,
						"runtimeSpec": {
							"hostname": "container-host",
							"linux": {
								"cgroupsPath": "/crio/crio-45e0df1c6e04fda693f5ef2654363c1ff5667bee7f8a9042ff5c629d48fbcbc4"
							}
						}
					}`,
				},
			}, nil
		},
	}

	store := &fakeWorkloadmetaStore{}
	crioCollector := collector{
		client: client,
		store:  store,
	}

	err := crioCollector.Pull(context.Background())
	assert.NoError(t, err)
	container := store.notifiedEvents[0].Entity.(*workloadmeta.Container)

	assert.Nil(t, container.Resources.CPULimit)
	assert.Nil(t, container.Resources.MemoryLimit)
	assert.Equal(t, 12345, container.PID)
	assert.Equal(t, "container-host", container.Hostname)
	assert.Equal(t, "/crio/crio-45e0df1c6e04fda693f5ef2654363c1ff5667bee7f8a9042ff5c629d48fbcbc4", container.CgroupPath)
}

// TestPullContainerResourceFallbackToInfo verifies that Pull uses resource limits from info when Resources in containerStatus is nil.
func TestPullContainerResourceFallbackToInfo(t *testing.T) {
	client := &fakeCRIOClient{
		mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
			return []*v1.Container{
				{Id: "container1", PodSandboxId: "pod1"},
			}, nil
		},
		mockGetContainerStatus: func(_ context.Context, _ string) (*v1.ContainerStatusResponse, error) {
			return &v1.ContainerStatusResponse{
				Status: &v1.ContainerStatus{
					Metadata:  &v1.ContainerMetadata{Name: "container1"},
					State:     v1.ContainerState_CONTAINER_RUNNING,
					CreatedAt: time.Now().Add(-10 * time.Minute).UnixNano(),
					Resources: nil, // No resources in status
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
	}

	store := &fakeWorkloadmetaStore{}
	crioCollector := collector{
		client: client,
		store:  store,
	}

	err := crioCollector.Pull(context.Background())
	assert.NoError(t, err)
	assert.Len(t, store.notifiedEvents, 1)
	container := store.notifiedEvents[0].Entity.(*workloadmeta.Container)

	assert.Equal(t, floatPtr(0.5), container.Resources.CPULimit)
	assert.Equal(t, uintPtr(104857600), container.Resources.MemoryLimit)
	assert.Equal(t, 12345, container.PID)
	assert.Equal(t, "container-host", container.Hostname)
	assert.Equal(t, "/crio/crio-45e0df1c6e04fda693f5ef2654363c1ff5667bee7f8a9042ff5c629d48fbcbc4", container.CgroupPath)
}

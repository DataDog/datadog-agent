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

// TestPull tests Pull with valid container data.
func TestPull(t *testing.T) {
	client := &mockCRIOClient{
		mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
			return []*v1.Container{
				{Id: "container1", PodSandboxId: "pod1", Metadata: &v1.ContainerMetadata{Name: "container1"}},
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
		mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec, verbose bool) (*v1.ImageStatusResponse, error) {
			return &v1.ImageStatusResponse{
				Image: &v1.Image{
					Id:          "image123",
					RepoTags:    []string{"myrepo/myimage:latest"},
					RepoDigests: []string{"myrepo/myimage@sha256:123abc"},
				},
			}, nil
		},
	}

	store := &mockWorkloadmetaStore{}
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
}

// TestPullContainerStatusError tests Pull when retrieving container status results in an error.
func TestPullContainerStatusError(t *testing.T) {
	client := &mockCRIOClient{
		mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
			return []*v1.Container{
				{Id: "container1", PodSandboxId: "pod1"},
			}, nil
		},
		mockGetContainerStatus: func(_ context.Context, _ string) (*v1.ContainerStatusResponse, error) {
			return nil, errors.New("container status error")
		},
	}

	store := &mockWorkloadmetaStore{}
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
}

// TestPullNoPodNamespace tests Pull with a missing pod namespace.
func TestPullNoPodNamespace(t *testing.T) {
	client := &mockCRIOClient{
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

	store := &mockWorkloadmetaStore{}
	crioCollector := collector{
		client: client,
		store:  store,
	}

	err := crioCollector.Pull(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "", store.notifiedEvents[0].Entity.(*workloadmeta.Container).EntityMeta.Namespace) // Namespace should be empty
}

// TestPullContainerImageError tests error handling when retrieving container image fails.
func TestPullContainerImageError(t *testing.T) {
	client := &mockCRIOClient{
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
		mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec, verbose bool) (*v1.ImageStatusResponse, error) {
			return nil, errors.New("image retrieval error")
		},
	}

	store := &mockWorkloadmetaStore{}
	crioCollector := collector{
		client: client,
		store:  store,
	}

	err := crioCollector.Pull(context.Background())
	assert.NoError(t, err)
	event := store.notifiedEvents[0]
	container := event.Entity.(*workloadmeta.Container)

	assert.Empty(t, container.Image.ID)
	assert.Empty(t, container.Image.RawName)
}

// TestPullContainerNoImageInfo verifies that Pull handles containers where the image info is missing.
func TestPullContainerNoImageInfo(t *testing.T) {
	client := &mockCRIOClient{
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
		mockGetContainerImage: func(_ context.Context, _ *v1.ImageSpec, verbose bool) (*v1.ImageStatusResponse, error) {
			return &v1.ImageStatusResponse{Image: nil}, nil // Simulate no image info available
		},
	}

	store := &mockWorkloadmetaStore{}
	crioCollector := collector{
		client: client,
		store:  store,
	}

	err := crioCollector.Pull(context.Background())
	assert.NoError(t, err)
	assert.NotEmpty(t, store.notifiedEvents) // Ensure an event is generated
	event := store.notifiedEvents[0]
	container := event.Entity.(*workloadmeta.Container)

	// Assert that image information is empty due to missing image info
	assert.Empty(t, container.Image.ID)
	assert.Empty(t, container.Image.RawName)
	assert.Equal(t, 12345, container.PID)
	assert.Equal(t, "container-host", container.Hostname)
	assert.Equal(t, "/crio/crio-45e0df1c6e04fda693f5ef2654363c1ff5667bee7f8a9042ff5c629d48fbcbc4", container.CgroupPath)
}

// TestPullNoContainers verifies that Pull handles an empty container list gracefully.
func TestPullNoContainers(t *testing.T) {
	client := &mockCRIOClient{
		mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
			return []*v1.Container{}, nil // Empty list
		},
	}

	store := &mockWorkloadmetaStore{}
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
	client := &mockCRIOClient{
		mockGetAllContainers: func(_ context.Context) ([]*v1.Container, error) {
			return nil, errors.New("failed to retrieve containers")
		},
	}

	store := &mockWorkloadmetaStore{}
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
	client := &mockCRIOClient{
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

	store := &mockWorkloadmetaStore{}
	crioCollector := collector{
		client: client,
		store:  store,
	}

	err := crioCollector.Pull(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "", store.notifiedEvents[0].Entity.(*workloadmeta.Container).EntityMeta.Name) // Default to unknown name
}

// TestPullContainerDefaultResourceLimits verifies that Pull handles containers with default resource limits.
func TestPullContainerDefaultResourceLimits(t *testing.T) {
	client := &mockCRIOClient{
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

	store := &mockWorkloadmetaStore{}
	crioCollector := collector{
		client: client,
		store:  store,
	}

	err := crioCollector.Pull(context.Background())
	assert.NoError(t, err)
	event := store.notifiedEvents[0]
	container := event.Entity.(*workloadmeta.Container)

	assert.Nil(t, container.Resources.CPULimit)
	assert.Nil(t, container.Resources.MemoryLimit)
}

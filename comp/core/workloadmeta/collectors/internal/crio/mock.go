// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build crio

package crio

import (
	"context"
	"errors"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// mockWorkloadmetaStore is a mock implementation of the workloadmeta store for testing purposes.
type mockWorkloadmetaStore struct {
	workloadmeta.Component
	notifiedEvents []workloadmeta.CollectorEvent
	existingImages map[string]*workloadmeta.ContainerImageMetadata
}

// Notify appends events to the store's notifiedEvents, simulating notification behavior in tests.
func (store *mockWorkloadmetaStore) Notify(events []workloadmeta.CollectorEvent) {
	store.notifiedEvents = append(store.notifiedEvents, events...)
}

// GetImage returns an image if it exists in the mock store, otherwise returns an error.
func (store *mockWorkloadmetaStore) GetImage(id string) (*workloadmeta.ContainerImageMetadata, error) {
	if store.existingImages == nil {
		return nil, errors.New("image not found")
	}
	
	if img, exists := store.existingImages[id]; exists {
		return img, nil
	}
	
	return nil, errors.New("image not found")
}

// mockCRIOClient simulates the CRI-O client, with configurable behavior through function hooks.
type mockCRIOClient struct {
	mockGetAllContainers   func(ctx context.Context) ([]*v1.Container, error)
	mockGetContainerStatus func(ctx context.Context, containerID string) (*v1.ContainerStatusResponse, error)
	mockGetPodStatus       func(ctx context.Context, podID string) (*v1.PodSandboxStatus, error)
	mockGetContainerImage  func(ctx context.Context, imageSpec *v1.ImageSpec, verbose bool) (*v1.ImageStatusResponse, error)
	mockRuntimeMetadata    func(ctx context.Context) (*v1.VersionResponse, error)
	mockGetCRIOImageLayers func(imgMeta *workloadmeta.ContainerImageMetadata) ([]string, error)
	mockListImages         func(ctx context.Context) ([]*v1.Image, error)
}

// GetAllContainers returns a list of containers, or calls a mock function if defined.
func (f *mockCRIOClient) GetAllContainers(ctx context.Context) ([]*v1.Container, error) {
	if f.mockGetAllContainers != nil {
		return f.mockGetAllContainers(ctx)
	}
	return []*v1.Container{}, nil
}

// GetContainerStatus retrieves the status of a container, or calls a mock function if defined.
func (f *mockCRIOClient) GetContainerStatus(ctx context.Context, containerID string) (*v1.ContainerStatusResponse, error) {
	if f.mockGetContainerStatus != nil {
		return f.mockGetContainerStatus(ctx, containerID)
	}
	return &v1.ContainerStatusResponse{}, nil
}

// GetPodStatus retrieves the status of a pod, or calls a mock function if defined.
func (f *mockCRIOClient) GetPodStatus(ctx context.Context, podID string) (*v1.PodSandboxStatus, error) {
	if f.mockGetPodStatus != nil {
		return f.mockGetPodStatus(ctx, podID)
	}
	return &v1.PodSandboxStatus{}, nil
}

// GetContainerImage retrieves image metadata, or calls a mock function if defined.
func (f *mockCRIOClient) GetContainerImage(ctx context.Context, imageSpec *v1.ImageSpec, verbose bool) (*v1.ImageStatusResponse, error) {
	if f.mockGetContainerImage != nil {
		return f.mockGetContainerImage(ctx, imageSpec, verbose)
	}
	return &v1.ImageStatusResponse{
		Image: &v1.Image{
			Id:          "image123",
			RepoTags:    []string{"myrepo/myimage:latest"},
			RepoDigests: []string{"myrepo/myimage@sha256:123abc"},
		},
	}, nil
}

// RuntimeMetadata retrieves the runtime metadata, or calls a mock function if defined.
func (f *mockCRIOClient) RuntimeMetadata(ctx context.Context) (*v1.VersionResponse, error) {
	if f.mockRuntimeMetadata != nil {
		return f.mockRuntimeMetadata(ctx)
	}
	return &v1.VersionResponse{RuntimeName: "cri-o", RuntimeVersion: "v1.30.0"}, nil
}

// GetCRIOImageLayers retrieves the `diff` directories of each image layer, or calls a mock function if defined.
func (f *mockCRIOClient) GetCRIOImageLayers(_ *workloadmeta.ContainerImageMetadata) ([]string, error) {
	if f.mockGetCRIOImageLayers != nil {
		return f.mockGetCRIOImageLayers(nil)
	}
	return nil, errors.New("mock GetCRIOImageLayers function not defined")
}

// ListImages retrieves all images, or calls a mock function if defined.
func (f *mockCRIOClient) ListImages(ctx context.Context) ([]*v1.Image, error) {
	if f.mockListImages != nil {
		return f.mockListImages(ctx)
	}
	return []*v1.Image{
		{
			Id:          "image123",
			RepoTags:    []string{"myrepo/myimage:latest"},
			RepoDigests: []string{"myrepo/myimage@sha256:123abc"},
		},
	}, nil
}

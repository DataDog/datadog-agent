// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package fake

import (
	"context"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"

	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

// MockedContainerdClient is a fake containerd client that implements the
// ContainerItf interface. It's only meant to be used in unit tests.
type MockedContainerdClient struct {
	MockClose                 func() error
	MockRawClient             func() *containerd.Client
	MockCheckConnectivity     func() *retry.Error
	MockEvents                func() containerd.EventService
	MockContainers            func(namespace string) ([]containerd.Container, error)
	MockContainer             func(namespace string, id string) (containerd.Container, error)
	MockContainerWithCtx      func(ctx context.Context, namespace string, id string) (containerd.Container, error)
	MockMetadata              func() (containerd.Version, error)
	MockListImages            func(namespace string) ([]containerd.Image, error)
	MockImage                 func(namespace string, name string) (containerd.Image, error)
	MockImageOfContainer      func(namespace string, ctn containerd.Container) (containerd.Image, error)
	MockImageSize             func(namespace string, ctn containerd.Container) (int64, error)
	MockTaskMetrics           func(namespace string, ctn containerd.Container) (*types.Metric, error)
	MockTaskPids              func(namespace string, ctn containerd.Container) ([]containerd.ProcessInfo, error)
	MockInfo                  func(namespace string, ctn containerd.Container) (containers.Container, error)
	MockLabels                func(namespace string, ctn containerd.Container) (map[string]string, error)
	MockLabelsWithContext     func(ctx context.Context, namespace string, ctn containerd.Container) (map[string]string, error)
	MockNamespaces            func(ctx context.Context) ([]string, error)
	MockSpec                  func(namespace string, ctn containers.Container) (*oci.Spec, error)
	MockStatus                func(namespace string, ctn containerd.Container) (containerd.ProcessStatus, error)
	MockCallWithClientContext func(namespace string, f func(context.Context) error) error
	MockIsSandbox             func(namespace string, ctn containerd.Container) (bool, error)
	MockMountImage            func(ctx context.Context, expiration time.Duration, namespace string, img containerd.Image, targetDir string) (func(context.Context) error, error)
}

// Close is a mock method
func (client *MockedContainerdClient) Close() error {
	return client.MockClose()
}

// Close is a mock method
func (client *MockedContainerdClient) RawClient() *containerd.Client {
	return client.MockRawClient()
}

// CheckConnectivity is a mock method
func (client *MockedContainerdClient) CheckConnectivity() *retry.Error {
	return client.MockCheckConnectivity()
}

// ListImages is a mock method
func (client *MockedContainerdClient) ListImages(namespace string) ([]containerd.Image, error) {
	return client.MockListImages(namespace)
}

// Image is a mock method
func (client *MockedContainerdClient) Image(namespace string, name string) (containerd.Image, error) {
	return client.MockImage(namespace, name)
}

// ImageOfContainer is a mock method
func (client *MockedContainerdClient) ImageOfContainer(namespace string, ctn containerd.Container) (containerd.Image, error) {
	return client.MockImageOfContainer(namespace, ctn)
}

// ImageSize is a mock method
func (client *MockedContainerdClient) ImageSize(namespace string, ctn containerd.Container) (int64, error) {
	return client.MockImageSize(namespace, ctn)
}

// Labels is a mock method
func (client *MockedContainerdClient) Labels(namespace string, ctn containerd.Container) (map[string]string, error) {
	return client.MockLabels(namespace, ctn)
}

// LabelsWithContext is a mock method
func (client *MockedContainerdClient) LabelsWithContext(ctx context.Context, namespace string, ctn containerd.Container) (map[string]string, error) {
	return client.MockLabelsWithContext(ctx, namespace, ctn)
}

// Info is a mock method
func (client *MockedContainerdClient) Info(namespace string, ctn containerd.Container) (containers.Container, error) {
	return client.MockInfo(namespace, ctn)
}

// TaskMetrics is a mock method
func (client *MockedContainerdClient) TaskMetrics(namespace string, ctn containerd.Container) (*types.Metric, error) {
	return client.MockTaskMetrics(namespace, ctn)
}

// TaskPids is a mock method
func (client *MockedContainerdClient) TaskPids(namespace string, ctn containerd.Container) ([]containerd.ProcessInfo, error) {
	return client.MockTaskPids(namespace, ctn)
}

// Metadata is a mock method
func (client *MockedContainerdClient) Metadata() (containerd.Version, error) {
	return client.MockMetadata()
}

// Namespaces is a mock method
func (client *MockedContainerdClient) Namespaces(ctx context.Context) ([]string, error) {
	return client.MockNamespaces(ctx)
}

// Containers is a mock method
func (client *MockedContainerdClient) Containers(namespace string) ([]containerd.Container, error) {
	return client.MockContainers(namespace)
}

// Container is a mock method
func (client *MockedContainerdClient) Container(namespace string, id string) (containerd.Container, error) {
	return client.MockContainer(namespace, id)
}

// ContainerWithContext is a mock method
func (client *MockedContainerdClient) ContainerWithContext(ctx context.Context, namespace string, id string) (containerd.Container, error) {
	return client.MockContainerWithCtx(ctx, namespace, id)
}

// GetEvents is a mock method
func (client *MockedContainerdClient) GetEvents() containerd.EventService {
	return client.MockEvents()
}

// Spec is a mock method
func (client *MockedContainerdClient) Spec(namespace string, ctn containers.Container, maxSize int) (*oci.Spec, error) {
	return client.MockSpec(namespace, ctn)
}

// Status is a mock method
func (client *MockedContainerdClient) Status(namespace string, ctn containerd.Container) (containerd.ProcessStatus, error) {
	return client.MockStatus(namespace, ctn)
}

// CallWithClientContext is a mock method
func (client *MockedContainerdClient) CallWithClientContext(namespace string, f func(context.Context) error) error {
	return client.MockCallWithClientContext(namespace, f)
}

// IsSandbox is a mock method
func (client *MockedContainerdClient) IsSandbox(namespace string, ctn containerd.Container) (bool, error) {
	return client.MockIsSandbox(namespace, ctn)
}

func (client *MockedContainerdClient) MountImage(ctx context.Context, expiration time.Duration, namespace string, img containerd.Image, targetDir string) (func(context.Context) error, error) {
	return client.MockMountImage(ctx, expiration, namespace, img, targetDir)
}

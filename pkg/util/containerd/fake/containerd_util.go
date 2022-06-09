// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd
// +build containerd

package fake

import (
	"context"

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
	MockCheckConnectivity     func() *retry.Error
	MockEvents                func() containerd.EventService
	MockContainers            func() ([]containerd.Container, error)
	MockContainer             func(id string) (containerd.Container, error)
	MockContainerWithCtx      func(ctx context.Context, id string) (containerd.Container, error)
	MockEnvVars               func(ctn containerd.Container) (map[string]string, error)
	MockMetadata              func() (containerd.Version, error)
	MockListImages            func() ([]containerd.Image, error)
	MockImage                 func(ctn containerd.Container) (containerd.Image, error)
	MockImageSize             func(ctn containerd.Container) (int64, error)
	MockTaskMetrics           func(ctn containerd.Container) (*types.Metric, error)
	MockTaskPids              func(ctn containerd.Container) ([]containerd.ProcessInfo, error)
	MockInfo                  func(ctn containerd.Container) (containers.Container, error)
	MockLabels                func(ctn containerd.Container) (map[string]string, error)
	MockLabelsWithContext     func(ctx context.Context, ctn containerd.Container) (map[string]string, error)
	MockCurrentNamespace      func() string
	MockSetCurrentNamespace   func(namespace string)
	MockNamespaces            func(ctx context.Context) ([]string, error)
	MockSpec                  func(ctn containerd.Container) (*oci.Spec, error)
	MockSpecWithContext       func(ctx context.Context, ctn containerd.Container) (*oci.Spec, error)
	MockStatus                func(ctn containerd.Container) (containerd.ProcessStatus, error)
	MockCallWithClientContext func(f func(context.Context) error) error
	MockAnnotations           func(ctn containerd.Container) (map[string]string, error)
	MockIsSandbox             func(ctn containerd.Container) (bool, error)
}

// Close TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) Close() error {
	return client.MockClose()
}

// CheckConnectivity TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) CheckConnectivity() *retry.Error {
	return client.MockCheckConnectivity()
}

// ListImages TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) ListImages() ([]containerd.Image, error) {
	return client.MockListImages()
}

// Image TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) Image(ctn containerd.Container) (containerd.Image, error) {
	return client.MockImage(ctn)
}

// ImageSize TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) ImageSize(ctn containerd.Container) (int64, error) {
	return client.MockImageSize(ctn)
}

// Labels TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) Labels(ctn containerd.Container) (map[string]string, error) {
	return client.MockLabels(ctn)
}

// LabelsWithContext TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) LabelsWithContext(ctx context.Context, ctn containerd.Container) (map[string]string, error) {
	return client.MockLabelsWithContext(ctx, ctn)
}

// Info TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) Info(ctn containerd.Container) (containers.Container, error) {
	return client.MockInfo(ctn)
}

// TaskMetrics TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) TaskMetrics(ctn containerd.Container) (*types.Metric, error) {
	return client.MockTaskMetrics(ctn)
}

// TaskPids TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) TaskPids(ctn containerd.Container) ([]containerd.ProcessInfo, error) {
	return client.MockTaskPids(ctn)
}

// Metadata TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) Metadata() (containerd.Version, error) {
	return client.MockMetadata()
}

// CurrentNamespace TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) CurrentNamespace() string {
	return client.MockCurrentNamespace()
}

// SetCurrentNamespace TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) SetCurrentNamespace(namespace string) {
	client.MockSetCurrentNamespace(namespace)
}

// Namespaces TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) Namespaces(ctx context.Context) ([]string, error) {
	return client.MockNamespaces(ctx)
}

// Containers TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) Containers() ([]containerd.Container, error) {
	return client.MockContainers()
}

// Container TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) Container(id string) (containerd.Container, error) {
	return client.MockContainer(id)
}

// ContainerWithContext TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) ContainerWithContext(ctx context.Context, id string) (containerd.Container, error) {
	return client.MockContainerWithCtx(ctx, id)
}

// GetEvents TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) GetEvents() containerd.EventService {
	return client.MockEvents()
}

// Spec TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) Spec(ctn containerd.Container) (*oci.Spec, error) {
	return client.MockSpec(ctn)
}

// SpecWithContext TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) SpecWithContext(ctx context.Context, ctn containerd.Container) (*oci.Spec, error) {
	return client.MockSpecWithContext(ctx, ctn)
}

// EnvVars TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) EnvVars(ctn containerd.Container) (map[string]string, error) {
	return client.MockEnvVars(ctn)
}

// Status TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) Status(ctn containerd.Container) (containerd.ProcessStatus, error) {
	return client.MockStatus(ctn)
}

// CallWithClientContext TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) CallWithClientContext(f func(context.Context) error) error {
	return client.MockCallWithClientContext(f)
}

// Annotations TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) Annotations(ctn containerd.Container) (map[string]string, error) {
	return client.MockAnnotations(ctn)
}

// IsSandbox TODO <container-integrations>: CONT-3353
func (client *MockedContainerdClient) IsSandbox(ctn containerd.Container) (bool, error) {
	return client.MockIsSandbox(ctn)
}

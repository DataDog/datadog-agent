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
}

func (client *MockedContainerdClient) Close() error {
	return client.MockClose()
}

func (client *MockedContainerdClient) CheckConnectivity() *retry.Error {
	return client.MockCheckConnectivity()
}

func (client *MockedContainerdClient) ListImages() ([]containerd.Image, error) {
	return client.MockListImages()
}

func (client *MockedContainerdClient) Image(ctn containerd.Container) (containerd.Image, error) {
	return client.MockImage(ctn)
}

func (client *MockedContainerdClient) ImageSize(ctn containerd.Container) (int64, error) {
	return client.MockImageSize(ctn)
}

func (client *MockedContainerdClient) Labels(ctn containerd.Container) (map[string]string, error) {
	return client.MockLabels(ctn)
}

func (client *MockedContainerdClient) LabelsWithContext(ctx context.Context, ctn containerd.Container) (map[string]string, error) {
	return client.MockLabelsWithContext(ctx, ctn)
}

func (client *MockedContainerdClient) Info(ctn containerd.Container) (containers.Container, error) {
	return client.MockInfo(ctn)
}

func (client *MockedContainerdClient) TaskMetrics(ctn containerd.Container) (*types.Metric, error) {
	return client.MockTaskMetrics(ctn)
}

func (client *MockedContainerdClient) TaskPids(ctn containerd.Container) ([]containerd.ProcessInfo, error) {
	return client.MockTaskPids(ctn)
}

func (client *MockedContainerdClient) Metadata() (containerd.Version, error) {
	return client.MockMetadata()
}

func (client *MockedContainerdClient) CurrentNamespace() string {
	return client.MockCurrentNamespace()
}

func (client *MockedContainerdClient) SetCurrentNamespace(namespace string) {
	client.MockSetCurrentNamespace(namespace)
}

func (client *MockedContainerdClient) Namespaces(ctx context.Context) ([]string, error) {
	return client.MockNamespaces(ctx)
}

func (client *MockedContainerdClient) Containers() ([]containerd.Container, error) {
	return client.MockContainers()
}

func (client *MockedContainerdClient) Container(id string) (containerd.Container, error) {
	return client.MockContainer(id)
}

func (client *MockedContainerdClient) ContainerWithContext(ctx context.Context, id string) (containerd.Container, error) {
	return client.MockContainerWithCtx(ctx, id)
}

func (client *MockedContainerdClient) GetEvents() containerd.EventService {
	return client.MockEvents()
}

func (client *MockedContainerdClient) Spec(ctn containerd.Container) (*oci.Spec, error) {
	return client.MockSpec(ctn)
}

func (client *MockedContainerdClient) SpecWithContext(ctx context.Context, ctn containerd.Container) (*oci.Spec, error) {
	return client.MockSpecWithContext(ctx, ctn)
}

func (client *MockedContainerdClient) EnvVars(ctn containerd.Container) (map[string]string, error) {
	return client.MockEnvVars(ctn)
}

func (client *MockedContainerdClient) Status(ctn containerd.Container) (containerd.ProcessStatus, error) {
	return client.MockStatus(ctn)
}

func (client *MockedContainerdClient) CallWithClientContext(f func(context.Context) error) error {
	return client.MockCallWithClientContext(f)
}

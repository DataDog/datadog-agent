// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build containerd

package containerd

import (
	"context"
	"testing"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/stretchr/testify/assert"

	containerdutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

type mockedContainer struct {
	containerd.Container
	mockID func() string
}

func (m *mockedContainer) ID() string {
	return m.mockID()
}

type mockedImage struct {
	containerd.Image
	mockName func() string
}

func (m *mockedImage) Name() string {
	return m.mockName()
}

type mockedContainerdClient struct {
	containerdutil.ContainerdItf
	// Not all the funcs are here. Add them as needed.
	mockContainerWithContext func(ctx context.Context, id string) (containerd.Container, error)
	mockEnvVars              func(ctn containerd.Container) (map[string]string, error)
	mockImage                func(ctn containerd.Container) (containerd.Image, error)
	mockInfo                 func(ctn containerd.Container) (containers.Container, error)
	mockLabels               func(ctn containerd.Container) (map[string]string, error)
	mockSpec                 func(ctn containerd.Container) (*oci.Spec, error)
	mockStatus               func(ctn containerd.Container) (containerd.ProcessStatus, error)
}

func (m *mockedContainerdClient) ContainerWithContext(ctx context.Context, id string) (containerd.Container, error) {
	return m.mockContainerWithContext(ctx, id)
}

func (m *mockedContainerdClient) EnvVars(ctn containerd.Container) (map[string]string, error) {
	return m.mockEnvVars(ctn)
}

func (m *mockedContainerdClient) Image(ctn containerd.Container) (containerd.Image, error) {
	return m.mockImage(ctn)
}

func (m *mockedContainerdClient) Info(ctn containerd.Container) (containers.Container, error) {
	return m.mockInfo(ctn)
}

func (m *mockedContainerdClient) Labels(ctn containerd.Container) (map[string]string, error) {
	return m.mockLabels(ctn)
}

func (m *mockedContainerdClient) Spec(ctn containerd.Container) (*oci.Spec, error) {
	return m.mockSpec(ctn)
}

func (m *mockedContainerdClient) Status(ctn containerd.Container) (containerd.ProcessStatus, error) {
	return m.mockStatus(ctn)
}

func TestBuildWorkloadMetaContainer(t *testing.T) {
	containerID := "10"
	labels := map[string]string{
		"some_label": "some_val",
	}
	imgName := "datadog/agent:7"
	envVars := map[string]string{
		"test_env": "test_val",
	}
	hostName := "test_hostname"
	createdAt, err := time.Parse("2006-01-02", "2021-10-11")
	assert.NoError(t, err)

	container := mockedContainer{
		mockID: func() string {
			return containerID
		},
	}

	client := mockedContainerdClient{
		mockLabels: func(ctn containerd.Container) (map[string]string, error) {
			return labels, nil
		},
		mockImage: func(ctn containerd.Container) (containerd.Image, error) {
			return &mockedImage{
				mockName: func() string {
					return imgName
				},
			}, nil
		},
		mockEnvVars: func(ctn containerd.Container) (map[string]string, error) {
			return envVars, nil
		},
		mockInfo: func(ctn containerd.Container) (containers.Container, error) {
			return containers.Container{CreatedAt: createdAt}, nil
		},
		mockSpec: func(ctn containerd.Container) (*oci.Spec, error) {
			return &oci.Spec{Hostname: hostName}, nil
		},
		mockStatus: func(ctn containerd.Container) (containerd.ProcessStatus, error) {
			return containerd.Running, nil
		},
	}

	result, err := buildWorkloadMetaContainer(&container, &client)
	assert.NoError(t, err)

	expected := workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   "", // Not available
			Labels: labels,
		},
		Image: workloadmeta.ContainerImage{
			RawName:   "datadog/agent:7",
			Name:      "datadog/agent",
			ShortName: "agent",
			Tag:       "7",
		},
		EnvVars: envVars,
		Ports:   nil, // Not available
		Runtime: workloadmeta.ContainerRuntimeContainerd,
		State: workloadmeta.ContainerState{
			Running:    true,
			StartedAt:  createdAt,
			FinishedAt: time.Time{}, // Not available
		},
		NetworkIPs: make(map[string]string), // Not available
		Hostname:   hostName,
		PID:        0, // Not available
	}
	assert.Equal(t, expected, result)
}

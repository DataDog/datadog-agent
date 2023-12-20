// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/containerd/fake"
)

type mockedContainer struct {
	containerd.Container
	mockID    func() string
	mockImage func() (containerd.Image, error)
}

func (m *mockedContainer) ID() string {
	return m.mockID()
}

// Image is from the containerd.Container interface
func (m *mockedContainer) Image(context.Context) (containerd.Image, error) {
	return m.mockImage()
}

type mockedImage struct {
	containerd.Image
	mockName   func() string
	mockConfig func() (ocispec.Descriptor, error)
}

func (m *mockedImage) Config(_ context.Context) (ocispec.Descriptor, error) {
	return m.mockConfig()
}

func TestBuildWorkloadMetaContainer(t *testing.T) {
	namespace := "default"
	containerID := "10"
	labels := map[string]string{
		"some_label": "some_val",
	}
	imgName := "datadog/agent:7"
	envVarStrs := []string{
		"test_env=test_val",
	}
	envVars := map[string]string{}
	for _, s := range envVarStrs {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) < 2 {
			continue
		}
		envVars[parts[0]] = parts[1]
	}
	hostName := "test_hostname"
	createdAt, err := time.Parse("2006-01-02", "2021-10-11")
	assert.NoError(t, err)

	image := &mockedImage{
		mockConfig: func() (ocispec.Descriptor, error) {
			return ocispec.Descriptor{Digest: "my_image_id"}, nil
		},
	}
	container := mockedContainer{
		mockID: func() string {
			return containerID
		},
		mockImage: func() (containerd.Image, error) {
			return image, nil
		},
	}

	client := fake.MockedContainerdClient{
		MockInfo: func(namespace string, ctn containerd.Container) (containers.Container, error) {
			return containers.Container{
				Labels:    labels,
				CreatedAt: createdAt,
				Image:     imgName,
				Runtime: containers.RuntimeInfo{
					Name: "io.containerd.kata-qemu.v2",
				},
			}, nil
		},
		MockSpec: func(namespace string, ctn containers.Container) (*oci.Spec, error) {
			return &oci.Spec{Hostname: hostName, Process: &specs.Process{Env: envVarStrs}}, nil
		},
		MockStatus: func(namespace string, ctn containerd.Container) (containerd.ProcessStatus, error) {
			return containerd.Running, nil
		},
		MockTaskPids: func(namespace string, ctn containerd.Container) ([]containerd.ProcessInfo, error) {
			return nil, nil
		},
	}

	result, err := buildWorkloadMetaContainer(namespace, &container, &client)
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
			ID:        "my_image_id",
		},
		EnvVars:       envVars,
		Ports:         nil, // Not available
		Runtime:       workloadmeta.ContainerRuntimeContainerd,
		RuntimeFlavor: workloadmeta.ContainerRuntimeFlavorKata,
		State: workloadmeta.ContainerState{
			Running:    true,
			Status:     workloadmeta.ContainerStatusRunning,
			StartedAt:  createdAt,
			CreatedAt:  createdAt,
			FinishedAt: time.Time{}, // Not available
		},
		NetworkIPs: make(map[string]string), // Not available
		Hostname:   hostName,
		PID:        0, // Not available
	}
	assert.Equal(t, expected, result)
}

func TestExtractRuntimeFlavor(t *testing.T) {
	tests := []struct {
		name     string
		runtime  string
		expected workloadmeta.ContainerRuntimeFlavor
	}{
		{
			name:     "kata",
			runtime:  "io.containerd.kata.v2",
			expected: workloadmeta.ContainerRuntimeFlavorKata,
		},
		{
			name:     "kata-qemu",
			runtime:  "io.containerd.kata-qemu.v2",
			expected: workloadmeta.ContainerRuntimeFlavorKata,
		},
		{
			name:     "non-kata",
			runtime:  "io.containerd.runc.v2",
			expected: workloadmeta.ContainerRuntimeFlavorDefault,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRuntimeFlavor(tt.runtime)
			assert.Equal(t, tt.expected, result)
		})
	}
}

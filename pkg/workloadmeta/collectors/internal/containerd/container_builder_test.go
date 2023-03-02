// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd
// +build containerd

package containerd

import (
	"strings"
	"testing"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/containerd/fake"
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

	container := mockedContainer{
		mockID: func() string {
			return containerID
		},
	}

	client := fake.MockedContainerdClient{
		MockInfo: func(namespace string, ctn containerd.Container) (containers.Container, error) {
			return containers.Container{
				Labels:    labels,
				CreatedAt: createdAt,
				Image:     imgName,
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
		},
		EnvVars: envVars,
		Ports:   nil, // Not available
		Runtime: workloadmeta.ContainerRuntimeContainerd,
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

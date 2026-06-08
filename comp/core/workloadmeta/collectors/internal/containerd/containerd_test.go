// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"testing"

	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/containerd/containerd"
	containerdcontainers "github.com/containerd/containerd/containers"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/containerd/fake"
)

func TestIgnoreContainer(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("container_exclude", "name:agent-excluded")
	mockFilterStore := workloadfilterfxmock.SetupMockFilter(t)

	containerID := "123"

	container := mockedContainer{
		mockID: func() string {
			return containerID
		},
	}

	tests := []struct {
		name           string
		imgName        string
		isSandbox      bool
		container      containerd.Container
		expectsIgnored bool
	}{
		{
			name:           "pause image",
			imgName:        "k8s.gcr.io/pause",
			container:      &container,
			isSandbox:      false,
			expectsIgnored: true,
		},
		{
			name:           "is sandbox",
			imgName:        "k8s.gcr.io/pause",
			container:      &container,
			isSandbox:      true,
			expectsIgnored: true,
		},
		{
			name:           "non-pause container that exists",
			imgName:        "datadog/agent",
			container:      &container,
			isSandbox:      false,
			expectsIgnored: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := fake.MockedContainerdClient{
				MockInfo: func(string, containerd.Container) (containerdcontainers.Container, error) {
					return containerdcontainers.Container{
						Image: test.imgName,
					}, nil
				},
				MockIsSandbox: func(string, containerd.Container) (bool, error) {
					return test.isSandbox, nil
				},
			}

			containerdCollector := collector{
				containerdClient:       &client,
				filterPausedContainers: mockFilterStore.GetContainerPausedFilters(),
			}

			ignored, err := containerdCollector.ignoreContainer("default", test.container)
			assert.NoError(t, err)

			if test.expectsIgnored {
				assert.True(t, ignored)
			} else {
				assert.False(t, ignored)
			}
		})
	}
}

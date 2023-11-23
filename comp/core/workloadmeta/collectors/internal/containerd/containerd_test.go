// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"testing"

	"github.com/containerd/containerd"
	containerdcontainers "github.com/containerd/containerd/containers"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/containerd/fake"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

func TestIgnoreContainer(t *testing.T) {
	pauseFilter, err := containers.GetPauseContainerFilter()
	assert.NoError(t, err)

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
				MockInfo: func(namespace string, ctn containerd.Container) (containerdcontainers.Container, error) {
					return containerdcontainers.Container{
						Image: test.imgName,
					}, nil
				},
				MockIsSandbox: func(namespace string, ctn containerd.Container) (bool, error) {
					return test.isSandbox, nil
				},
			}

			containerdCollector := collector{
				containerdClient:       &client,
				filterPausedContainers: pauseFilter,
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

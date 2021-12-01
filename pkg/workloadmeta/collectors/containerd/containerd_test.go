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
	apievents "github.com/containerd/containerd/api/events"
	containerdevents "github.com/containerd/containerd/events"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

func TestIgnoreEvent(t *testing.T) {
	pauseFilter, err := containers.GetPauseContainerFilter()
	assert.NoError(t, err)

	containerID := "123"

	eventEncoded, err := proto.Marshal(&apievents.ContainerCreate{
		ID: containerID,
	})
	assert.NoError(t, err)

	event := containerdevents.Envelope{
		Timestamp: time.Now(),
		Topic:     containerCreationTopic,
		Event: &types.Any{
			TypeUrl: "containerd.events.ContainerCreate",
			Value:   eventEncoded,
		},
	}

	container := mockedContainer{
		mockID: func() string {
			return containerID
		},
	}

	tests := []struct {
		name           string
		imgName        string
		getContainerFn func(ctx context.Context, id string) (containerd.Container, error)
		expectsIgnored bool
	}{
		{
			name:    "pause image",
			imgName: "k8s.gcr.io/pause",
			getContainerFn: func(ctx context.Context, id string) (containerd.Container, error) {
				return &container, nil
			},
			expectsIgnored: true,
		},
		{
			name:    "non-pause container that exists",
			imgName: "datadog/agent",
			getContainerFn: func(ctx context.Context, id string) (containerd.Container, error) {
				return &container, nil
			},
			expectsIgnored: false,
		},
		{
			name:    "container that does not exist",
			imgName: "datadog/agent",
			getContainerFn: func(ctx context.Context, id string) (containerd.Container, error) {
				return nil, errors.NewNotFound(id)
			},
			expectsIgnored: false, // Because it's a delete event that needs to be handled
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := mockedContainerdClient{
				mockContainerWithContext: test.getContainerFn,
				mockImage: func(ctn containerd.Container) (containerd.Image, error) {
					return &mockedImage{
						mockName: func() string {
							return test.imgName
						},
					}, nil
				},
			}

			containerdCollector := collector{
				containerdClient:       &client,
				filterPausedContainers: pauseFilter,
			}

			ignored, err := containerdCollector.ignoreEvent(context.TODO(), &event)
			assert.NoError(t, err)

			if test.expectsIgnored {
				assert.True(t, ignored)
			} else {
				assert.False(t, ignored)
			}
		})
	}
}

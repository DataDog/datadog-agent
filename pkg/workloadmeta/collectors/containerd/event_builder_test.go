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
	"github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/containers"
	containerdevents "github.com/containerd/containerd/events"
	"github.com/containerd/containerd/oci"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func TestBuildCollectorEvent(t *testing.T) {
	containerID := "10"

	container := mockedContainer{
		mockID: func() string {
			return containerID
		},
	}

	client := containerdClient(&container)

	workloadMetaContainer, err := buildWorkloadMetaContainer(&container, &client)
	assert.NoError(t, err)

	creationEvent, err := proto.Marshal(&events.ContainerCreate{
		ID: containerID,
	})
	assert.NoError(t, err)

	updateEvent, err := proto.Marshal(&events.ContainerUpdate{
		ID: containerID,
	})
	assert.NoError(t, err)

	deleteEvent, err := proto.Marshal(&events.ContainerDelete{
		ID: containerID,
	})
	assert.NoError(t, err)

	eventWithoutID, err := proto.Marshal(&events.ContainerCreate{
		ID: "",
	})
	assert.NoError(t, err)

	tests := []struct {
		name          string
		event         containerdevents.Envelope
		expectedEvent workloadmeta.CollectorEvent
		expectsError  bool
	}{
		{
			name: "create event",
			event: containerdevents.Envelope{
				Topic: containerCreationTopic,
				Event: &types.Any{
					TypeUrl: "containerd.events.ContainerCreate", Value: creationEvent,
				},
			},
			expectedEvent: workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: collectorID,
				Entity: &workloadMetaContainer,
			},
		},
		{
			name: "update event",
			event: containerdevents.Envelope{
				Topic: containerUpdateTopic,
				Event: &types.Any{
					TypeUrl: "containerd.events.ContainerUpdate", Value: updateEvent,
				},
			},
			expectedEvent: workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: collectorID,
				Entity: &workloadMetaContainer,
			},
		},
		{
			name: "delete event",
			event: containerdevents.Envelope{
				Topic: containerDeletionTopic,
				Event: &types.Any{
					TypeUrl: "containerd.events.ContainerDelete", Value: deleteEvent,
				},
			},
			expectedEvent: workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeUnset,
				Source: collectorID,
				Entity: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   containerID,
				},
			},
		},
		{
			name: "unknown event",
			event: containerdevents.Envelope{
				Topic: "Unknown Topic", // This causes the error
				Event: &types.Any{
					// Uses delete, but could be any other event in this test
					TypeUrl: "containerd.events.ContainerDelete", Value: deleteEvent,
				},
			},
			expectsError: true,
		},
		{
			name: "event without ID",
			event: containerdevents.Envelope{
				Topic: containerCreationTopic,
				Event: &types.Any{
					TypeUrl: "containerd.events.ContainerCreate", Value: eventWithoutID,
				},
			},
			expectsError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workloadMetaEvent, err := buildCollectorEvent(context.TODO(), &test.event, &client)

			if test.expectsError {
				assert.Error(t, err)
			} else {
				assert.Equal(t, test.expectedEvent, workloadMetaEvent)
			}
		})
	}
}

// containerdClient returns a mockedContainerdClient set up for the tests in this file.
func containerdClient(container containerd.Container) mockedContainerdClient {
	labels := map[string]string{"some_label": "some_val"}
	imgName := "datadog/agent:7"
	envVars := map[string]string{"test_env": "test_val"}
	hostName := "test_hostname"
	createdAt, _ := time.Parse("2006-01-02", "2021-10-11")

	return mockedContainerdClient{
		mockContainerWithContext: func(ctx context.Context, id string) (containerd.Container, error) {
			return container, nil
		},
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
}

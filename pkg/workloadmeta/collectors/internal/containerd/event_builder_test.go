// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

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
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/containerd/fake"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func TestBuildCollectorEvent(t *testing.T) {
	containerID := "10"
	namespace := "test_namespace"

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

	exitCode := uint32(137)
	exitTime := time.Now()
	fakeExitInfo := &exitInfo{exitCode: &exitCode, exitTS: exitTime}

	client := containerdClient(&container)

	workloadMetaContainer, err := buildWorkloadMetaContainer(namespace, &container, &client)
	workloadMetaContainer.Namespace = namespace
	assert.NoError(t, err)

	containerCreationEvent, err := proto.Marshal(&events.ContainerCreate{
		ID: containerID,
	})
	assert.NoError(t, err)

	containerUpdateEvent, err := proto.Marshal(&events.ContainerUpdate{
		ID: containerID,
	})
	assert.NoError(t, err)

	containerDeleteEvent, err := proto.Marshal(&events.ContainerDelete{
		ID: containerID,
	})
	assert.NoError(t, err)

	taskStartEvent, err := proto.Marshal(&events.TaskStart{
		ContainerID: containerID,
	})
	assert.NoError(t, err)

	taskOOMEvent, err := proto.Marshal(&events.TaskOOM{
		ContainerID: containerID,
	})
	assert.NoError(t, err)

	taskExitEvent, err := proto.Marshal(&events.TaskExit{
		ContainerID: containerID,
	})
	assert.NoError(t, err)

	taskDeleteEvent, err := proto.Marshal(&events.TaskDelete{
		ContainerID: containerID,
	})
	assert.NoError(t, err)

	taskPausedEvent, err := proto.Marshal(&events.TaskPaused{
		ContainerID: containerID,
	})
	assert.NoError(t, err)

	taskResumedEvent, err := proto.Marshal(&events.TaskResumed{
		ContainerID: containerID,
	})
	assert.NoError(t, err)

	tests := []struct {
		name          string
		event         containerdevents.Envelope
		expectedEvent workloadmeta.CollectorEvent
		expectsError  bool
		exitInfo      *exitInfo
	}{
		{
			name: "container create event",
			event: containerdevents.Envelope{
				Namespace: namespace,
				Topic:     containerCreationTopic,
				Event: &types.Any{
					TypeUrl: "containerd.events.ContainerCreate", Value: containerCreationEvent,
				},
			},
			expectedEvent: workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceRuntime,
				Entity: &workloadMetaContainer,
			},
		},
		{
			name: "container update event",
			event: containerdevents.Envelope{
				Namespace: namespace,
				Topic:     containerUpdateTopic,
				Event: &types.Any{
					TypeUrl: "containerd.events.ContainerUpdate", Value: containerUpdateEvent,
				},
			},
			expectedEvent: workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceRuntime,
				Entity: &workloadMetaContainer,
			},
		},
		{
			name: "container delete event",
			event: containerdevents.Envelope{
				Namespace: namespace,
				Topic:     containerDeletionTopic,
				Event: &types.Any{
					TypeUrl: "containerd.events.ContainerDelete", Value: containerDeleteEvent,
				},
			},
			expectedEvent: workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeUnset,
				Source: workloadmeta.SourceRuntime,
				Entity: &workloadmeta.Container{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   containerID,
					},
					State: workloadmeta.ContainerState{
						ExitCode:   &exitCode,
						FinishedAt: exitTime,
					},
				},
			},
			exitInfo: fakeExitInfo,
		},
		{
			name: "unknown event",
			event: containerdevents.Envelope{
				Namespace: namespace,
				Topic:     "Unknown Topic", // This causes the error
				Event: &types.Any{
					// Uses delete, but could be any other event in this test
					TypeUrl: "containerd.events.ContainerDelete", Value: containerDeleteEvent,
				},
			},
			expectsError: true,
		},
		{
			name: "task start event",
			event: containerdevents.Envelope{
				Namespace: namespace,
				Topic:     TaskStartTopic,
				Event: &types.Any{
					TypeUrl: "containerd.events.TaskStart", Value: taskStartEvent,
				},
			},
			expectedEvent: workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceRuntime,
				Entity: &workloadMetaContainer,
			},
		},
		{
			name: "task OOM event",
			event: containerdevents.Envelope{
				Namespace: namespace,
				Topic:     TaskOOMTopic,
				Event: &types.Any{
					TypeUrl: "containerd.events.TaskOOM", Value: taskOOMEvent,
				},
			},
			expectedEvent: workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceRuntime,
				Entity: &workloadMetaContainer,
			},
		},
		{
			name: "task exit event",
			event: containerdevents.Envelope{
				Namespace: namespace,
				Topic:     TaskExitTopic,
				Event: &types.Any{
					TypeUrl: "containerd.events.TaskExit", Value: taskExitEvent,
				},
			},
			expectedEvent: workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceRuntime,
				Entity: &workloadMetaContainer,
			},
		},
		{
			name: "task delete event",
			event: containerdevents.Envelope{
				Namespace: namespace,
				Topic:     TaskDeleteTopic,
				Event: &types.Any{
					TypeUrl: "containerd.events.TaskDelete", Value: taskDeleteEvent,
				},
			},
			expectedEvent: workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceRuntime,
				Entity: &workloadMetaContainer,
			},
		},
		{
			name: "task paused event",
			event: containerdevents.Envelope{
				Namespace: namespace,
				Topic:     TaskStartTopic,
				Event: &types.Any{
					TypeUrl: "containerd.events.TaskPaused", Value: taskPausedEvent,
				},
			},
			expectedEvent: workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceRuntime,
				Entity: &workloadMetaContainer,
			},
		},
		{
			name: "task resumed event",
			event: containerdevents.Envelope{
				Namespace: namespace,
				Topic:     TaskStartTopic,
				Event: &types.Any{
					TypeUrl: "containerd.events.TaskResumed", Value: taskResumedEvent,
				},
			},
			expectedEvent: workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceRuntime,
				Entity: &workloadMetaContainer,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := &collector{containerdClient: &client, contToExitInfo: make(map[string]*exitInfo)}
			if test.exitInfo != nil {
				c.contToExitInfo[containerID] = test.exitInfo
			}

			workloadMetaEvent, err := c.buildCollectorEvent(&test.event, container.ID(), &container)

			if test.expectsError {
				assert.Error(t, err)
			} else {
				assert.Equal(t, test.expectedEvent, workloadMetaEvent)
			}
		})
	}
}

// containerdClient returns a mockedContainerdClient set up for the tests in this file.
func containerdClient(container containerd.Container) fake.MockedContainerdClient {
	labels := map[string]string{"some_label": "some_val"}
	imgName := "datadog/agent:7"
	envVarStrs := []string{"test_env=test_val"}
	hostName := "test_hostname"
	createdAt, _ := time.Parse("2006-01-02", "2021-10-11")

	return fake.MockedContainerdClient{
		MockContainerWithCtx: func(ctx context.Context, namespace string, id string) (containerd.Container, error) {
			return container, nil
		},
		MockLabels: func(namespace string, ctn containerd.Container) (map[string]string, error) {
			return labels, nil
		},
		MockImageOfContainer: func(namespace string, ctn containerd.Container) (containerd.Image, error) {
			return &mockedImage{
				mockName: func() string {
					return imgName
				},
			}, nil
		},
		MockInfo: func(namespace string, ctn containerd.Container) (containers.Container, error) {
			return containers.Container{CreatedAt: createdAt}, nil
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
}

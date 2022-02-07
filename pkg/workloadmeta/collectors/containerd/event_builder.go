// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd
// +build containerd

package containerd

import (
	"context"
	"fmt"

	"github.com/containerd/containerd/api/events"
	containerdevents "github.com/containerd/containerd/events"
	"github.com/gogo/protobuf/proto"

	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// buildCollectorEvent generates a CollectorEvent from a containerdevents.Envelope
func (c *collector) buildCollectorEvent(ctx context.Context, containerdEvent *containerdevents.Envelope) (workloadmeta.CollectorEvent, error) {
	switch containerdEvent.Topic {
	case containerCreationTopic, containerUpdateTopic:
		ID, hasID := containerdEvent.Field([]string{"event", "id"})
		if !hasID {
			return workloadmeta.CollectorEvent{}, fmt.Errorf("missing ID in containerd event")
		}

		return createSetEvent(ctx, ID, containerdEvent.Namespace, c.containerdClient)
	case containerDeletionTopic:
		ID, hasID := containerdEvent.Field([]string{"event", "id"})
		if !hasID {
			return workloadmeta.CollectorEvent{}, fmt.Errorf("missing ID in containerd event")
		}

		exitInfo := c.getExitInfo(ID)
		defer c.deleteExitInfo(ID)

		return createDeletionEvent(ID, exitInfo), nil
	case TaskExitTopic:
		exited := &events.TaskExit{}
		if err := proto.Unmarshal(containerdEvent.Event.Value, exited); err != nil {
			return workloadmeta.CollectorEvent{}, err
		}

		c.cacheExitInfo(exited.ContainerID, &exited.ExitStatus, exited.ExitedAt)
		return createSetEventFromTask(ctx, containerdEvent.Namespace, containerdEvent, c.containerdClient)
	case TaskDeleteTopic:
		deleted := &events.TaskDelete{}
		if err := proto.Unmarshal(containerdEvent.Event.Value, deleted); err != nil {
			return workloadmeta.CollectorEvent{}, err
		}

		c.cacheExitInfo(deleted.ContainerID, &deleted.ExitStatus, deleted.ExitedAt)
		return createSetEventFromTask(ctx, containerdEvent.Namespace, containerdEvent, c.containerdClient)
	case TaskStartTopic, TaskOOMTopic, TaskPausedTopic, TaskResumedTopic:
		return createSetEventFromTask(ctx, containerdEvent.Namespace, containerdEvent, c.containerdClient)
	default:
		return workloadmeta.CollectorEvent{}, fmt.Errorf("unknown action type %s, ignoring", containerdEvent.Topic)
	}
}

func createSetEventFromTask(ctx context.Context, namespace string, containerdEvent *containerdevents.Envelope, containerdClient cutil.ContainerdItf) (workloadmeta.CollectorEvent, error) {
	// Notice that the ID field in this case is stored in "ContainerID".
	ID, hasID := containerdEvent.Field([]string{"event", "container_id"})
	if !hasID {
		return workloadmeta.CollectorEvent{}, fmt.Errorf("missing ID in containerd event")
	}

	return createSetEvent(ctx, ID, namespace, containerdClient)
}

func createSetEvent(ctx context.Context, containerID string, namespace string, containerdClient cutil.ContainerdItf) (workloadmeta.CollectorEvent, error) {
	container, err := containerdClient.ContainerWithContext(ctx, containerID)
	if err != nil {
		return workloadmeta.CollectorEvent{}, fmt.Errorf("could not fetch container %s: %s", containerID, err)
	}

	entity, err := buildWorkloadMetaContainer(container, containerdClient)
	if err != nil {
		return workloadmeta.CollectorEvent{}, fmt.Errorf("could not fetch info for container %s: %s", containerID, err)
	}

	// The namespace cannot be obtained from a container instance. That's why we
	// propagate it here using the one in the event.
	entity.Namespace = namespace

	return workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeSet,
		Source: workloadmeta.SourceContainerd,
		Entity: &entity,
	}, nil
}

func createDeletionEvent(containerID string, exitInfo *exitInfo) workloadmeta.CollectorEvent {
	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
	}

	if exitInfo != nil {
		container.State.ExitCode = exitInfo.exitCode
		container.State.FinishedAt = exitInfo.exitTS
	}

	return workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeUnset,
		Source: workloadmeta.SourceContainerd,
		Entity: container,
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build containerd

package containerd

import (
	"context"
	"fmt"

	containerdevents "github.com/containerd/containerd/events"

	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// buildCollectorEvent generates a CollectorEvent from a containerdevents.Envelope
func buildCollectorEvent(ctx context.Context, containerdEvent *containerdevents.Envelope, containerdClient cutil.ContainerdItf) (workloadmeta.CollectorEvent, error) {
	switch containerdEvent.Topic {
	case containerCreationTopic, containerUpdateTopic:
		ID, hasID := containerdEvent.Field([]string{"event", "id"})
		if !hasID {
			return workloadmeta.CollectorEvent{}, fmt.Errorf("missing ID in containerd event")
		}

		return createSetEvent(ctx, ID, containerdClient)
	case containerDeletionTopic:
		ID, hasID := containerdEvent.Field([]string{"event", "id"})
		if !hasID {
			return workloadmeta.CollectorEvent{}, fmt.Errorf("missing ID in containerd event")
		}

		return createDeletionEvent(ID), nil
	case TaskStartTopic, TaskOOMTopic, TaskExitTopic, TaskDeleteTopic, TaskPausedTopic, TaskResumedTopic:
		// Notice that the ID field in this case is stored in "ContainerID".
		ID, hasID := containerdEvent.Field([]string{"event", "container_id"})
		if !hasID {
			return workloadmeta.CollectorEvent{}, fmt.Errorf("missing ID in containerd event")
		}

		return createSetEvent(ctx, ID, containerdClient)
	default:
		return workloadmeta.CollectorEvent{}, fmt.Errorf("unknown action type %s, ignoring", containerdEvent.Topic)
	}
}

func createSetEvent(ctx context.Context, containerID string, containerdClient cutil.ContainerdItf) (workloadmeta.CollectorEvent, error) {
	container, err := containerdClient.ContainerWithContext(ctx, containerID)
	if err != nil {
		return workloadmeta.CollectorEvent{}, fmt.Errorf("could not fetch container %s: %s", containerID, err)
	}

	entity, err := buildWorkloadMetaContainer(container, containerdClient)
	if err != nil {
		return workloadmeta.CollectorEvent{}, fmt.Errorf("could not fetch info for container %s: %s", containerID, err)
	}

	return workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeSet,
		Source: workloadmeta.SourceContainerd,
		Entity: &entity,
	}, nil
}

func createDeletionEvent(containerID string) workloadmeta.CollectorEvent {
	return workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeUnset,
		Source: workloadmeta.SourceContainerd,
		Entity: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
	}
}

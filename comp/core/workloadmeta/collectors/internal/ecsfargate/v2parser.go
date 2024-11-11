// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

// Package ecsfargate implements the ECS Fargate Workloadmeta collector.
package ecsfargate

import (
	"context"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (c *collector) parseTaskFromV2Endpoint(ctx context.Context) ([]workloadmeta.CollectorEvent, error) {
	task, err := c.metaV2.GetTask(ctx)
	if err != nil {
		return nil, err
	}
	return c.parseV2Task(task), nil
}

func (c *collector) parseV2Task(task *v2.Task) []workloadmeta.CollectorEvent {
	events := []workloadmeta.CollectorEvent{}
	seen := make(map[workloadmeta.EntityID]struct{})

	// We only want to collect tasks without a STOPPED status.
	if task.KnownStatus == workloadmeta.ECSTaskKnownStatusStopped {
		return events
	}

	arnParts := strings.Split(task.TaskARN, "/")
	taskID := arnParts[len(arnParts)-1]
	entityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindECSTask,
		ID:   task.TaskARN,
	}

	seen[entityID] = struct{}{}

	taskContainers, containerEvents := c.parseTaskContainers(task, seen)
	taskRegion, taskAccountID := util.ParseRegionAndAWSAccountID(task.TaskARN)
	entity := &workloadmeta.ECSTask{
		EntityID: entityID,
		EntityMeta: workloadmeta.EntityMeta{
			Name: taskID,
		},
		ClusterName:  parseClusterName(task.ClusterName),
		Region:       taskRegion,
		AWSAccountID: taskAccountID,
		Family:       task.Family,
		Version:      task.Version,
		LaunchType:   workloadmeta.ECSLaunchTypeFargate,
		Containers:   taskContainers,

		// the AvailabilityZone metadata is only available for
		// Fargate tasks using platform version 1.4 or later
		AvailabilityZone: task.AvailabilityZone,
	}

	events = append(events, containerEvents...)
	events = append(events, workloadmeta.CollectorEvent{
		Source: workloadmeta.SourceRuntime,
		Type:   workloadmeta.EventTypeSet,
		Entity: entity,
	})

	for seenID := range c.seen {
		if _, ok := seen[seenID]; ok {
			continue
		}

		var entity workloadmeta.Entity
		switch seenID.Kind {
		case workloadmeta.KindECSTask:
			entity = &workloadmeta.ECSTask{EntityID: seenID}
		case workloadmeta.KindContainer:
			entity = &workloadmeta.Container{EntityID: seenID}
		default:
			log.Errorf("cannot handle expired entity of kind %q, skipping", seenID.Kind)
			continue
		}

		events = append(events, workloadmeta.CollectorEvent{
			Type:   workloadmeta.EventTypeUnset,
			Source: workloadmeta.SourceRuntime,
			Entity: entity,
		})
	}

	c.seen = seen

	return events
}

func (c *collector) parseTaskContainers(
	task *v2.Task,
	seen map[workloadmeta.EntityID]struct{},
) ([]workloadmeta.OrchestratorContainer, []workloadmeta.CollectorEvent) {
	taskContainers := make([]workloadmeta.OrchestratorContainer, 0, len(task.Containers))
	events := make([]workloadmeta.CollectorEvent, 0, len(task.Containers))

	for _, container := range task.Containers {
		containerID := container.DockerID
		taskContainers = append(taskContainers, workloadmeta.OrchestratorContainer{
			ID:   containerID,
			Name: container.Name,
		})
		entityID := workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		}

		seen[entityID] = struct{}{}

		image, err := workloadmeta.NewContainerImage(container.ImageID, container.Image)

		if err != nil {
			log.Debugf("cannot split image name %q: %s", container.Image, err)
		}

		ips := make(map[string]string)

		for _, net := range container.Networks {
			if net.NetworkMode == "awsvpc" && len(net.IPv4Addresses) > 0 {
				ips["awsvpc"] = net.IPv4Addresses[0]
			}
		}

		var startedAt time.Time
		if container.StartedAt != "" {
			startedAt, err = time.Parse(time.RFC3339, container.StartedAt)
			if err != nil {
				log.Debugf("cannot parse StartedAt %q for container %q: %s", container.StartedAt, container.DockerID, err)
			}
		}

		var createdAt time.Time
		if container.CreatedAt != "" {
			createdAt, err = time.Parse(time.RFC3339, container.CreatedAt)
			if err != nil {
				log.Debugf("could not parse creation time '%q' for container %q: %s", container.CreatedAt, container.DockerID, err)
			}
		}

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceRuntime,
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:   container.DockerName,
					Labels: container.Labels,
				},
				Image:      image,
				Runtime:    workloadmeta.ContainerRuntimeECSFargate,
				NetworkIPs: ips,
				State: workloadmeta.ContainerState{
					StartedAt: startedAt,
					CreatedAt: createdAt,
					Running:   container.KnownStatus == "RUNNING",
					Status:    parseStatus(container.KnownStatus),
				},
			},
		})
	}

	return taskContainers, events
}

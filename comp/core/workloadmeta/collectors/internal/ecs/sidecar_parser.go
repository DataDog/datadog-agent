// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

// Package ecs implements the ECS Workloadmeta collector.
package ecs

import (
	"context"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// setTaskCollectionParserForSidecar sets the task collection parser for sidecar mode
func (c *collector) setTaskCollectionParserForSidecar() {
	if !c.taskCollectionEnabled {
		log.Infof("detailed task collection disabled, using metadata v2 endpoint")
		c.taskCollectionParser = c.parseTaskFromV2Endpoint
		return
	}

	var err error
	c.metaV4, err = ecsmeta.V4FromCurrentTask()
	if err != nil {
		log.Warnf("failed to initialize metadata v4 client, using metadata v2: %v", err)
		c.taskCollectionParser = c.parseTaskFromV2Endpoint
		return
	}

	log.Infof("detailed task collection enabled, using metadata v4 endpoint")
	c.taskCollectionParser = c.parseTaskFromV4EndpointSidecar
}

// parseTaskFromV2Endpoint parses a single task from the v2 metadata endpoint (Fargate basic metadata)
func (c *collector) parseTaskFromV2Endpoint(ctx context.Context) ([]workloadmeta.CollectorEvent, error) {
	task, err := c.metaV2.GetTask(ctx)
	if err != nil {
		return nil, err
	}
	return c.parseV2TaskWithLaunchType(task), nil
}

// parseTaskFromV4EndpointSidecar parses a single task from the v4 metadata endpoint in sidecar mode
func (c *collector) parseTaskFromV4EndpointSidecar(ctx context.Context) ([]workloadmeta.CollectorEvent, error) {
	task, err := c.metaV4.GetTask(ctx)
	if err != nil {
		return nil, err
	}
	return c.parseV4TaskWithLaunchType(task), nil
}

// parseV2TaskWithLaunchType parses a v2 task and applies the correct launch type
func (c *collector) parseV2TaskWithLaunchType(task *v2.Task) []workloadmeta.CollectorEvent {
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

	taskContainers, containerEvents := c.parseV2TaskContainers(task, seen)
	taskRegion, taskAccountID := util.ParseRegionAndAWSAccountID(task.TaskARN)
	entity := &workloadmeta.ECSTask{
		EntityID: entityID,
		EntityMeta: workloadmeta.EntityMeta{
			Name: taskID,
		},
		ClusterName:  c.parseClusterName(task.ClusterName),
		Region:       taskRegion,
		AWSAccountID: taskAccountID,
		Family:       task.Family,
		Version:      task.Version,
		LaunchType:   c.actualLaunchType, // Use detected launch type, not hardcoded
		Containers:   taskContainers,

		// the AvailabilityZone metadata is only available for
		// Fargate tasks using platform version 1.4 or later
		AvailabilityZone: task.AvailabilityZone,
	}

	// Use appropriate source based on deployment mode
	source := workloadmeta.SourceRuntime
	if c.deploymentMode == deploymentModeDaemon {
		source = workloadmeta.SourceNodeOrchestrator
	}

	events = append(events, containerEvents...)
	events = append(events, workloadmeta.CollectorEvent{
		Source: source,
		Type:   workloadmeta.EventTypeSet,
		Entity: entity,
	})

	// Handle unseen entities
	events = c.handleUnseenEntities(events, seen, source)
	c.seen = seen

	return events
}

// parseV4TaskWithLaunchType parses a v4 task and overrides the launch type if needed
func (c *collector) parseV4TaskWithLaunchType(task *v3or4.Task) []workloadmeta.CollectorEvent {
	events := []workloadmeta.CollectorEvent{}
	seen := make(map[workloadmeta.EntityID]struct{})

	// We only want to collect tasks without a STOPPED status.
	if task.KnownStatus == workloadmeta.ECSTaskKnownStatusStopped {
		return events
	}

	// Parse the task using the shared utility
	taskEvents := util.ParseV4Task(*task, seen)

	// Override the launch type and source for all events
	source := workloadmeta.SourceRuntime
	if c.deploymentMode == deploymentModeDaemon {
		source = workloadmeta.SourceNodeOrchestrator
	}

	for i := range taskEvents {
		// Update source
		taskEvents[i].Source = source

		// Override launch type for ECS tasks
		if ecsTask, ok := taskEvents[i].Entity.(*workloadmeta.ECSTask); ok {
			ecsTask.LaunchType = c.actualLaunchType
		}

		// Override runtime for containers if not Fargate
		if container, ok := taskEvents[i].Entity.(*workloadmeta.Container); ok {
			if c.actualLaunchType == workloadmeta.ECSLaunchTypeEC2 {
				// EC2 sidecar should not set ECSFargate runtime
				container.Runtime = ""
			} else {
				container.Runtime = workloadmeta.ContainerRuntimeECSFargate
			}
		}
	}

	events = append(events, taskEvents...)

	// Handle unseen entities
	events = c.handleUnseenEntities(events, seen, source)
	c.seen = seen

	return events
}

// parseV2TaskContainers extracts containers from a v2 task
func (c *collector) parseV2TaskContainers(
	task *v2.Task,
	seen map[workloadmeta.EntityID]struct{},
) ([]workloadmeta.OrchestratorContainer, []workloadmeta.CollectorEvent) {
	taskContainers := make([]workloadmeta.OrchestratorContainer, 0, len(task.Containers))
	events := make([]workloadmeta.CollectorEvent, 0, len(task.Containers))

	source := workloadmeta.SourceRuntime
	if c.deploymentMode == deploymentModeDaemon {
		source = workloadmeta.SourceNodeOrchestrator
	}

	// Determine container runtime based on actual launch type
	containerRuntime := workloadmeta.ContainerRuntime("")
	if c.actualLaunchType == workloadmeta.ECSLaunchTypeFargate {
		containerRuntime = workloadmeta.ContainerRuntimeECSFargate
	}

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
			Source: source,
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:   container.DockerName,
					Labels: container.Labels,
				},
				Image:      image,
				Runtime:    containerRuntime,
				NetworkIPs: ips,
				State: workloadmeta.ContainerState{
					StartedAt: startedAt,
					CreatedAt: createdAt,
					Running:   container.KnownStatus == "RUNNING",
					Status:    c.parseStatus(container.KnownStatus),
				},
			},
		})
	}

	return taskContainers, events
}

// handleUnseenEntities creates unset events for entities that are no longer present
func (c *collector) handleUnseenEntities(
	events []workloadmeta.CollectorEvent,
	seen map[workloadmeta.EntityID]struct{},
	source workloadmeta.Source,
) []workloadmeta.CollectorEvent {
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
			Source: source,
			Entity: entity,
		})
	}

	return events
}

// parseClusterName returns the short name of a cluster
func (c *collector) parseClusterName(value string) string {
	if strings.Contains(value, "/") {
		parts := strings.Split(value, "/")
		return parts[len(parts)-1]
	}
	return value
}

// parseStatus converts ECS status strings to workloadmeta status
func (c *collector) parseStatus(status string) workloadmeta.ContainerStatus {
	switch status {
	case "RUNNING":
		return workloadmeta.ContainerStatusRunning
	case "STOPPED":
		return workloadmeta.ContainerStatusStopped
	case "PULLED", "CREATED", "RESOURCES_PROVISIONED":
		return workloadmeta.ContainerStatusCreated
	}
	return workloadmeta.ContainerStatusUnknown
}

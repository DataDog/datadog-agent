// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package ecsfargate

import (
	"context"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	collectorID   = "ecs_fargate"
	componentName = "workloadmeta-ecs_fargate"
)

type collector struct {
	store  workloadmeta.Store
	metaV2 *v2.Client
	seen   map[workloadmeta.EntityID]struct{}
}

func init() {
	workloadmeta.RegisterCollector(collectorID, func() workloadmeta.Collector {
		return &collector{
			seen: make(map[workloadmeta.EntityID]struct{}),
		}
	})
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Store) error {
	if !config.IsFeaturePresent(config.ECSFargate) {
		return errors.NewDisabled(componentName, "Agent is not running on Fargate")
	}

	var err error

	c.store = store
	c.metaV2, err = ecsmeta.V2()
	if err != nil {
		return err
	}

	return nil
}

func (c *collector) Pull(ctx context.Context) error {
	task, err := c.metaV2.GetTask(ctx)
	if err != nil {
		return err
	}

	c.store.Notify(c.parseTask(task))

	return nil
}

func (c *collector) parseTask(task *v2.Task) []workloadmeta.CollectorEvent {
	events := []workloadmeta.CollectorEvent{}
	seen := make(map[workloadmeta.EntityID]struct{})

	// We only want to collect tasks without a STOPPED status.
	if task.KnownStatus == "STOPPED" {
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
	entity := &workloadmeta.ECSTask{
		EntityID: entityID,
		EntityMeta: workloadmeta.EntityMeta{
			Name: taskID,
		},
		ClusterName: parseClusterName(task.ClusterName),
		Region:      parseRegion(task.ClusterName),
		Family:      task.Family,
		Version:     task.Version,
		LaunchType:  workloadmeta.ECSLaunchTypeFargate,
		Containers:  taskContainers,

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

// parseClusterName returns the short name of a cluster. it detects if the name
// is an ARN and converts it if that's the case.
func parseClusterName(value string) string {
	if strings.Contains(value, "/") {
		parts := strings.Split(value, "/")
		return parts[len(parts)-1]
	}

	return value
}

// parseRegion tries to parse the region out of a cluster ARN. returns empty if
// it's a malformed ARN.
func parseRegion(clusterARN string) string {
	arnParts := strings.Split(clusterARN, ":")
	if len(arnParts) < 4 {
		return ""
	}
	if arnParts[0] != "arn" || arnParts[1] != "aws" {
		return ""
	}
	region := arnParts[3]

	// Sanity check
	if strings.Count(region, "-") < 2 {
		return ""
	}

	return region
}

func parseStatus(status string) workloadmeta.ContainerStatus {
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

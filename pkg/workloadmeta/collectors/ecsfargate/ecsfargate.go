// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package ecsfargate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/util"
)

const (
	collectorID   = "ecs_fargate"
	componentName = "workloadmeta-ecs_fargate"
	expireFreq    = 15 * time.Second
)

type collector struct {
	store  workloadmeta.Store
	expire *util.Expire
	metaV2 *v2.Client
}

func init() {
	workloadmeta.RegisterCollector(collectorID, func() workloadmeta.Collector {
		return &collector{}
	})
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Store) error {
	if !config.IsFeaturePresent(config.ECSFargate) {
		return errors.NewDisabled(componentName, "Agent is not running on Fargate")
	}

	if !ecsutil.IsFargateInstance(ctx) {
		return fmt.Errorf("failed to connect to ECS Fargate task metadata API")
	}

	var err error

	c.store = store
	c.expire = util.NewExpire(expireFreq)
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

	events := c.parseTask(ctx, task)

	expires := c.expire.ComputeExpires()
	for _, expired := range expires {
		events = append(events, workloadmeta.CollectorEvent{
			Type:   workloadmeta.EventTypeUnset,
			Source: workloadmeta.SourceECSFargate,
			Entity: expired,
		})
	}

	c.store.Notify(events)

	return err
}

func (c *collector) parseTask(ctx context.Context, task *v2.Task) []workloadmeta.CollectorEvent {
	events := []workloadmeta.CollectorEvent{}

	// We only want to collect tasks without a STOPPED status.
	if task.KnownStatus == "STOPPED" {
		return events
	}

	now := time.Now()
	arnParts := strings.Split(task.TaskARN, "/")
	taskID := arnParts[len(arnParts)-1]
	taskContainers, containerEvents := c.parseTaskContainers(task)
	entityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindECSTask,
		ID:   task.TaskARN,
	}

	c.expire.Update(entityID, now)

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
		Source: workloadmeta.SourceECSFargate,
		Type:   workloadmeta.EventTypeSet,
		Entity: entity,
	})

	return events
}

func (c *collector) parseTaskContainers(task *v2.Task) ([]workloadmeta.OrchestratorContainer, []workloadmeta.CollectorEvent) {
	taskContainers := make([]workloadmeta.OrchestratorContainer, 0, len(task.Containers))
	events := make([]workloadmeta.CollectorEvent, 0, len(task.Containers))

	now := time.Now()

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

		c.expire.Update(entityID, now)

		image, err := workloadmeta.NewContainerImage(container.Image)
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

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceECSFargate,
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:   container.DockerName,
					Labels: container.Labels,
				},
				Image:      image,
				Runtime:    workloadmeta.ContainerRuntimeDocker,
				NetworkIPs: ips,
				State: workloadmeta.ContainerState{
					StartedAt: startedAt,
					Running:   container.KnownStatus == "RUNNING",
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

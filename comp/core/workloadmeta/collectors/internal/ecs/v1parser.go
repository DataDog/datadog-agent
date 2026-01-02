// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package ecs

import (
	"context"
	"errors"
	"strings"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/ecs"
	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (c *collector) parseTasksFromV1Endpoint(ctx context.Context) ([]workloadmeta.CollectorEvent, error) {
	tasks, err := c.metaV1.GetTasks(ctx)
	if err != nil {
		return nil, err
	}

	events := []workloadmeta.CollectorEvent{}
	seen := make(map[workloadmeta.EntityID]struct{})

	for _, task := range tasks {
		// We only want to collect tasks without a STOPPED status.
		if task.KnownStatus == workloadmeta.ECSTaskKnownStatusStopped {
			continue
		}

		entityID := workloadmeta.EntityID{
			Kind: workloadmeta.KindECSTask,
			ID:   task.Arn,
		}

		seen[entityID] = struct{}{}

		arnParts := strings.Split(task.Arn, "/")
		taskID := arnParts[len(arnParts)-1]
		taskContainers, containerEvents := c.parseTaskContainers(task, seen)
		taskRegion, taskAccountID := ecs.ParseRegionAndAWSAccountID(task.Arn)

		entity := &workloadmeta.ECSTask{
			EntityID: entityID,
			EntityMeta: workloadmeta.EntityMeta{
				Name: taskID,
			},
			ClusterName:          c.clusterName,
			ContainerInstanceARN: c.containerInstanceARN,
			Family:               task.Family,
			Version:              task.Version,
			Region:               taskRegion,
			AWSAccountID:         taskAccountID,
			LaunchType:           workloadmeta.ECSLaunchTypeEC2,
			Containers:           taskContainers,
		}

		// Only fetch tags if they're both available and used
		if c.hasResourceTags && c.collectResourceTags {
			rt := c.getResourceTags(ctx, entity)
			entity.ContainerInstanceTags = rt.containerInstanceTags
			entity.Tags = rt.tags
		}

		events = append(events, containerEvents...)
		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNodeOrchestrator,
			Type:   workloadmeta.EventTypeSet,
			Entity: entity,
		})
	}

	return c.setLastSeenEntitiesAndUnsetEvents(events, seen), nil
}

func (c *collector) parseTaskContainers(
	task v1.Task,
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

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNodeOrchestrator,
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: container.DockerName,
				},
				State: workloadmeta.ContainerState{
					Status: workloadmeta.ContainerStatusUnknown,
					Health: workloadmeta.ContainerHealthUnknown,
				},
				// Edge Case: Setting the runtime to "docker" causes issues,
				// although it's correct.
				//
				// In ECS, the logs agent assigns the "source" and "service"
				// tags based on the name of the container image. The ECS v1
				// collector does not gather container image information; only
				// the Docker collector does. As a result, the information
				// becomes complete only after the Docker collector has
				// processed the container.
				//
				// If the runtime is set here and the ECS collector runs before
				// the Docker collector, and the logs check configuration is
				// generated before the Docker collector stores the information,
				// the image details will be missing. As a result, the logs
				// configuration will have an incorrect "source" and "service"
				// tags.
				//
				// Setting an empty runtime here is a workaround to ensure that
				// the "source" and "service" tags are correct. The reason is
				// that autodiscovery is not expecting an empty runtime because
				// it uses it to generate the AD identifiers and things like the
				// service ID. Also, the logs agent rejects config with an empty
				// service ID. As a result, with an empty runtime, the logs
				// config will not be created until the Docker collector has run
				// and the image info is available.
				//
				// TODO: Remove this workaround when there's a better way of
				// handling this in AD + logs agent.
				Runtime: "",
			},
		})
	}

	return taskContainers, events
}

// getResourceTags fetches task and container instance tags from the ECS API,
// and caches them for the lifetime of the task, to avoid hitting throttling
// limits from tasks being updated on every pull. Tags won't change in the
// store even if they're changed in the resources themselves, but at least that
// matches the old behavior present in the tagger.
func (c *collector) getResourceTags(ctx context.Context, entity *workloadmeta.ECSTask) resourceTags {
	rt, ok := c.resourceTags[entity.ID]
	if ok {
		return rt
	}

	if len(entity.Containers) == 0 {
		log.Warnf("skip getting resource tags from task %q with zero container", entity.ID)
		return rt
	}

	var metaURI string
	var metaVersion string
	for _, taskContainer := range entity.Containers {
		container, err := c.store.GetContainer(taskContainer.ID)
		if err != nil {
			log.Tracef("cannot find container %q found in task %q: %s", taskContainer.String(false), entity.ID, err)
			continue
		}

		uri, ok := container.EnvVars[v3or4.DefaultMetadataURIv4EnvVariable]
		if ok && uri != "" {
			metaURI = uri
			metaVersion = "v4"
			break
		}

		uri, ok = container.EnvVars[v3or4.DefaultMetadataURIv3EnvVariable]
		if ok && uri != "" {
			metaURI = uri
			metaVersion = "v3"
			break
		}
	}

	if metaURI == "" {
		log.Errorf("failed to get client for metadata v3 or v4 API from task %q and the following containers: %v", entity.ID, entity.Containers)
		return rt
	}

	metaV3orV4 := c.metaV3or4(metaURI, metaVersion)
	taskWithTags, err := metaV3orV4.GetTaskWithTags(ctx)

	if err != nil {
		// If it's a timeout error, log it as debug to avoid spamming the logs as the data can be fetched in next run
		if errors.Is(err, context.DeadlineExceeded) {
			log.Debugf("timeout while getting task with tags from metadata %s API: %s", metaVersion, err)
		} else {
			log.Warnf("failed to get task with tags from metadata %s API: %s", metaVersion, err)
		}
		return rt
	}

	rt = resourceTags{
		tags:                  taskWithTags.TaskTags,
		containerInstanceTags: taskWithTags.ContainerInstanceTags,
	}

	c.resourceTags[entity.ID] = rt

	return rt
}

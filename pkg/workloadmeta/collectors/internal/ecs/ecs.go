// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package ecs

import (
	"context"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/security/log"
	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
	v3or4 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/util"
)

const (
	collectorID   = "ecs"
	componentName = "workloadmeta-ecs"
	expireFreq    = 15 * time.Second
)

type collector struct {
	store           workloadmeta.Store
	expire          *util.Expire
	metaV1          *v1.Client
	clusterName     string
	hasResourceTags bool
	resourceTags    map[string]resourceTags
}

type resourceTags struct {
	tags                  map[string]string
	containerInstanceTags map[string]string
}

func init() {
	workloadmeta.RegisterCollector(collectorID, func() workloadmeta.Collector {
		return &collector{
			resourceTags: make(map[string]resourceTags),
		}
	})
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Store) error {
	if !config.IsFeaturePresent(config.Docker) {
		return errors.NewDisabled(componentName, "Agent is not running on Docker")
	}

	if ecsutil.IsFargateInstance(ctx) {
		return errors.NewDisabled(componentName, "ECS collector is disabled on Fargate")
	}

	var err error

	c.store = store
	c.expire = util.NewExpire(expireFreq)
	c.metaV1, err = ecsmeta.V1()
	if err != nil {
		return err
	}

	c.hasResourceTags = ecsutil.HasEC2ResourceTags()

	instance, err := c.metaV1.GetInstance(ctx)
	if err == nil {
		c.clusterName = instance.Cluster
	} else {
		log.Warnf("cannot determine ECS cluster name: %s", err)
	}

	return nil
}

func (c *collector) Pull(ctx context.Context) error {
	tasks, err := c.metaV1.GetTasks(ctx)
	if err != nil {
		return err
	}

	// we always parse all the tasks coming from the API, as they are not
	// immutable: the list of containers in the task changes as containers
	// don't get added until they actually start running, and killed
	// containers will get re-created.
	events := c.parseTasks(ctx, tasks)

	expires := c.expire.ComputeExpires()
	for _, expired := range expires {
		if c.hasResourceTags && expired.Kind == workloadmeta.KindECSTask {
			delete(c.resourceTags, expired.ID)
		}

		var entity workloadmeta.Entity
		switch expired.Kind {
		case workloadmeta.KindECSTask:
			entity = &workloadmeta.ECSTask{EntityID: expired}
		case workloadmeta.KindContainer:
			entity = &workloadmeta.Container{EntityID: expired}
		default:
			log.Errorf("cannot handle expired entity of kind %q, skipping", expired.Kind)
			continue
		}

		events = append(events, workloadmeta.CollectorEvent{
			Type:   workloadmeta.EventTypeUnset,
			Source: workloadmeta.SourceNodeOrchestrator,
			Entity: entity,
		})
	}

	c.store.Notify(events)

	return err
}

func (c *collector) parseTasks(ctx context.Context, tasks []v1.Task) []workloadmeta.CollectorEvent {
	events := []workloadmeta.CollectorEvent{}

	now := time.Now()

	for _, task := range tasks {
		// We only want to collect tasks without a STOPPED status.
		if task.KnownStatus == "STOPPED" {
			continue
		}

		entityID := workloadmeta.EntityID{
			Kind: workloadmeta.KindECSTask,
			ID:   task.Arn,
		}

		c.expire.Update(entityID, now)

		arnParts := strings.Split(task.Arn, "/")
		taskID := arnParts[len(arnParts)-1]
		taskContainers, containerEvents := c.parseTaskContainers(task)

		entity := &workloadmeta.ECSTask{
			EntityID: entityID,
			EntityMeta: workloadmeta.EntityMeta{
				Name: taskID,
			},
			ClusterName: c.clusterName,
			Family:      task.Family,
			Version:     task.Version,
			LaunchType:  workloadmeta.ECSLaunchTypeEC2,
			Containers:  taskContainers,
		}

		if c.hasResourceTags {
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

	return events
}

func (c *collector) parseTaskContainers(task v1.Task) ([]workloadmeta.OrchestratorContainer, []workloadmeta.CollectorEvent) {
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

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNodeOrchestrator,
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: container.DockerName,
				},
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

	var metaURI string
	var metaVersion string
	for _, taskContainer := range entity.Containers {
		container, err := c.store.GetContainer(taskContainer.ID)
		if err != nil {
			log.Tracef("cannot find container %q found in task %q: %s", taskContainer, entity.ID, err)
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

	metaV3orV4 := v3or4.NewClient(metaURI, metaVersion)
	taskWithTags, err := metaV3orV4.GetTaskWithTags(ctx)
	if err != nil {
		log.Errorf("failed to get task with tags from metadata %s API: %s", metaVersion, err)
		return rt
	}

	rt = resourceTags{
		tags:                  taskWithTags.TaskTags,
		containerInstanceTags: taskWithTags.ContainerInstanceTags,
	}

	c.resourceTags[entity.ID] = rt

	return rt
}

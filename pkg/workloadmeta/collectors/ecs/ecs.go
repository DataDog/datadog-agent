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
	v3 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/util"
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
}

func init() {
	workloadmeta.RegisterCollector(collectorID, func() workloadmeta.Collector {
		return &collector{}
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

	events := c.parseTasks(ctx, tasks)

	expires := c.expire.ComputeExpires()
	for _, expired := range expires {
		events = append(events, workloadmeta.CollectorEvent{
			Type:   workloadmeta.EventTypeUnset,
			Source: workloadmeta.SourceECS,
			Entity: expired,
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

		if created := c.expire.Update(entityID, now); !created {
			// if the task already existed in the store, we don't
			// try to updated to avoid too many calls to the V3
			// metadata API, as it's very easy to hit throttling
			// limits.
			continue
		}

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
			var metaURI string
			for _, taskContainer := range taskContainers {
				container, err := c.store.GetContainer(taskContainer.ID)
				if err != nil {
					log.Tracef("cannot find container %q found in task %q: %s", taskContainer, task.Arn, err)
					continue
				}

				uri, ok := container.EnvVars[v3.DefaultMetadataURIEnvVariable]
				if ok && uri != "" {
					metaURI = uri
					break
				}
			}

			if metaURI != "" {
				metaV3 := v3.NewClient(metaURI)
				taskWithTags, err := metaV3.GetTaskWithTags(ctx)
				if err == nil {
					entity.Tags = taskWithTags.TaskTags
					entity.ContainerInstanceTags = taskWithTags.ContainerInstanceTags
				} else {
					log.Errorf("failed to get task with tags from metadata v3 API: %s", err)

					// forget this task so this gets
					// retried on the next pull. we do
					// still produce an ECSTask with
					// partial data.
					c.expire.Remove(entityID)
				}
			} else {
				log.Errorf("failed to get client for metadata v3 API from task %q and the following containers: %v", task.Arn, taskContainers)

				// forget this task so this gets retried on the
				// next pull. we do still produce an ECSTask
				// with partial data.
				c.expire.Remove(entityID)
			}
		}

		events = append(events, containerEvents...)
		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceECS,
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
			Source: workloadmeta.SourceECS,
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

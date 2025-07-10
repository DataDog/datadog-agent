// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package ecs

import (
	"context"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// parseV4Tasks queries the v4 task endpoint for each task, parses them and stores them in the store.
func (c *collector) parseTasksFromV4Endpoint(ctx context.Context) ([]workloadmeta.CollectorEvent, error) {
	tasks, err := c.metaV1.GetTasks(ctx)
	if err != nil {
		return nil, err
	}

	events := []workloadmeta.CollectorEvent{}
	seen := make(map[workloadmeta.EntityID]struct{})

	taskWorker := newWorker[v3or4.Task](c.taskRateRPS, c.taskRateBurst, c.taskCache, c.getTaskWithTagsFromV4Endpoint)
	processed, rest, skipped := taskWorker.execute(ctx, tasks)

	// parse the processed tasks
	for _, task := range processed {
		events = append(events, util.ParseV4Task(task, seen)...)
	}

	// parse the tasks that were in the queue but not processed due to context cancellation or api errors
	// if the task is not in the cache, convert it to v1 task
	// otherwise only update the last seen entities
	for _, task := range rest {
		if _, found := c.taskCache.Get(task.Arn); !found {
			events = append(events, util.ParseV4Task(v1TaskToV4Task(task), seen)...)
		} else {
			setLastSeenEntity(task, seen)
		}
	}

	// if task is skipped, it means it's already in the cache, update the last seen entities
	for _, task := range skipped {
		setLastSeenEntity(task, seen)
	}

	return c.setLastSeenEntitiesAndUnsetEvents(events, seen), nil
}

// getTaskWithTagsFromV4Endpoint fetches task and tags from the metadata v4 API
func (c *collector) getTaskWithTagsFromV4Endpoint(ctx context.Context, task v1.Task) (v3or4.Task, error) {
	// Get tags from the cache
	rt := c.resourceTags[task.Arn]

	var metaURI string
	for _, taskContainer := range task.Containers {
		containerID := taskContainer.DockerID
		container, err := c.store.GetContainer(containerID)
		if err != nil {
			log.Tracef("cannot find container %q found in task %s: %s", taskContainer, task.Arn, err)
			continue
		}

		uri, ok := container.EnvVars[v3or4.DefaultMetadataURIv4EnvVariable]
		if ok && uri != "" {
			metaURI = uri
			break
		}
	}

	if metaURI == "" {
		err := fmt.Sprintf("failed to get client for metadata v4 API from task %s and the following containers: %v", task.Arn, task.Containers)
		// log this as debug since it's expected that some recent created or deleted tasks won't have containers yet
		log.Debug(err)
		return v1TaskToV4Task(task), errors.New(err)
	}

	var returnTask v3or4.Task
	// If we have tags, are collecting tags and have zero tags present in the cache
	// Call getTaskWithTags
	if c.hasResourceTags && c.collectResourceTags && len(rt.tags) == 0 && len(rt.containerInstanceTags) == 0 {
		// No tags in the cache, fetch from metadata v4 API
		taskWithTags, err := c.metaV3or4(metaURI, "v4").GetTaskWithTags(ctx)
		if err != nil {
			// If it's a timeout error, log it as debug to avoid spamming the logs as the data can be fetched in next run
			if errors.Is(err, context.DeadlineExceeded) {
				log.Debugf("timeout while getting task with tags from metadata v4 API: %s", err)
			} else {
				log.Warnf("failed to get task with tags from metadata v4 API: %s", err)
			}
			return v1TaskToV4Task(task), err
		}

		// Add tags to the cache
		rt = resourceTags{
			tags:                  taskWithTags.TaskTags,
			containerInstanceTags: taskWithTags.ContainerInstanceTags,
		}

		c.resourceTags[task.Arn] = rt
		returnTask = *taskWithTags
	} else {
		// Do not query for tags, but use them if they exist
		taskWithTags, err := c.metaV3or4(metaURI, "v4").GetTask(ctx)
		if err != nil {
			// If it's a timeout error, log it as debug to avoid spamming the logs as the data can be fetched in next run
			if errors.Is(err, context.DeadlineExceeded) {
				log.Debugf("timeout while getting task from metadata v4 API: %s", err)
			} else {
				log.Warnf("failed to get task from metadata v4 API: %s", err)
			}
			return v1TaskToV4Task(task), err
		}
		// Add tags if they exist
		if len(rt.tags) > 0 {
			taskWithTags.TaskTags = rt.tags
		}
		if len(rt.containerInstanceTags) > 0 {
			taskWithTags.ContainerInstanceTags = rt.containerInstanceTags
		}

		returnTask = *taskWithTags
	}

	// Tasks might contain errors behind the scenes in ecs agent
	if len(returnTask.Errors) > 0 {
		for _, err := range returnTask.Errors {
			log.Warnf("error while getting task information from metadata v4 API: %+v", err)
		}
	}

	return returnTask, nil
}

func v1TaskToV4Task(task v1.Task) v3or4.Task {
	result := v3or4.Task{
		TaskARN:       task.Arn,
		DesiredStatus: task.DesiredStatus,
		KnownStatus:   task.KnownStatus,
		Family:        task.Family,
		Version:       task.Version,
		Containers:    make([]v3or4.Container, 0, len(task.Containers)),
	}

	for _, container := range task.Containers {
		result.Containers = append(result.Containers, v3or4.Container{
			Name:       container.Name,
			DockerName: container.DockerName,
			DockerID:   container.DockerID,
		})
	}
	return result
}

func setLastSeenEntity(task v1.Task, seen map[workloadmeta.EntityID]struct{}) {
	entityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindECSTask,
		ID:   task.Arn,
	}
	seen[entityID] = struct{}{}

	for _, container := range task.Containers {
		containerEntityID := workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   container.DockerID,
		}
		seen[containerEntityID] = struct{}{}
	}
}

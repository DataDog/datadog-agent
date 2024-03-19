// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

// Package ecs implements the ECS Workloadmeta collector.
package ecs

import (
	"context"
	"fmt"
	"hash/fnv"
	"reflect"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
	v3or4 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"

	"go.uber.org/fx"
)

const (
	collectorID   = "ecs"
	componentName = "workloadmeta-ecs"
)

type collector struct {
	id                      string
	store                   workloadmeta.Component
	catalog                 workloadmeta.AgentType
	metaV1                  v1.Client
	metaV3or4               func(metaURI, metaVersion string) v3or4.Client
	clusterName             string
	hasResourceTags         bool
	collectResourceTags     bool
	resourceTags            map[string]resourceTags
	seen                    map[workloadmeta.EntityID]struct{}
	v4TaskEnabled           bool
	v4TaskCache             *cache.Cache
	v4TaskRefreshInterval   time.Duration
	v4TaskQueue             []string
	v4TaskNumberLimitPerRun int
	v4TaskRateLimiter       *rate.Limiter
}

type resourceTags struct {
	tags                  map[string]string
	containerInstanceTags map[string]string
}

// NewCollector returns a new ecs collector provider and an error
func NewCollector() (workloadmeta.CollectorProvider, error) {
	v4TaskEnabled := util.Isv4TaskEnabled()
	v4TaskRefreshInterval := config.Datadog.GetDuration("ecs_ec2_task_cache_ttl")
	v4TaskNumberLimitPerRun := config.Datadog.GetInt("ecs_ec2_task_limit_per_run")
	v4TaskRateRPS := config.Datadog.GetInt("ecs_ec2_task_rate")
	v4TaskRateBurst := config.Datadog.GetInt("ecs_ec2_task_burst")

	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:                      collectorID,
			resourceTags:            make(map[string]resourceTags),
			seen:                    make(map[workloadmeta.EntityID]struct{}),
			catalog:                 workloadmeta.NodeAgent | workloadmeta.ProcessAgent,
			v4TaskEnabled:           v4TaskEnabled,
			v4TaskCache:             cache.New(v4TaskRefreshInterval, 30*time.Second),
			v4TaskQueue:             make([]string, 0, 2*v4TaskNumberLimitPerRun),
			v4TaskRefreshInterval:   v4TaskRefreshInterval,
			v4TaskNumberLimitPerRun: v4TaskNumberLimitPerRun,
			v4TaskRateLimiter:       rate.NewLimiter(rate.Every(time.Duration(1/v4TaskRateRPS)*time.Second), v4TaskRateBurst),
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Component) error {
	if !config.IsFeaturePresent(config.ECSEC2) {
		return errors.NewDisabled(componentName, "Agent is not running on ECS EC2")
	}

	var err error

	c.store = store
	c.metaV1, err = ecsmeta.V1()
	if err != nil {
		return err
	}

	// This only exists to allow overriding for testing
	c.metaV3or4 = func(metaURI, metaVersion string) v3or4.Client {
		return v3or4.NewClient(metaURI, metaVersion)
	}

	c.hasResourceTags = ecsutil.HasEC2ResourceTags()
	c.collectResourceTags = config.Datadog.GetBool("ecs_collect_resource_tags_ec2")

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
	if c.v4TaskEnabled {
		c.store.Notify(c.parseV4Tasks(ctx, tasks))
	} else {
		c.store.Notify(c.parseTasks(ctx, tasks))
	}
	return nil
}

func (c *collector) GetID() string {
	return c.id
}

func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}

func (c *collector) parseTasks(ctx context.Context, tasks []v1.Task) []workloadmeta.CollectorEvent {
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

	return c.setLastSeenEntitiesAndUnsetEvents(events, seen)
}

// parseV4Tasks queries the v4 task endpoint for each task, parses them and stores them in the store.
func (c *collector) parseV4Tasks(ctx context.Context, tasks []v1.Task) []workloadmeta.CollectorEvent {
	events := []workloadmeta.CollectorEvent{}
	seen := make(map[workloadmeta.EntityID]struct{})
	// get task ARNs to fetch from the metadata v4 API
	taskArns := c.getTaskArnsToFetch(tasks)

	for _, task := range tasks {
		// We only want to collect tasks without a STOPPED status.
		if task.KnownStatus == workloadmeta.ECSTaskKnownStatusStopped {
			continue
		}

		var v4Task v3or4.Task
		if _, ok := taskArns[task.Arn]; ok {
			v4Task = c.getV4TaskWithTags(ctx, task)
		} else {
			// if the task is not returned by getTaskArnsToFetch, it means it has been fetched during previous runs
			// retrieve it from the cache
			taskCached, found := c.v4TaskCache.Get(task.Arn)
			if found {
				v4Task = *taskCached.(*v3or4.Task)
			} else {
				v4Task = v1TaskToV4Task(task)
			}
		}

		events = append(events, util.ParseV4Task(v4Task, seen)...)
	}

	return c.setLastSeenEntitiesAndUnsetEvents(events, seen)
}

func (c *collector) setLastSeenEntitiesAndUnsetEvents(events []workloadmeta.CollectorEvent, seen map[workloadmeta.EntityID]struct{}) []workloadmeta.CollectorEvent {
	for seenID := range c.seen {
		if _, ok := seen[seenID]; ok {
			continue
		}

		if c.hasResourceTags && seenID.Kind == workloadmeta.KindECSTask {
			delete(c.resourceTags, seenID.ID)
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
			Source: workloadmeta.SourceNodeOrchestrator,
			Entity: entity,
		})
	}

	c.seen = seen
	return events
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
			},
		})
	}

	return taskContainers, events
}

// getV4TaskWithTags fetches task and tasks from the metadata v4 API
func (c *collector) getV4TaskWithTags(ctx context.Context, task v1.Task) v3or4.Task {
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
		log.Errorf("failed to get client for metadata v4 API from task %s and the following containers: %v", task.Arn, task.Containers)
		return v1TaskToV4Task(task)
	}

	err := c.v4TaskRateLimiter.Wait(ctx)
	if err != nil {
		log.Warnf("failed to get task with tags from metadata v4 API: %s", err)
		return v1TaskToV4Task(task)
	}

	taskWithTags, err := getV4TaskWithRetry(ctx, c.metaV3or4(metaURI, "v4"))
	if err != nil {
		log.Warnf("failed to get task with tags from metadata v4 API: %s", err)
		return v1TaskToV4Task(task)
	}

	c.v4TaskCache.Set(task.Arn, taskWithTags, c.v4TaskRefreshInterval+jitter(task.Arn))

	return *taskWithTags
}

func getV4TaskWithRetry(ctx context.Context, metaV3orV4 v3or4.Client) (*v3or4.Task, error) {
	var taskWithTagsRetry retry.Retrier
	var taskWithTags *v3or4.Task
	var err error
	maxRetryCount := 3
	retryCount := 0

	_ = taskWithTagsRetry.SetupRetrier(&retry.Config{
		Name: "get-v4-task-with-tags",
		AttemptMethod: func() error {
			retryCount++
			taskWithTags, err = metaV3orV4.GetTaskWithTags(ctx)
			return err
		},
		Strategy:          retry.Backoff,
		InitialRetryDelay: 250 * time.Millisecond,
		MaxRetryDelay:     1 * time.Second,
	})

	// retry 3 times with exponential backoff strategy: 250ms, 500ms, 1s
	for {
		err = taskWithTagsRetry.TriggerRetry()
		if err == nil || (reflect.ValueOf(err).Kind() == reflect.Ptr && reflect.ValueOf(err).IsNil()) {
			break
		}

		if retry.IsErrPermaFail(err) {
			return nil, err
		}

		if retryCount >= maxRetryCount {
			return nil, fmt.Errorf("failed to get task with tags from metadata v4 API after %d retries", maxRetryCount)
		}
	}
	return taskWithTags, nil
}

// getTaskArnsToFetch returns a list of task ARNs to fetch from the metadata v4 API
// It uses v4TaskCache to know whether a task has been fetched during previous runs
// The length of taskArns is limited by v4TaskNumberLimitPerRun to avoid long time running for a single pull
func (c *collector) getTaskArnsToFetch(tasks []v1.Task) map[string]struct{} {
	taskArns := make(map[string]struct{}, c.v4TaskNumberLimitPerRun)

	for _, task := range tasks {
		if task.KnownStatus == workloadmeta.ECSTaskKnownStatusStopped {
			continue
		}
		c.v4TaskQueue = append(c.v4TaskQueue, task.Arn)
	}

	index := 0
	for _, taskArn := range c.v4TaskQueue {
		if len(taskArns) >= c.v4TaskNumberLimitPerRun {
			break
		}

		// Task is in the queue but not in current running task list
		// It means the task has been stopped, skip it
		if !hasTask(taskArn, tasks) {
			index++
			continue
		}

		// if task is not in the cache or expired, add it
		if _, ok := c.v4TaskCache.Get(taskArn); !ok {
			taskArns[taskArn] = struct{}{}
		}
		index++
	}

	c.v4TaskQueue = c.v4TaskQueue[index:]

	return taskArns
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

	metaV3orV4 := c.metaV3or4(metaURI, metaVersion)
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

var hash32 = fnv.New32a()

func jitter(s string) time.Duration {
	defer hash32.Reset()
	_, err := hash32.Write([]byte(s))
	if err != nil {
		return 0
	}
	second := time.Duration(hash32.Sum32()%61) * time.Second
	return second
}

func hasTask(taskARN string, tasks []v1.Task) bool {
	for _, task := range tasks {
		if task.Arn == taskARN {
			return true
		}
	}
	return false
}

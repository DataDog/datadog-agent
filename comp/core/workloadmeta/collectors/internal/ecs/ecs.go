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
	"time"

	"github.com/patrickmn/go-cache"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
	v3or4 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID   = "ecs"
	componentName = "workloadmeta-ecs"
)

type collector struct {
	id                  string
	store               workloadmeta.Component
	catalog             workloadmeta.AgentType
	metaV1              v1.Client
	metaV3or4           func(metaURI, metaVersion string) v3or4.Client
	clusterName         string
	hasResourceTags     bool
	collectResourceTags bool
	resourceTags        map[string]resourceTags
	seen                map[workloadmeta.EntityID]struct{}
	// taskCollectionEnabled is a flag to enable detailed task collection
	// if the flag is enabled, the collector will query the latest metadata endpoint, currently v4, for each task
	// that is returned from the v1/tasks endpoint
	taskCollectionEnabled bool
	taskCollectionParser  util.TaskParser
	taskCache             *cache.Cache
	taskRateRPS           int
	taskRateBurst         int
}

type resourceTags struct {
	tags                  map[string]string
	containerInstanceTags map[string]string
}

// NewCollector returns a new ecs collector provider and an error
func NewCollector() (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:                    collectorID,
			resourceTags:          make(map[string]resourceTags),
			seen:                  make(map[workloadmeta.EntityID]struct{}),
			catalog:               workloadmeta.NodeAgent | workloadmeta.ProcessAgent,
			taskCollectionEnabled: util.IsTaskCollectionEnabled(),
			taskCache:             cache.New(config.Datadog.GetDuration("ecs_task_cache_ttl"), 30*time.Second),
			taskRateRPS:           config.Datadog.GetInt("ecs_task_collection_rate"),
			taskRateBurst:         config.Datadog.GetInt("ecs_task_collection_burst"),
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

	switch c.detectEndpoint() {
	case "v4":
		c.taskCollectionParser = c.parseTasksFromV4Endpoint
	case "v1":
		c.taskCollectionParser = c.parseTasksFromV1Endpoint
	default:
		return fmt.Errorf("failed to detect ECS metadata endpoint")
	}

	return nil
}

func (c *collector) detectEndpoint() string {
	_, err := ecsmeta.V4FromCurrentTask()
	if c.taskCollectionEnabled && err == nil {
		return "v4"
	}
	return "v1"
}

func (c *collector) Pull(ctx context.Context) error {
	// we always parse all the tasks coming from the API, as they are not
	// immutable: the list of containers in the task changes as containers
	// don't get added until they actually start running, and killed
	// containers will get re-created.
	events, err := c.taskCollectionParser(ctx)
	if err != nil {
		return err
	}
	c.store.Notify(events)
	return nil
}

func (c *collector) GetID() string {
	return c.id
}

func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
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

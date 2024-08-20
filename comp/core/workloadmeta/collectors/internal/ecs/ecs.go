// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

// Package ecs implements the ECS Workloadmeta collector.
package ecs

import (
	"context"
	"time"

	"github.com/patrickmn/go-cache"

	"github.com/DataDog/datadog-agent/comp/core/config"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/wmcatalog/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/errors"
	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID   = "ecs"
	componentName = "workloadmeta-ecs"
)

type collector struct {
	id                  string
	config              config.Component
	store               workloadmeta.Component
	catalog             workloadmeta.AgentType
	metaV1              v1.Client
	metaV3or4           func(metaURI, metaVersion string) v3or4.Client
	clusterName         string
	hasResourceTags     bool
	collectResourceTags bool
	resourceTags        map[string]resourceTags
	seen                map[workloadmeta.EntityID]struct{}
	config              config.Component
	// taskCollectionEnabled is a flag to enable detailed task collection
	// if the flag is enabled, the collector will query the latest metadata endpoint, currently v4, for each task
	// that is returned from the v1/tasks endpoint
	taskCollectionEnabled        bool
	taskCollectionParser         util.TaskParser
	taskCache                    *cache.Cache
	taskRateRPS                  int
	taskRateBurst                int
	metadataRetryInitialInterval time.Duration
	metadataRetryMaxElapsedTime  time.Duration
	metadataRetryTimeoutFactor   int
}

type resourceTags struct {
	tags                  map[string]string
	containerInstanceTags map[string]string
}

// NewCollector returns a new ecs collector
func NewCollector(cfg config.Component) (wmcatalog.Collector, error) {
	return &collector{
		id:                           collectorID,
		config:                       cfg,
		resourceTags:                 make(map[string]resourceTags),
		seen:                         make(map[workloadmeta.EntityID]struct{}),
		catalog:                      workloadmeta.NodeAgent | workloadmeta.ProcessAgent,
		taskCollectionEnabled:        util.IsTaskCollectionEnabled(cfg),
		taskCache:                    cache.New(cfg.GetDuration("ecs_task_cache_ttl"), 30*time.Second),
		taskRateRPS:                  cfg.GetInt("ecs_task_collection_rate"),
		taskRateBurst:                cfg.GetInt("ecs_task_collection_burst"),
		metadataRetryInitialInterval: cfg.GetDuration("ecs_metadata_retry_initial_interval"),
		metadataRetryMaxElapsedTime:  cfg.GetDuration("ecs_metadata_retry_max_elapsed_time"),
		metadataRetryTimeoutFactor:   cfg.GetInt("ecs_metadata_retry_timeout_factor"),
	}, nil
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Component) error {
	if !pkgconfig.IsFeaturePresent(pkgconfig.ECSEC2) {
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
	c.collectResourceTags = c.config.GetBool("ecs_collect_resource_tags_ec2")

	instance, err := c.metaV1.GetInstance(ctx)
	if err == nil {
		c.clusterName = instance.Cluster
	} else {
		log.Warnf("cannot determine ECS cluster name: %s", err)
	}

	c.setTaskCollectionParser(instance.Version)

	return nil
}

func (c *collector) setTaskCollectionParser(version string) {
	if !c.taskCollectionEnabled {
		log.Infof("detailed task collection disabled, using metadata v1 endpoint")
		c.taskCollectionParser = c.parseTasksFromV1Endpoint
		return
	}

	ok, err := ecsmeta.IsMetadataV4Available(util.ParseECSAgentVersion(version))
	if err != nil {
		log.Warnf("detailed task collection enabled but agent cannot determine if v4 metadata endpoint is available, using metadata v1 endpoint: %s", err.Error())
		c.taskCollectionParser = c.parseTasksFromV1Endpoint
		return
	}

	if !ok {
		log.Infof("detailed task collection enabled but v4 metadata endpoint is not available, using metadata v1 endpoint")
		c.taskCollectionParser = c.parseTasksFromV1Endpoint
		return
	}

	log.Infof("detailed task collection enabled, using metadata v4 endpoint")
	c.taskCollectionParser = c.parseTasksFromV4Endpoint
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

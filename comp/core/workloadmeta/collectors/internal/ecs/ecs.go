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

	"github.com/patrickmn/go-cache"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/errors"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID   = "ecs"
	componentName = "workloadmeta-ecs"
)

type deploymentMode string

const (
	deploymentModeDaemon  deploymentMode = "daemon"
	deploymentModeSidecar deploymentMode = "sidecar"
)

type dependencies struct {
	fx.In

	Config config.Component
}

type collector struct {
	id                   string
	store                workloadmeta.Component
	catalog              workloadmeta.AgentType
	metaV1               v1.Client
	metaV2               v2.Client
	metaV4               v3or4.Client
	metaV3or4            func(metaURI, metaVersion string) v3or4.Client
	clusterName          string
	containerInstanceARN string
	hasResourceTags      bool
	collectResourceTags  bool
	resourceTags         map[string]resourceTags
	seen                 map[workloadmeta.EntityID]struct{}
	config               config.Component
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
	// deploymentMode tracks whether agent runs as daemon or sidecar
	deploymentMode deploymentMode
	// actualLaunchType is the actual AWS ECS launch type (ec2 or fargate)
	actualLaunchType workloadmeta.ECSLaunchType
}

type resourceTags struct {
	tags                  map[string]string
	containerInstanceTags map[string]string
}

// NewCollector returns a new ecs collector provider and an error
func NewCollector(deps dependencies) (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:                           collectorID,
			resourceTags:                 make(map[string]resourceTags),
			seen:                         make(map[workloadmeta.EntityID]struct{}),
			catalog:                      workloadmeta.NodeAgent | workloadmeta.ProcessAgent,
			config:                       deps.Config,
			taskCollectionEnabled:        util.IsTaskCollectionEnabled(deps.Config),
			taskCache:                    cache.New(deps.Config.GetDuration("ecs_task_cache_ttl"), 30*time.Second),
			taskRateRPS:                  deps.Config.GetInt("ecs_task_collection_rate"),
			taskRateBurst:                deps.Config.GetInt("ecs_task_collection_burst"),
			metadataRetryInitialInterval: deps.Config.GetDuration("ecs_metadata_retry_initial_interval"),
			metadataRetryMaxElapsedTime:  deps.Config.GetDuration("ecs_metadata_retry_max_elapsed_time"),
			metadataRetryTimeoutFactor:   deps.Config.GetInt("ecs_metadata_retry_timeout_factor"),
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Component) error {
	// Check if running on ECS (EC2 or Fargate)
	if !env.IsFeaturePresent(env.ECSEC2) && !env.IsFeaturePresent(env.ECSFargate) {
		return errors.NewDisabled(componentName, "Agent is not running on ECS")
	}

	c.store = store

	// Determine deployment mode (daemon or sidecar)
	c.deploymentMode = c.determineDeploymentMode()
	log.Infof("ECS collector starting in %s mode", c.deploymentMode)

	// Detect actual launch type from AWS metadata
	c.actualLaunchType = c.detectLaunchType(ctx)
	log.Infof("Detected ECS launch type: %s", c.actualLaunchType)

	// Initialize metadata clients based on deployment mode
	if c.deploymentMode == deploymentModeDaemon {
		return c.initializeDaemonMode(ctx)
	}
	return c.initializeSidecarMode(ctx)
}

func (c *collector) determineDeploymentMode() deploymentMode {
	configMode := c.config.GetString("ecs_deployment_mode")

	switch strings.ToLower(configMode) {
	case "daemon":
		return deploymentModeDaemon
	case "sidecar":
		return deploymentModeSidecar
	case "auto":
		// Auto-detect based on environment
		if env.IsFeaturePresent(env.ECSFargate) {
			// Fargate can only run as sidecar
			return deploymentModeSidecar
		}
		// EC2 defaults to daemon
		return deploymentModeDaemon
	default:
		log.Warnf("Unknown ecs_deployment_mode %q, using auto-detection", configMode)
		// Default to daemon for EC2, sidecar for Fargate
		if env.IsFeaturePresent(env.ECSFargate) {
			return deploymentModeSidecar
		}
		return deploymentModeDaemon
	}
}

func (c *collector) detectLaunchType(ctx context.Context) workloadmeta.ECSLaunchType {
	// First check environment variable
	if env.IsFeaturePresent(env.ECSFargate) {
		return workloadmeta.ECSLaunchTypeFargate
	}

	// For EC2, try to detect from task metadata if running in sidecar mode
	if c.deploymentMode == deploymentModeSidecar {
		// Try to get current task metadata to determine launch type
		if metaV4, err := ecsmeta.V4FromCurrentTask(); err == nil {
			if task, err := metaV4.GetTask(ctx); err == nil && task != nil {
				if strings.ToUpper(task.LaunchType) == "FARGATE" {
					return workloadmeta.ECSLaunchTypeFargate
				}
			}
		}
	}

	// Default to EC2
	return workloadmeta.ECSLaunchTypeEC2
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

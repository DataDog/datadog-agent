// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

// Package ecsfargate implements the ECS Fargate Workloadmeta collector.
package ecsfargate

import (
	"context"
	"strings"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/errors"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID   = "ecs_fargate"
	componentName = "workloadmeta-ecs_fargate"
)

type dependencies struct {
	fx.In

	Config config.Component
}

type collector struct {
	id                    string
	store                 workloadmeta.Component
	catalog               workloadmeta.AgentType
	metaV2                v2.Client
	metaV4                v3or4.Client
	seen                  map[workloadmeta.EntityID]struct{}
	config                config.Component
	taskCollectionEnabled bool
	taskCollectionParser  util.TaskParser
}

// NewCollector returns a new ecsfargate collector provider and an error
func NewCollector(deps dependencies) (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:                    collectorID,
			catalog:               workloadmeta.NodeAgent | workloadmeta.ProcessAgent,
			seen:                  make(map[workloadmeta.EntityID]struct{}),
			config:                deps.Config,
			taskCollectionEnabled: util.IsTaskCollectionEnabled(deps.Config),
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

func (c *collector) Start(_ context.Context, store workloadmeta.Component) error {
	if !env.IsFeaturePresent(env.ECSFargate) {
		return errors.NewDisabled(componentName, "Agent is not running on ECS Fargate")
	}

	c.store = store

	var err error
	c.metaV2, err = ecsmeta.V2()
	if err != nil {
		return err
	}

	c.setTaskCollectionParser()

	return nil
}

func (c *collector) setTaskCollectionParser() {
	if !c.taskCollectionEnabled {
		log.Infof("detailed task collection disabled, using metadata v2 endpoint")
		c.taskCollectionParser = c.parseTaskFromV2Endpoint
		return
	}

	var err error
	c.metaV4, err = ecsmeta.V4FromCurrentTask()
	if err != nil {
		log.Warnf("failed to initialize metadata v4 client, using metdata v2: %v", err)
		c.taskCollectionParser = c.parseTaskFromV2Endpoint
		return
	}

	log.Infof("detailed task collection enabled, using metadata v4 endpoint")
	c.taskCollectionParser = c.parseTaskFromV4Endpoint
}

func (c *collector) Pull(ctx context.Context) error {
	task, err := c.taskCollectionParser(ctx)
	if err != nil {
		return err
	}

	c.store.Notify(task)

	return nil
}

func (c *collector) GetID() string {
	return c.id
}

func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
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

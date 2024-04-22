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
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	pkgConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
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
	if !pkgConfig.IsFeaturePresent(pkgConfig.ECSFargate) {
		return errors.NewDisabled(componentName, "Agent is not running on ECS Fargate")
	}

	c.store = store

	c.setTaskCollectionParser()

	return nil
}

func (c *collector) setTaskCollectionParser() {
	_, err := ecsmeta.V4FromCurrentTask()
	if c.taskCollectionEnabled && err == nil {
		c.taskCollectionParser = c.parseTaskFromV4Endpoint
		return
	}
	c.taskCollectionParser = c.parseTaskFromV2Endpoint
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

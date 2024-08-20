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

	"github.com/DataDog/datadog-agent/comp/core/config"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/wmcatalog/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
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

type collector struct {
	id                    string
	config                config.Component
	store                 workloadmeta.Component
	catalog               workloadmeta.AgentType
	metaV2                v2.Client
	metaV4                v3or4.Client
	seen                  map[workloadmeta.EntityID]struct{}
	config                config.Component
	taskCollectionEnabled bool
	taskCollectionParser  util.TaskParser
}

// NewCollector returns a new ecsfargate collector
func NewCollector(cfg config.Component) (wmcatalog.Collector, error) {
	return &collector{
		id:                    collectorID,
		config:                cfg,
		catalog:               workloadmeta.NodeAgent | workloadmeta.ProcessAgent,
		seen:                  make(map[workloadmeta.EntityID]struct{}),
		taskCollectionEnabled: util.IsTaskCollectionEnabled(cfg),
	}, nil
}

func (c *collector) Start(_ context.Context, store workloadmeta.Component) error {
	if !pkgconfig.IsFeaturePresent(pkgconfig.ECSFargate) {
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

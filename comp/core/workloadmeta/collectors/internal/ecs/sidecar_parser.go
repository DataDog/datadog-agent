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

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// initializeSidecarMode sets up the collector for sidecar deployment mode.
//
// In sidecar mode, the agent runs alongside a single task and monitors only that task.
// This mode uses V2 or V4 metadata API depending on the launch type:
//
//   - Fargate: Uses V2 metadata endpoint (basic task info)
//     See: v2parser.go - parseTaskFromV2Endpoint()
//
//   - EC2 with detailed collection: Uses V4 metadata endpoint (detailed task info)
//     See: v4parser.go - parseTaskFromV4EndpointSidecar()
func (c *collector) initializeSidecarMode(_ context.Context) error {
	var err error

	// Sidecar mode uses v2 or v4 API
	if c.actualLaunchType == workloadmeta.ECSLaunchTypeFargate {
		// Fargate uses v2 API
		c.metaV2, err = ecsmeta.V2()
		if err != nil {
			return err
		}
	}

	// Try to initialize v4 for detailed task collection
	c.setTaskCollectionParserForSidecar()

	return nil
}

// setTaskCollectionParserForSidecar sets up the appropriate task parser for sidecar deployment mode.
//
// In sidecar mode, the agent runs alongside a single task and monitors only that task.
// The parser selection depends on whether detailed task collection is enabled:
//
//   - Disabled or V4 unavailable: Uses V2 metadata endpoint (basic task info)
//     See: v2parser.go - parseTaskFromV2Endpoint()
//
//   - Enabled with V4: Uses V4 metadata endpoint (detailed task info with health, tags, etc.)
//     See: v4parser.go - parseTaskFromV4EndpointSidecar()
func (c *collector) setTaskCollectionParserForSidecar() {
	if !c.taskCollectionEnabled {
		log.Infof("detailed task collection disabled, using metadata v2 endpoint")
		c.taskCollectionParser = c.parseTaskFromV2Endpoint
		return
	}

	var err error
	c.metaV4, err = ecsmeta.V4FromCurrentTask()
	if err != nil {
		log.Warnf("failed to initialize metadata v4 client, using metadata v2: %v", err)
		c.taskCollectionParser = c.parseTaskFromV2Endpoint
		return
	}

	log.Infof("detailed task collection enabled, using metadata v4 endpoint")
	c.taskCollectionParser = c.parseTaskFromV4EndpointSidecar
}

// handleUnseenEntities creates unset events for entities that are no longer present.
// This is used to clean up entities that existed in previous collections but are now gone.
func (c *collector) handleUnseenEntities(
	events []workloadmeta.CollectorEvent,
	seen map[workloadmeta.EntityID]struct{},
	source workloadmeta.Source,
) []workloadmeta.CollectorEvent {
	for seenID := range c.seen {
		if _, ok := seen[seenID]; ok {
			continue
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
			Source: source,
			Entity: entity,
		})
	}

	return events
}

// parseClusterName extracts the short cluster name from an ARN or returns the name as-is.
// Handles formats like: "arn:aws:ecs:region:account:cluster/cluster-name" → "cluster-name"
func (c *collector) parseClusterName(value string) string {
	if strings.Contains(value, "/") {
		parts := strings.Split(value, "/")
		return parts[len(parts)-1]
	}
	return value
}

// parseStatus converts ECS container status strings to workloadmeta ContainerStatus enum.
func (c *collector) parseStatus(status string) workloadmeta.ContainerStatus {
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

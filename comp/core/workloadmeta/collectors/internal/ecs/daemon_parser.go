// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

// Package ecs implements the ECS Workloadmeta collector.
package ecs

import (
	"context"
	"os"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
)

// initializeDaemonMode sets up the collector for daemon deployment mode.
//
// In daemon mode, the agent runs as a daemon on an ECS instance and monitors all tasks on that instance.
// This mode requires V1 metadata API access and uses different parsing strategies based on configuration:
//
//   - V1 parsing: Lists all tasks on the instance (basic info)
//     See: v1parser.go - parseTasksFromV1Endpoint()
//
//   - V4 parsing: Lists tasks via V1, then fetches detailed info from V4 for each task
//     See: v4parser.go - parseTasksFromV4Endpoint()
func (c *collector) initializeDaemonMode(ctx context.Context) error {
	var err error

	// Daemon mode requires v1 API access
	c.metaV1, err = ecsmeta.V1()
	if err != nil {
		return err
	}

	// This only exists to allow overriding for testing
	c.metaV3or4 = func(metaURI, metaVersion string) v3or4.Client {
		return v3or4.NewClient(metaURI, metaVersion, v3or4.WithTryOption(
			c.metadataRetryInitialInterval,
			c.metadataRetryMaxElapsedTime,
			func(d time.Duration) time.Duration { return time.Duration(c.metadataRetryTimeoutFactor) * d }),
		)
	}

	c.hasResourceTags = ecsutil.HasEC2ResourceTags()
	c.collectResourceTags = c.config.GetBool("ecs_collect_resource_tags_ec2")

	instance, err := c.metaV1.GetInstance(ctx)
	if err == nil {
		c.clusterName = instance.Cluster
		c.containerInstanceARN = instance.ContainerInstanceARN
		c.setTaskCollectionParserForDaemon(instance.Version)
	} else {
		log.Warnf("cannot determine ECS cluster name: %s", err)
	}

	return nil
}

// setTaskCollectionParserForDaemon sets up the appropriate task parser for daemon deployment mode.
//
// In daemon mode, the agent monitors all tasks on the instance via the V1 endpoint.
// The parser selection depends on whether detailed task collection is enabled and V4 availability:
//
//   - Disabled or V4 unavailable: Uses V1 metadata endpoint (basic task info)
//     See: v1parser.go - parseTasksFromV1Endpoint()
//
//   - Enabled with V4: Uses V4 metadata endpoint (detailed task info with health, tags, etc.)
//     See: v4parser.go - parseTasksFromV4Endpoint()
func (c *collector) setTaskCollectionParserForDaemon(version string) {
	if !c.taskCollectionEnabled {
		log.Infof("detailed task collection disabled, using metadata v1 endpoint")
		c.taskCollectionParser = c.parseTasksFromV1Endpoint
		return
	}

	ok, err := ecsmeta.IsMetadataV4Available(util.ParseECSAgentVersion(version))
	if err != nil {
		// The managed instances ECS agent returns an empty version string from the v1 introspection
		// endpoint, causing the version check to fail. Fall back to checking for the
		// ECS_CONTAINER_METADATA_URI_V4 env var as a signal that v4 is supported
		if _, hasV4Env := os.LookupEnv(v3or4.DefaultMetadataURIv4EnvVariable); hasV4Env {
			log.Infof("detailed task collection enabled, v4 metadata endpoint available via env var (version check unavailable): using metadata v4 endpoint")
			c.taskCollectionParser = c.parseTasksFromV4Endpoint
			return
		}
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

// setLastSeenEntitiesAndUnsetEvents handles cleanup of entities that are no longer present in daemon mode.
// This is daemon-specific because it manages the resourceTags cache and always uses SourceNodeOrchestrator.
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

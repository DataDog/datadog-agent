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
	"os"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
)

// declare these as vars not const to ease testing: the underlying helpers memoize
// global state and perform network I/O, which unit tests cannot rely on.
//
// These are process-wide mutable globals, so tests that swap them must restore the
// originals and must not call t.Parallel().
var (
	ecsMetaV1                = ecsmeta.V1
	ecsMetaV4FromCurrentTask = ecsmeta.V4FromCurrentTask
	ecsHasEC2ResourceTags    = ecsutil.HasEC2ResourceTags
)

// initializeDaemonMode sets up the collector for daemon deployment mode.
//
// In daemon mode, the agent runs as a daemon on an ECS instance and monitors all tasks on that instance.
// The parsing strategy depends on configuration and on which metadata endpoints are reachable:
//
//   - V1 parsing: Lists all tasks on the instance (basic info)
//     See: v1parser.go - parseTasksFromV1Endpoint()
//
//   - V4 parsing: Lists tasks via V1, then fetches detailed info from V4 for each task
//     See: v4parser.go - parseTasksFromV4Endpoint()
//
//   - V4 /tasks parsing: Fetches all host tasks in a single call from the daemon container's v4 endpoint.
//     Used on ECS Managed Instances where the /tasks endpoint is available.
//     See: daemon_parser.go - parseTasksFromV4TasksEndpoint()
//
// V1 is required by the first two strategies, which read the task list from it. It is not
// required by the third: on ECS Managed Instances the v4 /tasks payload carries everything
// the parser needs, and the v1 introspection endpoint is not guaranteed to be reachable
// from the daemon container. Daemon mode therefore treats v1 as best-effort and only fails
// when no v1-independent strategy is available.
func (c *collector) initializeDaemonMode(ctx context.Context) error {
	// This only exists to allow overriding for testing
	c.metaV3or4 = func(metaURI, metaVersion string) v3or4.Client {
		return v3or4.NewClient(metaURI, metaVersion, v3or4.WithTryOption(
			c.metadataRetryInitialInterval,
			c.metadataRetryMaxElapsedTime,
			func(d time.Duration) time.Duration { return time.Duration(c.metadataRetryTimeoutFactor) * d }),
		)
	}

	// Attempt to initialize a v4 client for the daemon agent's own container.
	// This enables the /tasks endpoint on ECS Managed Instances.
	if v4Client, err := ecsMetaV4FromCurrentTask(); err == nil {
		c.metaV4 = v4Client
	} else {
		log.Debugf("ECS daemon: failed to initialize v4 client for current task (may not be available): %v", err)
	}

	c.hasResourceTags = ecsHasEC2ResourceTags()
	c.collectResourceTags = c.config.GetBool("ecs_collect_resource_tags_ec2")

	var v1Err error
	c.metaV1, v1Err = ecsMetaV1()
	if v1Err != nil {
		log.Warnf("ECS daemon: metadata v1 client unavailable: %v", v1Err)
		// Guard against a non-nil interface wrapping a nil client.
		c.metaV1 = nil
	} else {
		instance, err := c.metaV1.GetInstance(ctx)
		if err != nil {
			v1Err = err
			log.Warnf("cannot determine ECS cluster name: %s", err)
		} else {
			c.clusterName = instance.Cluster
			c.containerInstanceARN = instance.ContainerInstanceARN
			c.setTaskCollectionParserForDaemon(instance.Version)
			return nil
		}
	}

	// No usable v1 instance metadata. The v4 /tasks endpoint on Managed Instances derives
	// cluster identity from each task, so it can run without v1. Select it explicitly
	// rather than going through setTaskCollectionParserForDaemon, which would otherwise
	// pick a v1-backed parser and dereference the nil client on the first Pull.
	if c.taskCollectionEnabled && c.metaV4 != nil &&
		c.actualLaunchType == workloadmeta.ECSLaunchTypeManagedInstances {
		log.Infof("ECS daemon: metadata v1 unavailable, using metadata v4 /tasks endpoint for managed instances")
		c.taskCollectionParser = c.parseTasksFromV4TasksEndpoint
		return nil
	}

	// Returning a retriable error leaves the collector in workloadmeta's candidate set, so
	// Start is retried and picks up the cluster name once the endpoint recovers. This must
	// be a *retry.Error: workloadmeta gates on retry.IsErrWillRetry, which type-asserts
	// rather than unwrapping, so wrapping with %w here would drop the collector for good.
	return &retry.Error{
		LogicError:    fmt.Errorf("ECS daemon mode requires metadata v1: %w", v1Err),
		RessourceName: componentName,
		RetryStatus:   retry.FailWillRetry,
	}
}

// setTaskCollectionParserForDaemon sets up the appropriate task parser for daemon deployment mode.
//
// In daemon mode, the agent monitors all tasks on the instance via the V1 endpoint.
// The parser selection depends on whether detailed task collection is enabled and V4 availability:
//
//   - Disabled or V4 unavailable: Uses V1 metadata endpoint (basic task info)
//     See: v1parser.go - parseTasksFromV1Endpoint()
//
//   - Enabled with V4 on Managed Instances: Uses the v4 /tasks endpoint to fetch all host tasks
//     in a single call. The Group field on each task identifies daemon-scheduled tasks via
//     the "daemon:" prefix, which is mapped to ECSTask.DaemonName.
//     See: daemon_parser.go - parseTasksFromV4TasksEndpoint()
//
//   - Enabled with V4 on EC2: Uses V4 metadata endpoint per-task (detailed task info with health, tags, etc.)
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
			if c.metaV4 != nil && c.actualLaunchType == workloadmeta.ECSLaunchTypeManagedInstances {
				log.Infof("detailed task collection enabled, using metadata v4 /tasks endpoint for managed instances")
				c.taskCollectionParser = c.parseTasksFromV4TasksEndpoint
				return
			}
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

	if c.metaV4 != nil && c.actualLaunchType == workloadmeta.ECSLaunchTypeManagedInstances {
		log.Infof("detailed task collection enabled, using metadata v4 /tasks endpoint for managed instances")
		c.taskCollectionParser = c.parseTasksFromV4TasksEndpoint
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

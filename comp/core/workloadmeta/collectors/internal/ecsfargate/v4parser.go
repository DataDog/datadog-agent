// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

// Package ecsfargate implements the ECS Fargate Workloadmeta collector.
package ecsfargate

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (c *collector) parseTaskFromV4Endpoint(ctx context.Context) ([]workloadmeta.CollectorEvent, error) {
	task, err := c.metaV4.GetTask(ctx)
	if err != nil {
		return nil, err
	}
	return c.parseV4Task(task), nil
}

func (c *collector) parseV4Task(task *v3or4.Task) []workloadmeta.CollectorEvent {
	events := []workloadmeta.CollectorEvent{}
	seen := make(map[workloadmeta.EntityID]struct{})

	// We only want to collect tasks without a STOPPED status.
	if task.KnownStatus == workloadmeta.ECSTaskKnownStatusStopped {
		return events
	}

	events = append(events, util.ParseV4Task(*task, seen)...)

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
			Source: workloadmeta.SourceRuntime,
			Entity: entity,
		})
	}

	c.seen = seen

	return events
}

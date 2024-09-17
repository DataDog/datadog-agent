// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package host implements the host tag Workloadmeta collector.
package host

import (
	"context"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/comp/core/config"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/wmcatalog/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/hosttags"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const id = "host"

type collector struct {
	store        workloadmeta.Component
	catalog      workloadmeta.AgentType
	config       config.Component
	clock        clock.Clock
	timeoutTimer *clock.Timer
}

// NewCollector returns a new host collector
func NewCollector(cfg config.Component) (wmcatalog.Collector, error) {
	return &collector{
		catalog: workloadmeta.NodeAgent | workloadmeta.ProcessAgent,
		config:  cfg,
		clock:   clock.New(),
	}, nil
}

func (c *collector) Start(_ context.Context, store workloadmeta.Component) error {

	c.store = store

	duration := c.config.GetDuration("expected_tags_duration")
	if duration <= 0 {
		return nil
	}

	log.Debugf("Adding host tags to metrics for %v", duration)
	c.timeoutTimer = c.clock.Timer(duration)

	return nil
}

func (c *collector) Pull(ctx context.Context) error {
	// Feature is disabled or timeout has previously occurred
	if c.timeoutTimer == nil {
		return nil
	}

	// Timeout reached - expire any host tags in the store
	if c.resetTimerIfTimedOut() {
		c.store.Notify(makeEvent([]string{}))
		return nil
	}

	tags := hostMetadataUtils.Get(ctx, false, c.config).System
	c.store.Notify(makeEvent(tags))
	return nil
}

func (c *collector) GetID() string {
	return id
}

func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}

func (c *collector) resetTimerIfTimedOut() bool {
	select {
	case <-c.timeoutTimer.C:
		c.timeoutTimer = nil
		return true
	default:
		return false
	}
}

func makeEvent(tags []string) []workloadmeta.CollectorEvent {
	return []workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceHost,
			Entity: &workloadmeta.HostTags{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindHost,
					ID:   id,
				},
				HostTags: tags,
			},
		}}
}

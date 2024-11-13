// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build crio

// Package crio implements the crio Workloadmeta collector.
package crio

import (
	"context"

	"go.uber.org/fx"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/crio"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID           = "crio"
	componentName         = "workloadmeta-crio"
	defaultCrioSocketPath = "/var/run/crio/crio.sock"
)

type collector struct {
	id      string
	client  crio.Client
	store   workloadmeta.Component
	catalog workloadmeta.AgentType
	seen    map[workloadmeta.EntityID]struct{}
}

// NewCollector initializes a new CRI-O collector.
func NewCollector() (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:      collectorID,
			seen:    make(map[workloadmeta.EntityID]struct{}),
			catalog: workloadmeta.NodeAgent | workloadmeta.ProcessAgent,
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

// Start initializes the collector for workloadmeta.
func (c *collector) Start(_ context.Context, store workloadmeta.Component) error {
	if !env.IsFeaturePresent(env.Crio) {
		return dderrors.NewDisabled(componentName, "Crio not detected")
	}
	c.store = store

	criSocket := getCRIOSocketPath()
	client, err := crio.NewCRIOClient(criSocket)
	if err != nil {
		log.Errorf("CRI-O client creation failed for socket %s: %v", criSocket, err)
		client.Close()
		return err
	}
	c.client = client
	return nil
}

// Pull gathers container data.
func (c *collector) Pull(ctx context.Context) error {
	containers, err := c.client.GetAllContainers(ctx)
	if err != nil {
		log.Errorf("Failed to pull container list: %v", err)
		return err
	}

	seen := make(map[workloadmeta.EntityID]struct{})
	events := make([]workloadmeta.CollectorEvent, 0, len(containers))
	for _, container := range containers {
		event := c.convertToEvent(ctx, container)
		seen[event.Entity.GetID()] = struct{}{}
		events = append(events, event)
	}
	for seenID := range c.seen {
		if _, ok := seen[seenID]; !ok {
			events = append(events, generateUnsetEvent(seenID))
		}
	}
	c.seen = seen
	c.store.Notify(events)
	return nil
}

// GetID returns the collector ID.
func (c *collector) GetID() string {
	return c.id
}

// GetTargetCatalog returns the workloadmeta agent type.
func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}

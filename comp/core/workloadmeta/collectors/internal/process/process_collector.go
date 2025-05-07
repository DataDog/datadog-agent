// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package process implements the process collector for Workloadmeta.
package process

import (
	"context"
	"time"

	"github.com/benbjohnson/clock"
	"go.uber.org/fx"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID       = "process-collector"
	componentName     = "workloadmeta-process"
	cacheValidityNoRT = 2 * time.Second
)

type collector struct {
	id      string
	store   workloadmeta.Component
	catalog workloadmeta.AgentType ``

	collectionClock clock.Clock

	// TODO: update to actual type used
	processEventsCh <-chan int
	// TODO: add any other fields you need
}

// NewProcessCollector returns a new process collector provider and an error.
// Currently, this is only used on Linux when language detection and run in core agent are enabled.
func NewProcessCollector() (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:              collectorID,
			catalog:         workloadmeta.NodeAgent,
			collectionClock: clock.New(),
			// TODO: add any other fields you need
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewProcessCollector)
}

// isEnabled returns a boolean indicating if the process collector is enabled and what collection interval to use if it is.
func (c *collector) isEnabled() (bool, time.Duration) {
	// TODO: implement the logic to check if the process collector is enabled based on dependent configs (process collection, language detection, service discovery)
	return false, time.Second * 10
}

// Start starts the collector. The collector should run until the context
// is done. It also gets a reference to the store that started it so it
// can use Notify, or get access to other entities in the store.
func (c *collector) Start(ctx context.Context, store workloadmeta.Component) error {
	// TODO: implement the start-up logic for the process collector
	// Once setup logic is complete, start collection and streaming goroutines
	return nil
}

func (c *collector) collect(ctx context.Context, collectionTicker *clock.Ticker) {
	// TODO: implement the full collection logic for the process collector. Once collection is done, submit events.
	ctx, cancel := context.WithCancel(ctx)
	defer collectionTicker.Stop()
	defer cancel()
	for {
		select {
		case <-collectionTicker.C:
			// Fetch process data and submit events
		case <-ctx.Done():
			log.Infof("The %s collector has stopped", collectorID)
			return
		}
	}
}

func (c *collector) stream(ctx context.Context) {
	// TODO: implement the full streaming logic for the process collector
	health := health.RegisterLiveness(componentName)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		select {
		case <-health.C:

		case <-c.processEventsCh:
		// TODO: implement the logic to handle events
		// c.store.Notify(events)

		case <-ctx.Done():
			err := health.Deregister()
			if err != nil {
				log.Warnf("error de-registering health check: %s", err)
			}
			return
		}
	}
}

// Pull triggers an entity collection. To be used by collectors that
// don't have streaming functionality, and called periodically by the
// store. This is not needed for the process collector.
func (c *collector) Pull(_ context.Context) error {
	return nil
}

// GetID returns the identifier for the respective component.
func (c *collector) GetID() string {
	return c.id
}

// GetTargetCatalog gets the expected catalog.
func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}

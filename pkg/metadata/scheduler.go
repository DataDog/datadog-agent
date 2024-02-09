// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

// Catalog keeps track of metadata collectors by name
var catalog = make(map[string]Collector)

var (
	// For testing purposes
	newTimer = time.NewTimer
)

type scheduledCollector struct {
	sendTimer    *time.Timer
	healthHandle *health.Handle
}

// Scheduler takes care of sending metadata at specific
// time intervals
type Scheduler struct {
	demux         aggregator.Demultiplexer
	collectors    map[string]*scheduledCollector
	context       context.Context
	contextCancel context.CancelFunc
}

// NewScheduler builds and returns a new Metadata Scheduler
func NewScheduler(demux aggregator.Demultiplexer) *Scheduler {
	scheduler := &Scheduler{
		demux:      demux,
		collectors: make(map[string]*scheduledCollector),
	}

	scheduler.context, scheduler.contextCancel = context.WithCancel(context.Background())

	return scheduler
}

// Stop scheduling collectors
func (c *Scheduler) Stop() {
	c.contextCancel()
	for _, sc := range c.collectors {
		sc.sendTimer.Stop()
		sc.healthHandle.Deregister() //nolint:errcheck
	}
}

// AddCollector schedules a Metadata Collector at the given interval
func (c *Scheduler) AddCollector(name string, interval time.Duration) error {
	if c.IsScheduled(name) {
		return fmt.Errorf("trying to schedule %s twice", name)
	}

	p, found := catalog[name]
	if !found {
		return fmt.Errorf("Unable to find metadata collector: %s", name)
	}

	firstInterval := interval
	if withFirstRun, ok := p.(CollectorWithFirstRun); ok {
		firstInterval = withFirstRun.FirstRunInterval()
	}

	sc := &scheduledCollector{
		sendTimer:    newTimer(firstInterval),
		healthHandle: health.RegisterLiveness("metadata-" + name),
	}

	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		for {
			select {
			case <-c.context.Done():
				cancel()
				return
			case healthDeadline := <-sc.healthHandle.C:
				cancel()
				ctx, cancel = context.WithDeadline(context.Background(), healthDeadline)
			case <-sc.sendTimer.C:
				sc.sendTimer.Reset(interval) // Reset the timer, so it fires again after `interval`.
				// Note we call `p.Send` on the collector *after* resetting the Timer, so
				// the time spent by `p.Send` is not added to the total time between runs.
				if err := p.Send(ctx, c.demux.Serializer()); err != nil {
					log.Errorf("Unable to send '%s' metadata: %v", name, err)
				}
			}
		}
	}()
	c.collectors[name] = sc

	if withInit, ok := p.(CollectorWithInit); ok {
		withInit.Init() //nolint:errcheck
	}

	return nil
}

// IsScheduled returns wether a given Collector has been added to this Scheduler
func (c *Scheduler) IsScheduled(name string) bool {
	_, found := c.collectors[name]
	return found
}

// TriggerAndResetCollectorTimer runs a collector manually, on demand. The delay
// parameter can be set to 0 to run it immediately, or to a duration after which
// the collector will run. The runs at regular intervals of the collector will
// be resumed *after* this manual run. Calling TriggerAndResetCollectorTimer on
// a stopped Scheduler does nothing, since the goroutine that waits on the Timer
// will not be running.
func (c *Scheduler) TriggerAndResetCollectorTimer(name string, delay time.Duration) {
	sc, found := c.collectors[name]

	if !found {
		log.Errorf("Unable to find '" + name + "' in the running metadata collectors!")
	}

	if !sc.sendTimer.Stop() {
		// Drain the channel after stoping, as per Timer's documentation
		// Explanation here: https://blogtitle.github.io/go-advanced-concurrency-patterns-part-2-timers/
		select {
		case <-sc.sendTimer.C:
		default: //So we don't block if the channel is empty
		}
	}

	sc.sendTimer.Reset(delay)
}

// RegisterCollector adds a Metadata Collector to the catalog
func RegisterCollector(name string, metadataCollector Collector) {
	catalog[name] = metadataCollector
}

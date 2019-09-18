// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package metadata

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

// Catalog keeps track of metadata collectors by name
var catalog = make(map[string]Collector)

var (
	// For testing purposes
	newTimer = time.NewTimer
	firstRun = true
)

type scheduledCollector struct {
	sendTimer    *time.Timer
	healthHandle *health.Handle
}

// Scheduler takes care of sending metadata at specific
// time intervals
type Scheduler struct {
	srl           *serializer.Serializer
	collectors    map[string]*scheduledCollector
	context       context.Context
	contextCancel context.CancelFunc
}

// NewScheduler builds and returns a new Metadata Scheduler
func NewScheduler(s *serializer.Serializer) *Scheduler {
	scheduler := &Scheduler{
		srl:        s,
		collectors: make(map[string]*scheduledCollector),
	}

	if firstRun {
		err := scheduler.firstRun()
		if err != nil {
			log.Errorf("Unable to send host metadata at first run: %v", err)
		}
	}

	scheduler.context, scheduler.contextCancel = context.WithCancel(context.Background())

	return scheduler
}

// Stop scheduling collectors
func (c *Scheduler) Stop() {
	c.contextCancel()
	for _, sc := range c.collectors {
		sc.sendTimer.Stop()
		sc.healthHandle.Deregister()
	}
}

// AddCollector schedules a Metadata Collector at the given interval
func (c *Scheduler) AddCollector(name string, interval time.Duration) error {
	p, found := catalog[name]
	if !found {
		return fmt.Errorf("Unable to find metadata collector: %s", name)
	}

	sc := &scheduledCollector{
		sendTimer:    newTimer(interval),
		healthHandle: health.Register("metadata-" + name),
	}

	go func() {
		ctx, cancelCtxFunc := context.WithCancel(c.context)
		defer cancelCtxFunc()
		for {
			select {
			case <-ctx.Done():
				return
			case <-sc.healthHandle.C:
				// Purposely empty
			case <-sc.sendTimer.C:
				sc.sendTimer.Reset(interval) // Reset the timer, so it fires again after `interval`.
				// Note we call `p.Send` on the collector *after* resetting the Timer, so
				// the time spent by `p.Send` is not added to the total time between runs.
				if err := p.Send(c.srl); err != nil {
					log.Errorf("Unable to send '%s' metadata: %v", name, err)
				}
			}
		}
	}()
	c.collectors[name] = sc

	return nil
}

// SendNow runs a collector immediately and resets its ticker.
// Does nothing if the Scheduler has been stopped, since the
// goroutine that listens on the Timer will not be running.
func (c *Scheduler) SendNow(name string) {
	sc, found := c.collectors[name]

	if !found {
		log.Errorf("Unable to find '" + name + "' in the running metadata collectors!")
	}

	if !sc.sendTimer.Stop() {
		// Drain the channel after stoping, as per Timer's documentation
		// Explanation here: https://blogtitle.github.io/go-advanced-concurrency-patterns-part-2-timers/
		<-sc.sendTimer.C
	}

	sc.sendTimer.Reset(0) // Fire immediately
}

// Always send host metadata at the first run
func (c *Scheduler) firstRun() error {
	p, found := catalog["host"]
	if !found {
		log.Error("Unable to find 'host' metadata collector in the catalog!")
		signals.ErrorStopper <- true
	}
	return p.Send(c.srl)
}

// RegisterCollector adds a Metadata Collector to the catalog
func RegisterCollector(name string, metadataCollector Collector) {
	catalog[name] = metadataCollector
}

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
	newTicker = time.NewTicker
)

type scheduledCollector struct {
	interval     time.Duration
	sendTicker   *time.Ticker
	healthHandle *health.Handle
	forceC       chan bool
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

	err := scheduler.firstRun()
	if err != nil {
		log.Errorf("Unable to send host metadata at first run: %v", err)
	}

	scheduler.context, scheduler.contextCancel = context.WithCancel(context.Background())

	return scheduler
}

// Stop scheduling collectors
func (c *Scheduler) Stop() {
	c.contextCancel()
	for _, sc := range c.collectors {
		sc.sendTicker.Stop()
		sc.healthHandle.Deregister()
		sc.forceC = nil //No need to close it, will be GC'd once the goroutine using it ends
	}
}

// AddCollector schedules a Metadata Collector at the given interval
func (c *Scheduler) AddCollector(name string, interval time.Duration) error {
	p, found := catalog[name]
	if !found {
		return fmt.Errorf("Unable to find metadata collector: %s", name)
	}

	sc := &scheduledCollector{
		interval:     interval,
		sendTicker:   newTicker(interval),
		healthHandle: health.Register("metadata-" + name),
		forceC:       make(chan bool, 1),
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
			case <-sc.sendTicker.C:
				if err := p.Send(c.srl); err != nil {
					log.Errorf("Unable to send '%s' metadata: %v", name, err)
				}
			case <-sc.forceC:
				if err := p.Send(c.srl); err != nil {
					log.Errorf("Unable to send '%s' metadata: %v", name, err)
				}
			}
		}
	}()
	c.collectors[name] = sc

	return nil
}

// SendNow runs a collector immediately and resets its ticker
func (c *Scheduler) SendNow(name string) {
	sc, found := c.collectors[name]

	if !found {
		log.Errorf("Unable to find '" + name + "' metadata collector in the catalog!")
	}

	if sc.forceC == nil {
		log.Debugf("Ignoring SendNow for '" + name + "', looks like the Scheduler has been stopped.")
	}

	// There is no function to reset a ticker. We have to Stop it and create a new one.
	sc.sendTicker.Stop()
	sc.sendTicker = newTicker(sc.interval)

	// Signal *after* reseting the ticker so the goroutine picks up the new ticker on the next iteration.
	sc.forceC <- true
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

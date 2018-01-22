// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package metadata

import (
	"fmt"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

// Catalog keeps track of metadata collectors by name
var catalog = make(map[string]Collector)

// Scheduler takes care of sending metadata at specific
// time intervals
type Scheduler struct {
	srl          *serializer.Serializer
	hostname     string
	tickers      []*time.Ticker
	healthTokens []health.ID
}

// NewScheduler builds and returns a new Metadata Scheduler
func NewScheduler(s *serializer.Serializer, hostname string) *Scheduler {
	scheduler := &Scheduler{
		srl:      s,
		hostname: hostname,
	}

	err := scheduler.firstRun()
	if err != nil {
		log.Errorf("Unable to send host metadata at first run: %v", err)
	}

	return scheduler
}

// Stop scheduling collectors
func (c *Scheduler) Stop() {
	for _, t := range c.tickers {
		t.Stop()
	}
	for _, t := range c.healthTokens {
		health.Deregister(t)
	}
}

// AddCollector schedules a Metadata Collector at the given interval
func (c *Scheduler) AddCollector(name string, interval time.Duration) error {
	p, found := catalog[name]
	if !found {
		return fmt.Errorf("Unable to find metadata collector: %s", name)
	}

	sendTicker := time.NewTicker(interval)
	healthTicker := time.NewTicker(15 * time.Second)
	healthToken := health.Register("metadata-" + name)

	go func() {
		for {
			select {
			case <-healthTicker.C:
				health.Ping(healthToken)
			case <-sendTicker.C:
				p.Send(c.srl)
			}
		}
	}()
	c.tickers = append(c.tickers, sendTicker)
	c.tickers = append(c.tickers, healthTicker)
	c.healthTokens = append(c.healthTokens, healthToken)

	return nil
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

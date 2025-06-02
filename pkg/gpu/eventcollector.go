// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type eventCollector struct {
	enabled           bool
	mtx               sync.Mutex
	maxEventsToRecord int
	eventStore        [][]byte
	finished          chan struct{}
}

func newEventCollector() *eventCollector {
	return &eventCollector{
		finished: make(chan struct{}),
	}
}

func (c *eventCollector) tryRecordEvent(event any) {
	if !c.enabled {
		return
	}

	// Lock *after* checking if we're enabled. A bool read/write is atomic (fits
	// in one word) so we are not going to an inconsistent value, which means we don't
	// really need to take an expensive lock or use atomic operations, as this
	// function can be called in the hot path. The only problem we could have
	// is that we might see the change in the "enabled" flag before we see the other
	// fields updated, but as we have the lock here which is also taken by enable()
	// we ensure that we see the consistent state of the struct.`
	c.mtx.Lock()
	defer c.mtx.Unlock()

	marshaled, err := json.Marshal(event)
	if err != nil {
		if logLimitProbe.ShouldLog() {
			log.Warnf("Failed to marshal event: %v", err)
		}
		return
	}

	c.eventStore = append(c.eventStore, marshaled)
	if len(c.eventStore) >= c.maxEventsToRecord {
		// Tell ourselves we are done (we are the only ones looking at this variable now)
		c.enabled = false

		// Tell the listener we are done. Do not close the channel as we might reuse it later
		c.finished <- struct{}{}
	}
}

func (c *eventCollector) enable(maxEvents int) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.maxEventsToRecord = maxEvents
	c.eventStore = make([][]byte, 0, maxEvents)

	c.enabled = true
}

func (c *eventCollector) wait(ctx context.Context) ([][]byte, error) {
outer:
	for {
		select {
		case <-c.finished:
			break outer
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Lock to ensure we see the consistent state of the struct (likely unnecessary, but better safe than sorry)
	c.mtx.Lock()
	defer c.mtx.Unlock()

	store := c.eventStore
	c.eventStore = nil // ensure we're not hanging on to the data

	return store, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry provides a way to collect metrics from eBPF programs.
package telemetry

import (
	"sync"
	"time"
)

const stateTTL = 5 * time.Minute

type deltaCalculator struct {
	mux             sync.Mutex
	stateByClientID map[string]*clientState
}

// GetState returns the state for the given clientID.
func (d *deltaCalculator) GetState(clientID string) *clientState {
	d.mux.Lock()
	defer d.mux.Unlock()

	// lazily initialize everything so the zero value can be used
	if d.stateByClientID == nil {
		d.stateByClientID = make(map[string]*clientState)
	}

	state := d.stateByClientID[clientID]
	if state == nil {
		state = &clientState{prevValues: make(map[string]int64)}
		d.stateByClientID[clientID] = state
	}

	state.lastSeen = time.Now()
	d.clean()
	return state
}

func (d *deltaCalculator) clean() {
	now := time.Now()
	for clientID, state := range d.stateByClientID {
		if now.Sub(state.lastSeen) > stateTTL {
			delete(d.stateByClientID, clientID)
		}
	}
}

type clientState struct {
	mux        sync.Mutex
	prevValues map[string]int64
	lastSeen   time.Time
}

// ValueFor returns the delta between the current value of the metric and the previous one.
func (c *clientState) ValueFor(m metric) int64 {
	base := m.base()
	if _, ok := m.(*Gauge); ok {
		// If metric is of type `*Gauge` we return its value as it is
		return base.Get()
	}

	name := base.Name()
	current := base.Get()

	c.mux.Lock()
	defer c.mux.Unlock()

	// If the metric is of type `*Counter` we calculate the delta
	prev := c.prevValues[name]
	c.prevValues[name] = current
	return current - prev
}

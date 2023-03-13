// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const stateTTL = 5 * time.Minute

type deltaCalculator struct {
	mux             sync.Mutex
	stateByClientID map[string]*clientState
}

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

func (c *clientState) ValueFor(m *Metric) int64 {
	if !contains(OptMonotonic, m.opts) {
		return m.Get()
	}

	name := m.Name()
	current := m.Get()

	c.mux.Lock()
	defer c.mux.Unlock()

	prev := c.prevValues[name]
	if prev > current {
		// let the library client know if this metric is being misconfigured
		log.Debugf(
			"error: metric %q is not growing monotonically but it was instantiated with `OptMonotonic`",
			name,
		)
	}

	c.prevValues[name] = current
	return current - prev
}

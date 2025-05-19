// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package health implements the internal healthcheck
package health

import (
	"errors"
	"sync"
	"time"
)

var pingFrequency = 15 * time.Second
var bufferSize = 2

// Handle holds the token and the channel for components to use
type Handle struct {
	C <-chan time.Time
}

// Deregister allows a component to easily deregister itself
func (h *Handle) Deregister() error {
	return Deregister(h)
}

type component struct {
	name       string
	healthChan chan time.Time
	healthy    bool
	// if set to true, once the check is healthy, we mark it as healthy forever and we stop checking it
	once bool
}

type catalog struct {
	sync.RWMutex
	components map[*Handle]*component
	latestRun  time.Time
}

func newCatalog() *catalog {
	return &catalog{
		components: make(map[*Handle]*component),
		latestRun:  time.Now(), // Start healthy
	}
}

// register a component with the default 30 seconds timeout, returns a token
func (c *catalog) register(name string, options ...Option) *Handle {
	c.Lock()
	defer c.Unlock()

	if len(c.components) == 0 {
		go c.run()
	}

	component := &component{
		name:       name,
		healthChan: make(chan time.Time, bufferSize),
		healthy:    false,
	}

	for _, option := range options {
		option(component)
	}

	h := &Handle{
		C: component.healthChan,
	}

	// Start with a full channel to component is unhealthy until its first read
	for i := 0; i < bufferSize; i++ {
		component.healthChan <- time.Now().Add(pingFrequency)
	}

	c.components[h] = component
	return h
}

// run is the healthcheck goroutine that triggers a ping every 15 sec
// it must be started when the first component registers, and will
// return if no components are registered anymore
func (c *catalog) run() {
	pingTicker := time.NewTicker(pingFrequency)

	for {
		t := <-pingTicker.C
		empty := c.pingComponents(t.Add(mulDuration(pingFrequency, bufferSize)))
		if empty {
			break
		}
	}
	pingTicker.Stop()
}

func mulDuration(d time.Duration, x int) time.Duration {
	return time.Duration(int64(d) * int64(x))
}

// pingComponents is the actual pinging logic, separated for unit tests.
// Returns true if the component list is empty, to make the pooling logic stop.
func (c *catalog) pingComponents(healthDeadline time.Time) bool {
	c.Lock()
	defer c.Unlock()
	for _, component := range c.components {
		// We skip components that are registered to be skipped once they pass once
		if component.healthy && component.once {
			continue
		}
		select {
		case component.healthChan <- healthDeadline:
			component.healthy = true
		default:
			component.healthy = false
		}
	}
	c.latestRun = time.Now()
	return len(c.components) == 0
}

// deregister a component from the healthcheck
func (c *catalog) deregister(handle *Handle) error {
	c.Lock()
	defer c.Unlock()
	if _, found := c.components[handle]; !found {
		return errors.New("component not registered")
	}
	close(c.components[handle].healthChan)
	delete(c.components, handle)
	return nil
}

// Status represents the current status of registered components
// it is built and returned by GetStatus()
type Status struct {
	Healthy   []string
	Unhealthy []string
}

// getStatus allows to query the health status of the agent
func (c *catalog) getStatus() Status {
	status := Status{}
	c.RLock()
	defer c.RUnlock()

	// If no component registered, do not check anything, not even the checker itself
	// as the `run()` function exits in such a case.
	if len(c.components) == 0 {
		return status
	}

	// Test the checker itself
	if time.Now().After(c.latestRun.Add(2 * pingFrequency)) {
		status.Unhealthy = append(status.Unhealthy, "healthcheck")
	} else {
		status.Healthy = append(status.Healthy, "healthcheck")
	}

	// Check components
	for _, component := range c.components {
		if component.healthy {
			status.Healthy = append(status.Healthy, component.name)
		} else {
			status.Unhealthy = append(status.Unhealthy, component.name)
		}
	}
	return status
}

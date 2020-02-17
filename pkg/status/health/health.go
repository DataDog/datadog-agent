// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

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
	C <-chan struct{}
}

// Deregister allows a component to easily deregister itself
func (h *Handle) Deregister() error {
	return Deregister(h)
}

type component struct {
	name       string
	healthChan chan struct{}
	healthy    bool
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
func (c *catalog) register(name string) *Handle {
	c.Lock()
	defer c.Unlock()

	if len(c.components) == 0 {
		go c.run()
	}

	component := &component{
		name:       name,
		healthChan: make(chan struct{}, bufferSize),
		healthy:    false,
	}
	h := &Handle{
		C: component.healthChan,
	}

	// Start with a full channel to component is unhealthy until its first read
	for i := 0; i < bufferSize; i++ {
		component.healthChan <- struct{}{}
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
		<-pingTicker.C
		empty := c.pingComponents()
		if empty {
			break
		}
	}
	pingTicker.Stop()
}

// pingComponents is the actual pinging logic, separated for unit tests.
// Returns true if the component list is empty, to make the pooling logic stop.
func (c *catalog) pingComponents() bool {
	c.Lock()
	defer c.Unlock()
	for _, component := range c.components {
		select {
		case component.healthChan <- struct{}{}:
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

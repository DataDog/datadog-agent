// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package health

import (
	"errors"
	"sync"
	"time"
)

// Handle holds the token and the channel for components to use
type Handle struct {
	C <-chan struct{}
}

func (h *Handle) Deregister() error {
	return Deregister(h)
}

// Status represents the current status of registered components
// it is built and returned by GetStatus()
type Status struct {
	Healthy   []string
	Unhealthy []string
}

type component struct {
	name       string
	healthChan chan struct{}
	healthy    bool
}

type catalog struct {
	sync.RWMutex
	components map[*Handle]*component
}

func newCatalog() *catalog {
	return &catalog{
		components: make(map[*Handle]*component),
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
		healthChan: make(chan struct{}, 2),
		healthy:    false,
	}
	h := &Handle{
		C: component.healthChan,
	}

	c.components[h] = component
	return h
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

func (c *catalog) run() {
	pingTicker := time.NewTicker(15 * time.Second)

	for {
		<-pingTicker.C
		c.Lock()
		if len(c.components) == 0 {
			break
		}
		c.pingComponents()
		c.Unlock()
	}
	pingTicker.Stop()
}

func (c *catalog) pingComponents() {
	for _, component := range c.components {
		select {
		case component.healthChan <- struct{}{}:
			component.healthy = true
		default:
			component.healthy = false
		}
	}
}

// getStatus allows to query the health status of the agent
func (c *catalog) getStatus() Status {
	status := Status{}
	c.RLock()
	defer c.RUnlock()

	for _, component := range c.components {
		if component.healthy {
			status.Healthy = append(status.Healthy, component.name)
		} else {
			status.Unhealthy = append(status.Unhealthy, component.name)
		}
	}
	return status
}

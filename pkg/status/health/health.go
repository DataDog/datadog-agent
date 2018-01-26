// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package health

import (
	"fmt"
	"sync"
	"time"

	log "github.com/cihub/seelog"
)

// DefaultPingFreq holds the preferred time between two pings
const DefaultPingFreq = 15 * time.Second

// DefaultTimeout holds the duration used for default (twice DefaultPingFreq)
const DefaultTimeout = 30 * time.Second

// ID objects are returned when registering and are to be used when pinging
type ID string

// Status represents the current status of registered components
type Status struct {
	Healthy   []string
	Unhealthy []string
}

type component struct {
	name       string
	timeout    time.Duration
	latestPing time.Time
}

type componentCatalog struct {
	sync.RWMutex
	components map[ID]*component
}

var catalog = componentCatalog{
	components: make(map[ID]*component),
}

// Register a component with the default 30 seconds timeout, returns a token
func Register(name string) ID {
	return RegisterWithCustomTimeout(name, DefaultTimeout)
}

// RegisterWithCustomTimeout allows to register with a custom timeout duration
func RegisterWithCustomTimeout(name string, timeout time.Duration) ID {
	catalog.Lock()
	defer catalog.Unlock()

	id := ID(name)
	_, taken := catalog.components[id]
	if taken {
		for n := 2; n < 100; n++ {
			// Loop to 99 to avoid introducing an infinite loop.
			newid := ID(fmt.Sprintf("%s-%d", name, n))
			_, taken = catalog.components[newid]
			if !taken {
				id = newid
				break
			}
		}
		// The case is improbable though, so we errorf and continue
		if taken {
			log.Errorf("Failed to find a unique token for component %s", name)
		}
	}

	catalog.components[id] = &component{
		name:       name,
		timeout:    timeout,
		latestPing: time.Now().Add(-2 * timeout), // Register as unhealthy by default
	}

	return id
}

// Deregister a component from the healthcheck
func Deregister(token ID) error {
	catalog.Lock()
	defer catalog.Unlock()
	if _, found := catalog.components[token]; !found {
		return fmt.Errorf("component %s not registered", token)
	}
	delete(catalog.components, token)
	return nil
}

// Ping is to be called regularly by component to signal they are still healthy
func Ping(token ID) error {
	return registerPing(token, time.Now())
}

// registerPing is private and used for unit testing
func registerPing(token ID, timestamp time.Time) error {
	catalog.Lock()
	defer catalog.Unlock()
	c, found := catalog.components[token]
	if !found {
		return fmt.Errorf("component %s not registered", token)
	}
	c.latestPing = timestamp
	return nil
}

// GetStatus allows to query the health status of the agent
func GetStatus() Status {
	status := Status{}
	now := time.Now()

	catalog.RLock()
	defer catalog.RUnlock()

	for _, c := range catalog.components {
		if c.latestPing.IsZero() {
			log.Warnf("Error processing component %q, considering it unhealthy", c.name)
			status.Unhealthy = append(status.Unhealthy, c.name)
			continue
		}
		if now.After(c.latestPing.Add(c.timeout)) {
			status.Unhealthy = append(status.Unhealthy, c.name)
		} else {
			status.Healthy = append(status.Healthy, c.name)
		}
	}
	return status
}

// reset is used for unit testing
func reset() {
	catalog.Lock()
	for token := range catalog.components {
		delete(catalog.components, token)
	}
	catalog.Unlock()
}

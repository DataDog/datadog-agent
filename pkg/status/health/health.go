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

// DefaultTimeout holds the duration used for default
const DefaultTimeout = 30 * time.Second

// ID is returned when registering and is to be used when pinging
type ID string

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

// HealthStatus represents the current status of registered components
type HealthStatus struct {
	Healthy   []string
	UnHealthy []string
}

// Register a component with the default 30 seconds timeout, returns a token
func Register(name string) ID {
	return RegisterWithCustomTimeout(name, DefaultTimeout)
}

// RegisterWithCustomTimeout allows to register with a custom timeout duration
func RegisterWithCustomTimeout(name string, timeout time.Duration) ID {
	catalog.Lock()
	defer catalog.Unlock()

	// TODO: create unique IDs
	catalog.components[ID(name)] = &component{
		name:       name,
		timeout:    timeout,
		latestPing: time.Now().Add(-2 * timeout), // Register as unhealthy by default
	}

	return ID(name)
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

// Status allows to query the health status of the agent
func Status() HealthStatus {
	status := HealthStatus{}
	now := time.Now()

	catalog.RLock()
	defer catalog.RUnlock()

	for _, c := range catalog.components {
		if c.latestPing.IsZero() {
			log.Warnf("Error processing component %q, considering it unhealthy", c.name)
			status.UnHealthy = append(status.UnHealthy, c.name)
			continue
		}
		if now.After(c.latestPing.Add(c.timeout)) {
			status.UnHealthy = append(status.UnHealthy, c.name)
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

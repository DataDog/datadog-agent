// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package health

import (
	"errors"
	"time"
)

var readinessAndLivenessCatalog = newCatalog()
var readinessOnlyCatalog = newCatalog()
var startupOnlyCatalog = newCatalog()

// RegisterReadiness registers a component for readiness check with the default 30 seconds timeout, returns a token
func RegisterReadiness(name string, options ...Option) *Handle {
	return readinessOnlyCatalog.register(name, options...)
}

// RegisterLiveness registers a component for liveness check with the default 30 seconds timeout, returns a token
func RegisterLiveness(name string, options ...Option) *Handle {
	return readinessAndLivenessCatalog.register(name, options...)
}

// RegisterStartup registers a component for startup check, returns a token
func RegisterStartup(name string, options ...Option) *Handle {
	// Startup health checks are registered with Once option because, by design, they should stop being checked
	// once they are marked as healthy once
	return startupOnlyCatalog.register(name, append(options, Once)...)
}

// Deregister a component from the healthcheck
func Deregister(handle *Handle) error {
	if readinessAndLivenessCatalog.deregister(handle) == nil {
		return nil
	}
	if readinessOnlyCatalog.deregister(handle) == nil {
		return nil
	}
	if startupOnlyCatalog.deregister(handle) == nil {
		return nil
	}
	return errors.New("component not registered")
}

// GetLive returns health of all components registered for liveness
func GetLive() Status {
	return readinessAndLivenessCatalog.getStatus()
}

// GetReady returns health of all components registered for both readiness and liveness
func GetReady() (ret Status) {
	liveStatus := readinessAndLivenessCatalog.getStatus()
	readyStatus := readinessOnlyCatalog.getStatus()
	ret.Healthy = append(liveStatus.Healthy, readyStatus.Healthy...)
	ret.Unhealthy = append(liveStatus.Unhealthy, readyStatus.Unhealthy...)
	return
}

// GetStartup returns health of all components registered for startup
func GetStartup() Status {
	return startupOnlyCatalog.getStatus()
}

// getStatusNonBlocking allows to query the health status of the agent
// and is guaranteed to return under 500ms.
func getStatusNonBlocking(getStatus func() Status) (Status, error) {
	// Run the health status in a goroutine
	ch := make(chan Status, 1)
	go func() {
		ch <- getStatus()
	}()

	// Only wait 500ms before returning
	select {
	case status := <-ch:
		return status, nil
	case <-time.After(500 * time.Millisecond):
		return Status{}, errors.New("timeout when getting health status")
	}
}

// GetLiveNonBlocking returns the health of all components registered for liveness with a 500ms timeout
func GetLiveNonBlocking() (Status, error) {
	return getStatusNonBlocking(GetLive)
}

// GetReadyNonBlocking returns the health of all components registered for both readiness and liveness with a 500ms timeout
func GetReadyNonBlocking() (Status, error) {
	return getStatusNonBlocking(GetReady)
}

// GetStartupNonBlocking returns the health of all components registered for startup with a 500ms timeout
func GetStartupNonBlocking() (Status, error) {
	return getStatusNonBlocking(GetStartup)
}

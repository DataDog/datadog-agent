// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package health provides utilities for collecting and reporting agent health issues
package health

import (
	"sync"
	"time"
)

// HealthCheckFunc is a function that checks for a health issue and returns the issue ID and context if present
// Returns empty issueID if no issue is detected
type HealthCheckFunc func() (issueID string, context map[string]string)

// HealthCheckOptions holds configuration options for a health check
type HealthCheckOptions struct {
	Interval time.Duration // Custom interval for this check (0 means use default)
}

// HealthCheckOption is a functional option for configuring a health check
type HealthCheckOption func(*HealthCheckOptions)

// WithInterval sets a custom interval for the health check
// If not provided or set to 0, the default interval from config will be used
func WithInterval(d time.Duration) HealthCheckOption {
	return func(opts *HealthCheckOptions) {
		opts.Interval = d
	}
}

// Collector is the interface for registering health checks
type Collector interface {
	RegisterHealthCheck(checkID, checkName string, checkFunc HealthCheckFunc, opts ...HealthCheckOption)
}

// startupHealthCheck represents a periodic health check registered during startup
type startupHealthCheck struct {
	CheckID   string               // Unique identifier for this check
	CheckName string               // Human-readable name
	CheckFunc HealthCheckFunc      // Function to call to check for issues
	Options   []HealthCheckOption  // Options for configuring the check
}

var (
	healthChecks []startupHealthCheck
	collector    Collector
	mutex        sync.RWMutex
)

// SetCollector sets the health collector component (called by health platform on initialization)
// Any health checks registered before this call will be flushed to the collector
func SetCollector(c Collector) {
	mutex.Lock()
	defer mutex.Unlock()
	collector = c

	// Flush any health checks that were collected before the component was available
	for _, check := range healthChecks {
		collector.RegisterHealthCheck(check.CheckID, check.CheckName, check.CheckFunc, check.Options...)
	}

	// Clear the temporary storage
	healthChecks = nil
}

// RegisterHealthCheck registers a periodic health check to be run by the health platform
// If the health platform component is not yet available, the check is stored temporarily
// Use WithInterval() to set a custom interval for this check
func RegisterHealthCheck(checkID, checkName string, checkFunc HealthCheckFunc, opts ...HealthCheckOption) {
	mutex.Lock()
	defer mutex.Unlock()

	// If the component is available, register directly
	if collector != nil {
		collector.RegisterHealthCheck(checkID, checkName, checkFunc, opts...)
		return
	}

	// Otherwise, store it for later registration when the component initializes
	healthChecks = append(healthChecks, startupHealthCheck{
		CheckID:   checkID,
		CheckName: checkName,
		CheckFunc: checkFunc,
		Options:   opts,
	})
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
)

const (
	// nvmlUnavailableThreshold is the time to wait before marking NVML as unavailable
	// the library might be unavailable for a while due to driver installation, etc. We don't want
	// to alert on this too early. Also, we don't care about early detection too much, so we can wait a bit
	// to be sure it's really unavailable.
	nvmlUnavailableThreshold = 5 * time.Minute

	// defaultCheckInterval is the default interval for periodic NVML library checks
	defaultCheckInterval = 30 * time.Second
)

// NvmlStateTracker tracks the state of the NVML library initialization
// and reports telemetry when it remains unavailable for extended periods.
type NvmlStateTracker struct {
	mu sync.Mutex

	errorCount           int
	firstCheckTime       time.Time
	unavailableGaugeSet  bool
	unavailabilityMarked bool

	// Telemetry metrics
	errorCounter     telemetry.Counter
	unavailableGauge telemetry.Gauge

	// Goroutine lifecycle management
	checkInterval time.Duration
	done          chan struct{}
	wg            sync.WaitGroup
}

// NewNvmlStateTracker creates a new NvmlStateTracker with the given telemetry component.
func NewNvmlStateTracker(tm telemetry.Component) *NvmlStateTracker {
	return NewNvmlStateTrackerWithInterval(tm, defaultCheckInterval)
}

// NewNvmlStateTrackerWithInterval creates a new NvmlStateTracker with a custom check interval.
func NewNvmlStateTrackerWithInterval(tm telemetry.Component, checkInterval time.Duration) *NvmlStateTracker {
	subsystem := "gpu__nvml"

	return &NvmlStateTracker{
		errorCounter:     tm.NewCounter(subsystem, "init_errors", nil, "Number of errors when initializing NVML library"),
		unavailableGauge: tm.NewGauge(subsystem, "library_unavailable", nil, "Whether NVML library is unavailable after threshold time (1=unavailable, 0=available)"),
		checkInterval:    checkInterval,
		done:             make(chan struct{}),
	}
}

// Check attempts to get the NVML library and tracks errors.
// If the library remains unavailable for more than nvmlUnavailableThreshold,
// it sets the unavailable gauge to 1.
func (n *NvmlStateTracker) Check() {
	n.mu.Lock()
	defer n.mu.Unlock()

	_, err := GetSafeNvmlLib()
	if err != nil {
		// Track the first check time
		if n.firstCheckTime.IsZero() {
			n.firstCheckTime = time.Now()
		}

		// Increment error counter
		n.errorCount++
		n.errorCounter.Add(1)

		// Check if threshold has been exceeded
		if !n.unavailabilityMarked && time.Since(n.firstCheckTime) >= nvmlUnavailableThreshold {
			n.unavailableGauge.Set(1)
			n.unavailabilityMarked = true
		}
	} else {
		// Library is available - reset state and set gauge to 0 if it was previously set
		if n.unavailabilityMarked || n.unavailableGaugeSet {
			n.unavailableGauge.Set(0)
			n.unavailableGaugeSet = true
		}
		n.firstCheckTime = time.Time{}
		n.errorCount = 0
		n.unavailabilityMarked = false
	}
}

// Start begins periodic checking of the NVML library status in a background goroutine.
func (n *NvmlStateTracker) Start() {
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()

		ticker := time.NewTicker(n.checkInterval)
		defer ticker.Stop()

		// Perform initial check immediately
		n.Check()

		for {
			select {
			case <-n.done:
				return
			case <-ticker.C:
				n.Check()
			}
		}
	}()
}

// Stop stops the background checking goroutine and waits for it to finish.
func (n *NvmlStateTracker) Stop() {
	close(n.done)
	n.wg.Wait()
}

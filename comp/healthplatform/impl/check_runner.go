// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package healthplatformimpl

import (
	"errors"
	"fmt"
	"sync"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
)

const (
	// defaultCheckInterval is the default interval for health checks if not specified
	defaultCheckInterval = 15 * time.Minute
)

// registeredCheck holds the metadata and function for a registered health check
type registeredCheck struct {
	checkID   string
	checkName string
	checkFn   healthplatform.HealthCheckFunc
	interval  time.Duration
	stopCh    chan struct{}
}

// issueReporter is the interface for reporting issues (satisfied by healthPlatformImpl)
type issueReporter interface {
	ReportIssue(checkID string, checkName string, report *healthplatformpayload.IssueReport) error
}

// checkRunner manages periodic health checks.
// Each registered check runs in its own goroutine with an independent ticker.
// This design allows per-check intervals and prevents slow checks from blocking others.
type checkRunner struct {
	log      log.Component
	reporter issueReporter

	checks   map[string]*registeredCheck
	checkMux sync.RWMutex

	started bool
	wg      sync.WaitGroup
}

// newCheckRunner creates a new check runner instance
func newCheckRunner(logger log.Component, reporter issueReporter) *checkRunner {
	return &checkRunner{
		log:      logger,
		reporter: reporter,
		checks:   make(map[string]*registeredCheck),
	}
}

// Start begins running all registered checks in background goroutines
func (r *checkRunner) Start() {
	r.log.Info("Starting health platform check runner")

	r.checkMux.Lock()
	defer r.checkMux.Unlock()

	r.started = true

	// Start goroutines for all already-registered checks
	for _, check := range r.checks {
		r.startCheck(check)
	}
}

// Stop stops all running checks and waits for graceful shutdown
func (r *checkRunner) Stop() {
	r.log.Info("Stopping health platform check runner")

	r.checkMux.Lock()
	r.started = false
	// Signal all checks to stop
	for _, check := range r.checks {
		close(check.stopCh)
	}
	r.checkMux.Unlock()

	// Wait for all goroutines to finish
	r.wg.Wait()
}

// RegisterCheck registers a new periodic health check
func (r *checkRunner) RegisterCheck(checkID, checkName string, checkFn healthplatform.HealthCheckFunc, interval time.Duration) error {
	if checkID == "" {
		return errors.New("check ID cannot be empty")
	}
	if checkFn == nil {
		return errors.New("check function cannot be nil")
	}

	// Use default interval if not specified
	if interval <= 0 {
		interval = defaultCheckInterval
	}

	r.checkMux.Lock()
	defer r.checkMux.Unlock()

	// Check if already registered
	if _, exists := r.checks[checkID]; exists {
		return fmt.Errorf("Health check %s is already registered", checkID)
	}

	check := &registeredCheck{
		checkID:   checkID,
		checkName: checkName,
		checkFn:   checkFn,
		interval:  interval,
		stopCh:    make(chan struct{}),
	}

	r.checks[checkID] = check

	// If runner is already started, start this check immediately
	if r.started {
		r.startCheck(check)
	}

	r.log.Debug(fmt.Sprintf("Registered health check: %s (interval: %v)", checkName, interval))
	return nil
}

// startCheck launches a goroutine to run the check at its interval
func (r *checkRunner) startCheck(check *registeredCheck) {
	r.wg.Add(1)
	go r.runAndScheduleCheck(check)
}

// runAndScheduleCheck runs a check immediately and schedules it to run periodically
func (r *checkRunner) runAndScheduleCheck(check *registeredCheck) {
	defer r.wg.Done()

	// Run immediately on start
	r.executeCheck(check)

	ticker := time.NewTicker(check.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.executeCheck(check)
		case <-check.stopCh:
			return
		}
	}
}

// executeCheck runs a single health check and reports the result
func (r *checkRunner) executeCheck(check *registeredCheck) {
	report, err := check.checkFn()
	if err != nil {
		r.log.Warn(fmt.Sprintf("Health check %s failed: %v", check.checkName, err))
		return
	}

	// Report the result (nil report clears any existing issue)
	if err := r.reporter.ReportIssue(check.checkID, check.checkName, report); err != nil {
		r.log.Warn(fmt.Sprintf("Failed to report issue for check %s: %v", check.checkName, err))
	}
}

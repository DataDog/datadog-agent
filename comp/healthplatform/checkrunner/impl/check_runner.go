// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package checkrunnerimpl implements the health platform check runner component.
package checkrunnerimpl

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	checkrunnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/checkrunner/def"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

const (
	// defaultCheckInterval is the default interval for health checks if not specified
	defaultCheckInterval = 15 * time.Minute
)

// registeredCheck holds the metadata and function for a registered health check
type registeredCheck struct {
	checkID   string
	checkName string
	checkFn   checkrunnerdef.HealthCheckFunc
	interval  time.Duration
	stopCh    chan struct{}
}

// checkRunner manages periodic health checks.
// Each registered check runs in its own goroutine with an independent ticker.
// This design allows per-check intervals and prevents slow checks from blocking others.
type checkRunner struct {
	log        log.Component
	reporterMu sync.RWMutex
	reporter   checkrunnerdef.IssueReporter

	checks   map[string]*registeredCheck
	checkMux sync.RWMutex

	started bool
	wg      sync.WaitGroup
}

// Requires defines the dependencies for the check runner.
type Requires struct {
	Log       log.Component
	Lifecycle compdef.Lifecycle
}

// New creates a new check runner instance and registers its lifecycle hooks.
func New(reqs Requires) checkrunnerdef.Component {
	r := &checkRunner{
		log:    reqs.Log,
		checks: make(map[string]*registeredCheck),
	}
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: r.start,
		OnStop:  r.stop,
	})
	return r
}

func (r *checkRunner) start(_ context.Context) error {
	r.log.Info("Starting health platform check runner")
	r.checkMux.Lock()
	defer r.checkMux.Unlock()
	r.started = true
	for _, check := range r.checks {
		r.startCheck(check)
	}
	return nil
}

func (r *checkRunner) stop(_ context.Context) error {
	r.log.Info("Stopping health platform check runner")
	r.checkMux.Lock()
	r.started = false
	for _, check := range r.checks {
		close(check.stopCh)
	}
	r.checkMux.Unlock()
	r.wg.Wait()
	return nil
}

// SetReporter wires the issue reporter. Safe to call concurrently with executeCheck.
func (r *checkRunner) SetReporter(reporter checkrunnerdef.IssueReporter) {
	r.reporterMu.Lock()
	defer r.reporterMu.Unlock()
	r.reporter = reporter
}

// RegisterCheck registers a new periodic health check
func (r *checkRunner) RegisterCheck(checkID, checkName string, checkFn checkrunnerdef.HealthCheckFunc, interval time.Duration) error {
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

// RunCheck runs a single health check immediately
func (r *checkRunner) RunCheck(checkID, checkName string, checkFn checkrunnerdef.HealthCheckFunc) error {
	if checkID == "" {
		return errors.New("check ID cannot be empty")
	}
	if checkFn == nil {
		return errors.New("check function cannot be nil")
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.executeCheck(&registeredCheck{
			checkID:   checkID,
			checkName: checkName,
			checkFn:   checkFn,
			stopCh:    make(chan struct{}),
		})
	}()
	return nil
}

// startCheck launches a goroutine to run the check at its interval
func (r *checkRunner) startCheck(check *registeredCheck) {
	r.wg.Add(1)
	r.log.Debugf("Running health check '%s' on interval %v", check.checkName, check.interval)
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
	r.reporterMu.RLock()
	reporter := r.reporter
	r.reporterMu.RUnlock()

	if reporter == nil {
		r.log.Warn("Health check runner has no reporter set, skipping check: " + check.checkName)
		return
	}

	report, err := check.checkFn()
	if err != nil {
		r.log.Warn(fmt.Sprintf("Health check %s failed: %v", check.checkName, err))
		return
	}

	// Report the result (nil report clears any existing issue)
	if err := reporter.ReportIssue(check.checkID, check.checkName, report); err != nil {
		r.log.Warn(fmt.Sprintf("Failed to report issue for check %s: %v", check.checkName, err))
	}
}

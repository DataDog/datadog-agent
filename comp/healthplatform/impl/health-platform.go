// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package healthplatformimpl implements the health-platform component interface
package healthplatformimpl

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	healthcheck "github.com/DataDog/datadog-agent/comp/healthplatform/impl/health-check"
)

const (
	// tickerInterval is the interval for the health check ticker
	tickerInterval = 15 * time.Second
)

// Requires defines the dependencies for the health-platform component
type Requires struct {
	Lifecycle compdef.Lifecycle
	Log       log.Component
}

// Provides defines the output of the health-platform component
type Provides struct {
	Comp healthplatform.Component
}

// healthPlatformImpl implements the health platform component
type healthPlatformImpl struct {
	log       log.Component
	ticker    *time.Ticker
	stopCh    chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	checks    map[string]healthplatform.CheckConfig
	checksMux sync.RWMutex
	issues    map[string][]healthplatform.Issue
	issuesMux sync.RWMutex
}

// NewComponent creates a new health-platform component
func NewComponent(reqs Requires) (Provides, error) {
	reqs.Log.Info("Creating health platform component")
	ctx, cancel := context.WithCancel(context.Background())

	comp := &healthPlatformImpl{
		log:       reqs.Log,
		ticker:    time.NewTicker(tickerInterval),
		stopCh:    make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
		checks:    make(map[string]healthplatform.CheckConfig),
		checksMux: sync.RWMutex{},
		issues:    make(map[string][]healthplatform.Issue),
		issuesMux: sync.RWMutex{},
	}

	// Register lifecycle hooks
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: comp.start,
		OnStop:  comp.stop,
	})

	comp.RegisterDefaultChecks()

	provides := Provides{Comp: comp}
	return provides, nil
}

// start starts the health platform component
func (h *healthPlatformImpl) start(_ context.Context) error {
	h.log.Info("Starting health platform component")

	go h.runTicker()
	return nil
}

// stop stops the health platform component
func (h *healthPlatformImpl) stop(_ context.Context) error {
	h.log.Info("Stopping health platform component")

	h.cancel()
	h.ticker.Stop()
	close(h.stopCh)

	return nil
}

// Other components will register their check on their side directly, this one will be moved later
// RegisterDefaultChecks registers the default set of health checks
func (h *healthPlatformImpl) RegisterDefaultChecks() {
	h.checksMux.Lock()
	defer h.checksMux.Unlock()

	h.checks[healthcheck.NewDockerLogPermissionsCheckConfig().CheckID] = healthcheck.NewDockerLogPermissionsCheckConfig()
}

// RegisterCheck registers a health check with the platform
func (h *healthPlatformImpl) RegisterCheck(check healthplatform.CheckConfig) error {
	h.checksMux.Lock()
	defer h.checksMux.Unlock()

	if check.CheckID == "" {
		return fmt.Errorf("check ID cannot be empty")
	}

	if check.Callback == nil {
		return fmt.Errorf("check callback cannot be nil")
	}

	h.checks[check.CheckID] = check
	h.log.Info("Registered health check: " + check.CheckName + " (ID: " + check.CheckID + ")")
	return nil
}

// runTicker runs the periodic ticker that executes health checks every 15 seconds
func (h *healthPlatformImpl) runTicker() {
	for {
		select {
		case <-h.ticker.C:
			h.runHealthChecks()
		case <-h.stopCh:
			return
		case <-h.ctx.Done():
			return
		}
	}
}

// runHealthChecks executes all registered health checks
func (h *healthPlatformImpl) runHealthChecks() {
	h.checksMux.RLock()
	defer h.checksMux.RUnlock()

	if len(h.checks) == 0 {
		h.log.Debug("No health checks registered")
		return
	}

	// Check which checks already have issues
	h.issuesMux.RLock()
	checksToRun := make([]healthplatform.CheckConfig, 0)
	for _, check := range h.checks {
		issues, exists := h.issues[check.CheckID]
		if !exists {
			// Never run - should run
			checksToRun = append(checksToRun, check)
		} else if len(issues) == 0 {
			// Run but found no issues - should run again
			checksToRun = append(checksToRun, check)
		}
		// If len(issues) > 0, skip (has issues)
	}
	h.issuesMux.RUnlock()

	if len(checksToRun) == 0 {
		h.log.Debug("All health checks already have issues detected, skipping execution")
		return
	}

	h.log.Debug("Running " + fmt.Sprintf("%d", len(checksToRun)) + " health checks (skipping " + fmt.Sprintf("%d", len(h.checks)-len(checksToRun)) + " with existing issues)")
	for _, check := range checksToRun {
		go h.executeCheck(check)
	}
}

// executeCheck executes a single health check
func (h *healthPlatformImpl) executeCheck(check healthplatform.CheckConfig) {
	defer func() {
		if r := recover(); r != nil {
			h.log.Warn("Health check panicked: " + check.CheckName + " - " + fmt.Sprintf("%v", r))
		}
	}()

	issues, err := check.Callback()
	if err != nil {
		h.log.Warn("Health check failed: " + check.CheckName + " - " + err.Error())
		// Store empty issues for failed checks so they can run again
		h.storeIssues(check.CheckID, []healthplatform.Issue{})
		return
	}

	// Store the issues
	h.storeIssues(check.CheckID, issues)

	if len(issues) > 0 {
		h.log.Info("Health check found issues: " + check.CheckName + " - " + fmt.Sprintf("%d issues", len(issues)))
		for _, issue := range issues {
			h.log.Info("Issue: " + issue.Title + " (" + issue.Severity + ")")
		}
	} else {
		h.log.Debug("Health check passed: " + check.CheckName)
	}
}

// storeIssues stores issues for a specific check
func (h *healthPlatformImpl) storeIssues(checkID string, issues []healthplatform.Issue) {
	h.issuesMux.Lock()
	defer h.issuesMux.Unlock()

	// Add timestamp to issues
	now := time.Now().Format(time.RFC3339)
	for i := range issues {
		issues[i].DetectedAt = now
	}

	h.issues[checkID] = issues
}

// GetAllIssues returns all issues from all checks
func (h *healthPlatformImpl) GetAllIssues() map[string][]healthplatform.Issue {
	h.issuesMux.RLock()
	defer h.issuesMux.RUnlock()

	// Create a copy to avoid race conditions
	result := make(map[string][]healthplatform.Issue)
	for checkID, issues := range h.issues {
		result[checkID] = make([]healthplatform.Issue, len(issues))
		copy(result[checkID], issues)
	}

	return result
}

// GetIssuesForCheck returns issues for a specific check
func (h *healthPlatformImpl) GetIssuesForCheck(checkID string) []healthplatform.Issue {
	h.issuesMux.RLock()
	defer h.issuesMux.RUnlock()

	issues, exists := h.issues[checkID]
	if !exists {
		return []healthplatform.Issue{}
	}

	// Return a copy to avoid race conditions
	result := make([]healthplatform.Issue, len(issues))
	copy(result, issues)
	return result
}

// GetTotalIssueCount returns the total number of issues across all checks
func (h *healthPlatformImpl) GetTotalIssueCount() int {
	h.issuesMux.RLock()
	defer h.issuesMux.RUnlock()

	total := 0
	for _, issues := range h.issues {
		total += len(issues)
	}
	return total
}

// ClearIssuesForCheck clears issues for a specific check (useful when issues are resolved)
func (h *healthPlatformImpl) ClearIssuesForCheck(checkID string) {
	h.issuesMux.Lock()
	defer h.issuesMux.Unlock()

	delete(h.issues, checkID)
	h.log.Info("Cleared issues for check: " + checkID)
}

// ClearAllIssues clears all issues (useful for testing or when all issues are resolved)
func (h *healthPlatformImpl) ClearAllIssues() {
	h.issuesMux.Lock()
	defer h.issuesMux.Unlock()

	h.issues = make(map[string][]healthplatform.Issue)
	h.log.Info("Cleared all issues")
}

// Run runs the health checks and reports the issues
func (h *healthPlatformImpl) Run(_ context.Context) (*healthplatform.HealthReport, error) {
	// TODO: Implement actual health checks
	return nil, nil
}

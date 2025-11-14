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
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/remediations"
)

const (
	// tickerInterval is the interval for the health check ticker
	tickerInterval = 15 * time.Second
)

// Requires defines the dependencies for the health-platform component
type Requires struct {
	Lifecycle compdef.Lifecycle
	Log       log.Component
	Telemetry telemetry.Component
}

// Provides defines the output of the health-platform component
type Provides struct {
	Comp healthplatform.Component
}

// healthPlatformImpl implements the health platform component
// It manages a collection of health checks that run periodically and tracks
// any issues they detect. The component provides methods to register checks,
// retrieve issues, and manage the health monitoring lifecycle.
type healthPlatformImpl struct {
	// Core dependencies
	log       log.Component       // Logger for health platform operations
	telemetry telemetry.Component // Telemetry component for metrics collection

	// Lifecycle management
	ticker *time.Ticker       // Periodic ticker for running health checks
	ctx    context.Context    // Context for managing component lifecycle
	cancel context.CancelFunc // Cancel function for the context

	// Health check management
	checks    map[string]healthplatform.CheckConfig // Registered health checks by ID
	checksMux sync.RWMutex                          // Mutex for thread-safe access to checks

	// Issue tracking
	issues    map[string]*healthplatform.Issue // Issue detected by check ID (nil if no issue)
	issuesMux sync.RWMutex                     // Mutex for thread-safe access to issues

	// Remediation management
	remediationRegistry *remediations.Registry // Registry of remediation templates

	// Metrics
	metrics telemetryMetrics // Telemetry metrics for health platform
}

type telemetryMetrics struct {
	issuesCounter telemetry.Counter
}

// ============================================================================
// Constructor
// ============================================================================

// NewComponent creates a new health-platform component
// It initializes the component with its dependencies, sets up lifecycle management,
// configures telemetry metrics, and registers default health checks.
func NewComponent(reqs Requires) (Provides, error) {
	reqs.Log.Info("Creating health platform component")

	// Create context for component lifecycle management
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize the health platform implementation
	comp := &healthPlatformImpl{
		// Core dependencies
		log:       reqs.Log,
		telemetry: reqs.Telemetry,

		// Lifecycle management
		ticker: time.NewTicker(tickerInterval), // Start periodic ticker
		ctx:    ctx,                            // Set lifecycle context
		cancel: cancel,                         // Set context cancel function

		// Health check management
		checks: make(map[string]healthplatform.CheckConfig), // Initialize checks map

		// Remediation management
		remediationRegistry: remediations.NewRegistry(), // Initialize remediation registry with built-in templates
		checksMux:           sync.RWMutex{},             // Initialize checks mutex

		// Issue tracking
		issues:    make(map[string]*healthplatform.Issue), // Initialize issues map
		issuesMux: sync.RWMutex{},                         // Initialize issues mutex
	}

	// Register lifecycle hooks for component start/stop
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: comp.start,
		OnStop:  comp.stop,
	})

	// Initialize telemetry metrics
	comp.metrics = telemetryMetrics{
		issuesCounter: reqs.Telemetry.NewCounter(
			"health_platform",
			"issues_detected",
			[]string{"health_check_id"},
			"Number of health issues detected",
		),
	}

	// Register default health checks
	comp.RegisterDefaultChecks()

	// Return the component wrapped in Provides
	provides := Provides{Comp: comp}
	return provides, nil
}

// ============================================================================
// Lifecycle Methods
// ============================================================================

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

	return nil
}

// ============================================================================
// Registration Methods
// ============================================================================

// RegisterCheck registers a health check with the platform
func (h *healthPlatformImpl) RegisterCheck(check healthplatform.CheckConfig) error {
	h.checksMux.Lock()
	defer h.checksMux.Unlock()

	if check.CheckID == "" {
		return fmt.Errorf("check ID cannot be empty")
	}

	if check.Run == nil {
		return fmt.Errorf("check callback cannot be nil")
	}

	h.checks[check.CheckID] = check
	h.log.Info("Registered health check: " + check.CheckName + " (ID: " + check.CheckID + ")")
	return nil
}

// RegisterDefaultChecks registers the default set of health checks
// Currently empty as components now self-register their health checks
func (h *healthPlatformImpl) RegisterDefaultChecks() {
	h.checksMux.Lock()
	defer h.checksMux.Unlock()

	// Components like logs agent now register their own health checks during startup
	// This keeps health check logic co-located with the component it monitors
}

// ============================================================================
// Core Public API
// ============================================================================

// ReportIssue reports an issue with context, and the health platform fills in all metadata and remediation
// This is the preferred way for integrations to report issues as it keeps all issue knowledge
// centralized in the health platform registry
// If report is nil, it clears any existing issue (issue resolution)
func (h *healthPlatformImpl) ReportIssue(checkID string, checkName string, report *healthplatform.IssueReport) error {
	if checkID == "" {
		return fmt.Errorf("check ID cannot be empty")
	}

	// Get previous issue for state change detection
	h.issuesMux.RLock()
	previousIssue := h.issues[checkID]
	h.issuesMux.RUnlock()

	// Build the new issue (or nil if resolved)
	var newIssue *healthplatform.Issue
	if report != nil {
		if report.IssueID == "" {
			return fmt.Errorf("issue ID cannot be empty")
		}

		// Build complete issue from the registry using the issue ID and context
		issue, err := h.remediationRegistry.BuildIssue(report.IssueID, report.Context)
		if err != nil {
			return fmt.Errorf("failed to build issue %s: %w", report.IssueID, err)
		}

		// Append any additional tags from the report
		if len(report.Tags) > 0 {
			issue.Tags = append(issue.Tags, report.Tags...)
		}

		newIssue = issue
	}

	// Handle state change and logging (handles both new issues and resolution)
	h.handleIssueStateChange(checkName, previousIssue, newIssue)

	// Store the new issue (or delete if nil)
	if newIssue != nil {
		h.storeIssue(checkID, newIssue)
	} else {
		h.issuesMux.Lock()
		delete(h.issues, checkID)
		h.issuesMux.Unlock()
	}

	return nil
}

// RunHealthChecks manually triggers health check execution
// If async is true, checks run in parallel goroutines
// If async is false, checks run synchronously (useful for testing)
func (h *healthPlatformImpl) RunHealthChecks(async bool) {
	h.checksMux.RLock()
	defer h.checksMux.RUnlock()

	if len(h.checks) == 0 {
		h.log.Debug("No health checks registered")
		return
	}

	mode := ""
	if !async {
		mode = " synchronously"
	}
	h.log.Debug("Running " + fmt.Sprintf("%d", len(h.checks)) + " health checks" + mode)

	for _, check := range h.checks {
		if async {
			go h.executeCheck(check)
		} else {
			h.executeCheck(check)
		}
	}
}

// ============================================================================
// Query Methods
// ============================================================================

// GetAllIssues returns the count and all issues from all checks (indexed by check ID)
func (h *healthPlatformImpl) GetAllIssues() (int, map[string]*healthplatform.Issue) {
	h.issuesMux.RLock()
	defer h.issuesMux.RUnlock()

	// Create a copy to avoid external modifications and count issues
	count := 0
	result := make(map[string]*healthplatform.Issue)
	for checkID, issue := range h.issues {
		if issue != nil {
			issueCopy := *issue
			result[checkID] = &issueCopy
			count++
		} else {
			result[checkID] = nil
		}
	}
	return count, result
}

// GetIssueForCheck returns the issue for a specific check (nil if no issue)
func (h *healthPlatformImpl) GetIssueForCheck(checkID string) *healthplatform.Issue {
	h.issuesMux.RLock()
	defer h.issuesMux.RUnlock()

	issue := h.issues[checkID]
	if issue == nil {
		return nil
	}

	// Return a copy to avoid external modifications
	issueCopy := *issue
	return &issueCopy
}

// ============================================================================
// Clear Methods
// ============================================================================

// ClearIssuesForCheck clears the issue for a specific check (useful when issue is resolved)
func (h *healthPlatformImpl) ClearIssuesForCheck(checkID string) {
	h.issuesMux.Lock()
	defer h.issuesMux.Unlock()

	delete(h.issues, checkID)
	h.log.Info("Cleared issue for check: " + checkID)
}

// ClearAllIssues clears all issues (useful for testing or when all issues are resolved)
func (h *healthPlatformImpl) ClearAllIssues() {
	h.issuesMux.Lock()
	defer h.issuesMux.Unlock()

	h.issues = make(map[string]*healthplatform.Issue)
	h.log.Info("Cleared all issues")
}

// ============================================================================
// Internal Helper Methods
// ============================================================================

// runTicker runs the periodic ticker that executes health checks every 15 seconds
func (h *healthPlatformImpl) runTicker() {
	for {
		select {
		case <-h.ticker.C:
			h.RunHealthChecks(true)
		case <-h.ctx.Done():
			return
		}
	}
}

// executeCheck executes a single health check
func (h *healthPlatformImpl) executeCheck(check healthplatform.CheckConfig) {
	defer func() {
		if r := recover(); r != nil {
			h.log.Warn("Health check panicked: " + check.CheckName + " - " + fmt.Sprintf("%v", r))
		}
	}()

	// Pass the component's context to the health check so it can respect cancellation
	report, err := check.Run(h.ctx)
	if err != nil {
		h.log.Warn("Health check failed: " + check.CheckName + " - " + err.Error())
		return
	}

	// Report the issue (or resolution if nil) - this handles all state management
	_ = h.ReportIssue(check.CheckID, check.CheckName, report)
}

// handleIssueStateChange detects state changes and logs appropriately
func (h *healthPlatformImpl) handleIssueStateChange(checkName string, oldIssue, newIssue *healthplatform.Issue) {
	// If both are nil, no change
	if oldIssue == nil && newIssue == nil {
		return
	}

	// New issue detected
	if newIssue != nil && oldIssue == nil {
		h.log.Info("Health check found NEW issue: " + checkName)
		h.log.Info("Issue: " + newIssue.Title + " (" + newIssue.Severity + ")")
		return
	}

	// Issue resolved
	if newIssue == nil && oldIssue != nil {
		h.log.Info("Health check issue RESOLVED: " + checkName)
		return
	}

	// Both exist, check if details changed
	if oldIssue.ID != newIssue.ID ||
		oldIssue.Title != newIssue.Title ||
		oldIssue.Severity != newIssue.Severity ||
		oldIssue.Description != newIssue.Description {
		h.log.Info("Health check issue CHANGED: " + checkName)
		h.log.Info("Issue: " + newIssue.Title + " (" + newIssue.Severity + ")")
	}
}

// storeIssue stores an issue for a specific check (nil if no issue)
func (h *healthPlatformImpl) storeIssue(checkID string, issue *healthplatform.Issue) {
	h.issuesMux.Lock()
	defer h.issuesMux.Unlock()

	// Add timestamp to issue if present
	if issue != nil {
		issue.DetectedAt = time.Now().Format(time.RFC3339)
		// Update telemetry
		h.metrics.issuesCounter.Add(1, checkID)
	}

	h.issues[checkID] = issue
}

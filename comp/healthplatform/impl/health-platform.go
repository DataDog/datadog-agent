// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package healthplatformimpl implements the health-platform component interface
package healthplatformimpl

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/remediations"
)

// Requires defines the dependencies for the health-platform component
type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    config.Component
	Log       log.Component
	Telemetry telemetry.Component
}

// Provides defines the output of the health-platform component
type Provides struct {
	Comp healthplatform.Component
}

// healthPlatformImpl implements the health platform component
// It aggregates health issues reported by various agent components and integrations.
// The component provides methods to report issues, retrieve them, and manage the health monitoring lifecycle.
type healthPlatformImpl struct {
	// Core dependencies
	log       log.Component       // Logger for health platform operations
	telemetry telemetry.Component // Telemetry component for metrics collection

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
// It initializes the component with its dependencies and configures telemetry metrics.
func NewComponent(reqs Requires) (Provides, error) {
	// Check if health platform is enabled
	if !reqs.Config.GetBool("health_platform.enabled") {
		reqs.Log.Info("Health platform component is disabled")
		return Provides{Comp: &noopHealthPlatform{}}, nil
	}

	reqs.Log.Info("Creating health platform component")

	// Initialize the health platform implementation
	comp := &healthPlatformImpl{
		// Core dependencies
		log:       reqs.Log,
		telemetry: reqs.Telemetry,

		// Remediation management
		remediationRegistry: remediations.NewRegistry(), // Initialize remediation registry with built-in templates

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
	return nil
}

// stop stops the health platform component
func (h *healthPlatformImpl) stop(_ context.Context) error {
	h.log.Info("Stopping health platform component")
	return nil
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
		return errors.New("check ID cannot be empty")
	}

	// Get previous issue for state change detection
	h.issuesMux.RLock()
	previousIssue := h.issues[checkID]
	h.issuesMux.RUnlock()

	// Build the new issue (or nil if resolved)
	var newIssue *healthplatform.Issue
	if report != nil {
		if report.IssueID == "" {
			return errors.New("issue ID cannot be empty")
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

	// Store the new issue (or clear if nil)
	if newIssue != nil {
		h.storeIssue(checkID, newIssue)
	} else {
		h.ClearIssuesForCheck(checkID)
	}

	return nil
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

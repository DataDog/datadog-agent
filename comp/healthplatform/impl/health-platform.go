// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package healthplatformimpl implements the health-platform component interface
package healthplatformimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/proto"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	issuesmod "github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
	"github.com/DataDog/datadog-agent/pkg/version"

	// Import issue modules to trigger their init() registration
	_ "github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues/checkfailure"
	_ "github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues/dockerpermissions"
)

// Requires defines the dependencies for the health-platform component
type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    config.Component
	Log       log.Component
	Telemetry telemetry.Component
	Hostname  hostnameinterface.Component
}

// Provides defines the output of the health-platform component
type Provides struct {
	compdef.Out
	Comp          healthplatformdef.Component
	APIGetIssues  api.AgentEndpointProvider
	FlareProvider flaretypes.Provider
}

// healthPlatformImpl implements the health platform component
// It aggregates health issues reported by various agent components and integrations.
// The component provides methods to report issues, retrieve them, and manage the health monitoring lifecycle.
type healthPlatformImpl struct {
	// Core dependencies
	config           config.Component            // Config component for accessing configuration
	log              log.Component               // Logger for health platform operations
	telemetry        telemetry.Component         // Telemetry component for metrics collection
	hostnameProvider hostnameinterface.Component // Hostname provider for runtime resolution

	// Issue tracking
	issues    map[string]*healthplatform.Issue // Issue detected by check ID (nil if no issue)
	issuesMux sync.RWMutex                     // Mutex for thread-safe access to issues

	// Issue module registry (combines checks + remediations)
	issueRegistry *issuesmod.Registry

	// Forwarder for sending reports to Datadog intake
	forwarder *forwarder

	// Check runner for periodic health checks
	checkRunner *checkRunner

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
		noop := &noopHealthPlatform{}
		return Provides{
			Comp: noop,
			APIGetIssues: api.NewAgentEndpointProvider(
				noop.getIssuesHandler,
				"/health-platform/issues",
				"GET",
			),
			FlareProvider: flaretypes.NewProvider(noop.fillFlare),
		}, nil
	}

	reqs.Log.Info("Creating health platform component")

	// Create unified issue registry and register all self-registered modules
	issueRegistry := issuesmod.NewRegistry()
	for _, module := range issuesmod.GetAllModules() {
		issueRegistry.RegisterModule(module)
	}

	// Initialize the health platform implementation
	comp := &healthPlatformImpl{
		// Core dependencies
		config:           reqs.Config,
		log:              reqs.Log,
		telemetry:        reqs.Telemetry,
		hostnameProvider: reqs.Hostname,

		// Issue module registry
		issueRegistry: issueRegistry,

		// Issue tracking
		issues:    make(map[string]*healthplatform.Issue),
		issuesMux: sync.RWMutex{},
	}

	// Initialize check runner (must be after comp is created as it needs the reporter interface)
	comp.checkRunner = newCheckRunner(reqs.Log, comp)

	// Register built-in health checks from issue modules
	for _, check := range issueRegistry.GetBuiltInChecks() {
		if err := comp.RegisterCheck(check.ID, check.Name, check.CheckFn, check.Interval); err != nil {
			reqs.Log.Warn("Failed to register health check " + check.ID + ": " + err.Error())
		}
	}

	if err := comp.initForwarder(reqs); err != nil {
		reqs.Log.Warn("Health platform forwarder not initialized: " + err.Error())
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

	// Return the component wrapped in Provides with API endpoints
	return Provides{
		Comp: comp,
		APIGetIssues: api.NewAgentEndpointProvider(
			comp.getIssuesHandler,
			"/health-platform/issues",
			"GET",
		),
		FlareProvider: flaretypes.NewProvider(comp.fillFlare),
	}, nil
}

// ============================================================================
// Lifecycle Methods
// ============================================================================

// start starts the health platform component
func (h *healthPlatformImpl) start(_ context.Context) error {
	h.log.Info("Starting health platform component")

	// Start the check runner for periodic health checks
	if h.checkRunner != nil {
		h.checkRunner.Start()
	}

	// Start the forwarder for sending reports to intake
	if h.forwarder != nil {
		h.forwarder.Start()
	}

	return nil
}

// stop stops the health platform component
func (h *healthPlatformImpl) stop(_ context.Context) error {
	h.log.Info("Stopping health platform component")

	// Stop the check runner first to prevent new issues being reported
	if h.checkRunner != nil {
		h.checkRunner.Stop()
	}

	// Stop the forwarder
	if h.forwarder != nil {
		h.forwarder.Stop()
	}

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
		if report.IssueId == "" {
			return errors.New("issue ID cannot be empty")
		}

		// Build complete issue from the registry using the issue ID and context
		issue, err := h.issueRegistry.BuildIssue(report.IssueId, report.Context)
		if err != nil {
			return fmt.Errorf("failed to build issue %s: %w", report.IssueId, err)
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

// RegisterCheck registers a periodic health check function
// The check function will be called at the specified interval
// If interval is 0 or negative, uses default of 15 minutes
func (h *healthPlatformImpl) RegisterCheck(checkID string, checkName string, checkFn healthplatformdef.HealthCheckFunc, interval time.Duration) error {
	return h.checkRunner.RegisterCheck(checkID, checkName, checkFn, interval)
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
			result[checkID] = proto.Clone(issue).(*healthplatform.Issue)
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
	return proto.Clone(issue).(*healthplatform.Issue)
}

// ============================================================================
// Clear Methods
// ============================================================================

// ClearIssuesForCheck clears the issue for a specific check (useful when issue is resolved)
func (h *healthPlatformImpl) ClearIssuesForCheck(checkID string) {
	h.issuesMux.Lock()
	defer h.issuesMux.Unlock()

	// Only log if there was actually an issue to clear
	if _, existed := h.issues[checkID]; existed {
		h.log.Info("Cleared issue for check: " + checkID)
	}
	delete(h.issues, checkID)
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
	if oldIssue.Id != newIssue.Id ||
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

// initForwarder initializes the forwarder for sending health reports to Datadog intake
func (h *healthPlatformImpl) initForwarder(reqs Requires) error {
	hostname, err := reqs.Hostname.Get(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}

	h.forwarder = newForwarder(reqs.Config, h, reqs.Log, hostname)
	return nil
}

// ============================================================================
// HTTP API Handlers
// ============================================================================

// getIssuesHandler handles GET /health-platform/issues
func (h *healthPlatformImpl) getIssuesHandler(w http.ResponseWriter, _ *http.Request) {
	count, issues := h.GetAllIssues()

	response := struct {
		Count  int                              `json:"count"`
		Issues map[string]*healthplatform.Issue `json:"issues"`
	}{
		Count:  count,
		Issues: issues,
	}

	h.writeJSONResponse(w, http.StatusOK, response)
}

// writeJSONResponse writes a JSON response with the given status code
func (h *healthPlatformImpl) writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.log.Warn("Failed to encode JSON response: " + err.Error())
	}
}

// ============================================================================
// Flare Provider
// ============================================================================

// fillFlare adds health platform issues to the flare archive
func (h *healthPlatformImpl) fillFlare(fb flaretypes.FlareBuilder) error {
	count, issues := h.GetAllIssues()

	// Only create the file if there are issues
	if count == 0 {
		return nil
	}

	hostname, err := h.hostnameProvider.Get(context.Background())
	if err != nil {
		hostname = "unknown"
	}

	report := &healthplatform.HealthReport{
		EventType: "agent.health",
		EmittedAt: time.Now().UTC().Format(time.RFC3339),
		Host: &healthplatform.HostInfo{
			Hostname:     hostname,
			AgentVersion: version.AgentVersion,
		},
		Issues: issues,
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	return fb.AddFile("health-platform-issues.json", data)
}

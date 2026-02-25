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
	"os"
	"path/filepath"
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
	_ "github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues/rofspermissions"
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

	// Persistence
	persistedIssues map[string]*PersistedIssue // Persisted issues with status tracking
	persistencePath string                     // Path to the persistence file

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

// IssueState is a type alias for the proto enum healthplatform.IssueState.
type IssueState = healthplatform.IssueState

const (
	IssueStateNew      = healthplatform.IssueState_ISSUE_STATE_NEW
	IssueStateOngoing  = healthplatform.IssueState_ISSUE_STATE_ONGOING
	IssueStateResolved = healthplatform.IssueState_ISSUE_STATE_RESOLVED

	// resolvedIssueTTL is the time after which resolved issues are pruned from the persistence file.
	resolvedIssueTTL = 24 * time.Hour
)

var issueStateToString = map[IssueState]string{
	IssueStateNew:      "new",
	IssueStateOngoing:  "ongoing",
	IssueStateResolved: "resolved",
}

func issueStateFromString(s string) IssueState {
	for k, v := range issueStateToString {
		if v == s {
			return k
		}
	}
	return 0
}

// pruneOldResolvedIssues removes resolved issues older than resolvedIssueTTL from the given map.
// It modifies the map in place.
func pruneOldResolvedIssues(issues map[string]*PersistedIssue) {
	now := time.Now()
	for checkID, persisted := range issues {
		if persisted == nil || persisted.State != IssueStateResolved || persisted.ResolvedAt == "" {
			continue
		}
		resolvedAt, err := time.Parse(time.RFC3339, persisted.ResolvedAt)
		if err != nil {
			continue
		}
		if now.Sub(resolvedAt) > resolvedIssueTTL {
			delete(issues, checkID)
		}
	}
}

// PersistedIssue tracks issue state for disk persistence.
// Custom JSON marshaling keeps the on-disk format unchanged (state as string).
type PersistedIssue struct {
	IssueID    string     `json:"issue_id"`
	State      IssueState `json:"state"`
	FirstSeen  string     `json:"first_seen"`
	LastSeen   string     `json:"last_seen"`
	ResolvedAt string     `json:"resolved_at,omitempty"`
}

// MarshalJSON converts the proto IssueState enum to its string representation for disk.
func (p *PersistedIssue) MarshalJSON() ([]byte, error) {
	type Alias PersistedIssue
	return json.Marshal(&struct {
		*Alias
		State string `json:"state"`
	}{
		Alias: (*Alias)(p),
		State: issueStateToString[p.State],
	})
}

// UnmarshalJSON parses the string state from disk back to the proto enum.
func (p *PersistedIssue) UnmarshalJSON(data []byte) error {
	type Alias PersistedIssue
	aux := &struct {
		*Alias
		State string `json:"state"`
	}{
		Alias: (*Alias)(p),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	p.State = issueStateFromString(aux.State)
	return nil
}

// persistedIssueToProto converts a local PersistedIssue to the proto PersistedIssue
// used in the Issue payload. The proto type uses *string for ResolvedAt.
func persistedIssueToProto(p *PersistedIssue) *healthplatform.PersistedIssue {
	pi := &healthplatform.PersistedIssue{
		State:     p.State,
		FirstSeen: p.FirstSeen,
		LastSeen:  p.LastSeen,
	}
	if p.ResolvedAt != "" {
		pi.ResolvedAt = &p.ResolvedAt
	}
	return pi
}

// PersistedState is the full state written to disk
type PersistedState struct {
	UpdatedAt string                     `json:"updated_at"`
	Issues    map[string]*PersistedIssue `json:"issues"`
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

	// Build persistence path: <run_path>/health-platform/issues.json
	runPath := reqs.Config.GetString("run_path")
	persistencePath := filepath.Join(runPath, "health-platform", "issues.json")
	// Create unified issue registry and register all self-registered modules
	issueRegistry := issuesmod.NewRegistry()
	for _, module := range issuesmod.GetAllModules(reqs.Config) {
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
		issues:    make(map[string]*healthplatform.Issue), // Initialize issues map
		issuesMux: sync.RWMutex{},                         // Initialize issues mutex

		// Persistence
		persistedIssues: make(map[string]*PersistedIssue),
		persistencePath: persistencePath,
	}

	// Initialize check runner (must be after comp is created as it needs the reporter interface)
	comp.checkRunner = newCheckRunner(reqs.Log, comp)

	// Register built-in health checks from issue modules
	for _, check := range issueRegistry.GetBuiltInChecks() {
		if err := comp.RegisterCheck(check.ID, check.Name, check.CheckFn, check.Interval, check.Once); err != nil {
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

	// Load persisted issues from disk
	if err := h.loadFromDisk(); err != nil {
		h.log.Warn("Failed to load persisted issues: " + err.Error())
	}

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
func (h *healthPlatformImpl) RegisterCheck(checkID string, checkName string, checkFn healthplatformdef.HealthCheckFunc, interval time.Duration, once bool) error {
	return h.checkRunner.RegisterCheck(checkID, checkName, checkFn, interval, once)
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

	// Only log and update persistence if there was actually an issue to clear
	existed := false
	if _, ok := h.issues[checkID]; ok {
		existed = true
		h.log.Info("Cleared issue for check: " + checkID)
	}
	delete(h.issues, checkID)

	// Update persisted issue status to resolved
	if persisted := h.persistedIssues[checkID]; persisted != nil {
		persisted.State = IssueStateResolved
		persisted.ResolvedAt = time.Now().Format(time.RFC3339)
	}

	h.issuesMux.Unlock()

	// Persist to disk if there was a change
	if existed {
		if err := h.saveToDisk(); err != nil {
			h.log.Warn("Failed to persist issues to disk: " + err.Error())
		}
	}
}

// ClearAllIssues clears all issues (useful for testing or when all issues are resolved)
func (h *healthPlatformImpl) ClearAllIssues() {
	h.issuesMux.Lock()

	now := time.Now().Format(time.RFC3339)

	// Mark all persisted issues as resolved
	for _, persisted := range h.persistedIssues {
		if persisted != nil && persisted.State != IssueStateResolved {
			persisted.State = IssueStateResolved
			persisted.ResolvedAt = now
		}
	}

	h.issues = make(map[string]*healthplatform.Issue)
	h.log.Info("Cleared all issues")

	h.issuesMux.Unlock()

	// Persist to disk
	if err := h.saveToDisk(); err != nil {
		h.log.Warn("Failed to persist issues to disk: " + err.Error())
	}
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

// storeIssue stores an issue for a specific check and persists to disk
func (h *healthPlatformImpl) storeIssue(checkID string, issue *healthplatform.Issue) {
	h.issuesMux.Lock()

	now := time.Now().Format(time.RFC3339)

	// Add timestamp to issue if present
	if issue != nil {
		issue.DetectedAt = now
		// Update telemetry
		h.metrics.issuesCounter.Add(1, checkID)
	}

	h.issues[checkID] = issue

	// Update persisted issue with state tracking
	existing := h.persistedIssues[checkID]
	if existing == nil {
		// No previous record - new issue
		h.persistedIssues[checkID] = &PersistedIssue{
			IssueID:   issue.Id,
			State:     IssueStateNew,
			FirstSeen: now,
			LastSeen:  now,
		}
	} else if existing.State == IssueStateResolved {
		// Previously resolved - treat as a new occurrence
		existing.IssueID = issue.Id
		existing.State = IssueStateNew
		existing.FirstSeen = now
		existing.LastSeen = now
		existing.ResolvedAt = ""
	} else if existing.IssueID != issue.Id {
		// The check is reporting a different issue ID than what was previously stored.
		// This is an internal agent bug: a given check should always report the same issue type.
		_ = h.log.Errorf("health platform: check %s changed issue ID from %q to %q; this is an agent bug",
			checkID, existing.IssueID, issue.Id)
		existing.IssueID = issue.Id
		existing.State = IssueStateNew
		existing.FirstSeen = now
		existing.LastSeen = now
		existing.ResolvedAt = ""
	} else {
		// Same issue still active - update to ongoing
		existing.State = IssueStateOngoing
		existing.LastSeen = now
	}

	// Populate the proto PersistedIssue on the issue for the health report payload
	persisted := h.persistedIssues[checkID]
	if persisted != nil {
		issue.PersistedIssue = persistedIssueToProto(persisted)
	}

	h.issuesMux.Unlock()

	// Persist to disk (outside lock to avoid blocking)
	if err := h.saveToDisk(); err != nil {
		h.log.Warn("Failed to persist issues to disk: " + err.Error())
	}
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
// Persistence Methods
// ============================================================================

// loadFromDisk loads persisted issues from disk
func (h *healthPlatformImpl) loadFromDisk() error {
	data, err := os.ReadFile(h.persistencePath)
	if err != nil {
		if os.IsNotExist(err) {
			h.log.Info("No persisted issues file found, starting fresh")
			return nil
		}
		return fmt.Errorf("failed to read persisted issues: %w", err)
	}

	var state PersistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("failed to unmarshal persisted issues: %w", err)
	}

	h.issuesMux.Lock()
	defer h.issuesMux.Unlock()

	// Restore persisted issues and prune any resolved issues older than the TTL
	h.persistedIssues = state.Issues
	pruneOldResolvedIssues(h.persistedIssues)
	activeCount := 0
	for checkID, persisted := range state.Issues {
		// Only restore active issues (not resolved ones)
		if persisted.State != IssueStateResolved && persisted.IssueID != "" {
			// Rebuild issue from registry using the issue ID
			issue, err := h.issueRegistry.BuildIssue(persisted.IssueID, nil)
			if err != nil {
				h.log.Warn(fmt.Sprintf("Failed to rebuild issue %s for check %s: %v", persisted.IssueID, checkID, err))
				continue
			}
			issue.PersistedIssue = persistedIssueToProto(persisted)
			h.issues[checkID] = issue
			activeCount++
		}
	}

	h.log.Info(fmt.Sprintf("Loaded %d persisted issues from disk (%d active)", len(state.Issues), activeCount))
	return nil
}

// saveToDisk persists issues to disk using atomic write (temp file + rename)
func (h *healthPlatformImpl) saveToDisk() error {
	h.issuesMux.RLock()
	// Make a deep copy to avoid race conditions during marshaling
	issuesCopy := make(map[string]*PersistedIssue, len(h.persistedIssues))
	for k, v := range h.persistedIssues {
		if v != nil {
			copied := *v
			issuesCopy[k] = &copied
		}
	}
	h.issuesMux.RUnlock()

	// Prune resolved issues older than the TTL before writing to disk
	pruneOldResolvedIssues(issuesCopy)

	state := PersistedState{
		UpdatedAt: time.Now().Format(time.RFC3339),
		Issues:    issuesCopy,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal issues: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(h.persistencePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create persistence directory: %w", err)
	}

	// Write to temp file first
	tmpPath := h.persistencePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, h.persistencePath); err != nil {
		os.Remove(tmpPath) // Clean up temp file on failure
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

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

	agentVersion := version.AgentVersion
	report := &healthplatform.HealthReport{
		EventType: "agent.health",
		EmittedAt: time.Now().UTC().Format(time.RFC3339),
		Host: &healthplatform.HostInfo{
			Hostname:     hostname,
			AgentVersion: &agentVersion,
		},
		Issues: issues,
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	return fb.AddFile("health-platform-issues.json", data)
}

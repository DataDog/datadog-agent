// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package storeimpl implements the health-platform store component interface
package storeimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	noopimpl "github.com/DataDog/datadog-agent/comp/healthplatform/store/noop-impl"
	configenv "github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/version"
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
	agentFlavor      string                      // Agent flavor captured at construction time

	// Issue tracking
	issues       map[string]*healthplatform.Issue // IssueID → active Issue
	issuesByName map[string]map[string]struct{}   // IssueName → set of active IssueIDs
	issuesMux    sync.RWMutex                     // Mutex for thread-safe access to issues

	// Persistence
	persistedIssues map[string]*PersistedIssue // Persisted issues with status tracking
	persistence     issuesPersistence          // Persistence strategy (disk or noop)

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

	// persistedStateVersion is the on-disk schema version written by this binary.
	// loadFromDisk refuses to load files with a different version (no migration).
	persistedStateVersion = 2
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
// Custom JSON marshaling keeps the on-disk state field as a string.
// The proto fields (Title through Remediation) are populated on every write so
// that issues can be fully restored on restart without re-running the template.
type PersistedIssue struct {
	IssueType  string     `json:"issue_type"`
	State      IssueState `json:"state"`
	FirstSeen  string     `json:"first_seen"`
	LastSeen   string     `json:"last_seen"`
	ResolvedAt string     `json:"resolved_at,omitempty"`

	// Proto fields — mirror of healthplatform.Issue, written on every ReportIssue.
	IssueName   string          `json:"issue_name,omitempty"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	Category    string          `json:"category,omitempty"`
	Location    string          `json:"location,omitempty"`
	Severity    string          `json:"severity,omitempty"`
	Source      string          `json:"source,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Extra       json.RawMessage `json:"extra,omitempty"`
	Remediation json.RawMessage `json:"remediation,omitempty"`
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

// PersistedState is the full state written to disk.
// Version must equal persistedStateVersion; files with a different version
// are logged and ignored on load (no migration).
type PersistedState struct {
	Version   int                        `json:"version"`
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
		noop := noopimpl.NewNoopHealthPlatform()
		return Provides{
			Comp: noop,
			APIGetIssues: api.NewAgentEndpointProvider(
				noop.GetIssuesHandler,
				"/health-platform/issues",
				"GET",
			),
			FlareProvider: flaretypes.NewProvider(noop.FillFlare),
		}, nil
	}

	reqs.Log.Info("Creating health platform component")

	// Select persistence strategy: noop on Kubernetes (emptyDir makes disk persistence meaningless),
	// disk-based elsewhere so issues survive agent restarts.
	// Operators who mount run_path as a durable volume (hostPath, PVC) can opt in to disk
	// persistence on Kubernetes by setting health_platform.persist_on_kubernetes: true.
	var persistence issuesPersistence
	persistOnKubernetes := reqs.Config.GetBool("health_platform.persist_on_kubernetes")
	if configenv.IsKubernetes() && !persistOnKubernetes {
		reqs.Log.Info("Running on Kubernetes: health platform persistence disabled (set health_platform.persist_on_kubernetes: true to enable)")
		persistence = &noopPersistence{}
	} else {
		runPath := reqs.Config.GetString("run_path")
		persistencePath := filepath.Join(runPath, "health-platform", "issues.json")
		persistence = newDiskPersistence(persistencePath, reqs.Log)
	}

	// Initialize the health platform implementation
	comp := &healthPlatformImpl{
		// Core dependencies
		config:           reqs.Config,
		log:              reqs.Log,
		telemetry:        reqs.Telemetry,
		hostnameProvider: reqs.Hostname,
		agentFlavor:      flavor.GetFlavor(),

		// Issue tracking
		issues:       make(map[string]*healthplatform.Issue),
		issuesByName: make(map[string]map[string]struct{}),
		issuesMux:    sync.RWMutex{},

		// Persistence
		persistedIssues: make(map[string]*PersistedIssue),
		persistence:     persistence,
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
			[]string{"issue_type"},
			"Number of health issues detected",
		),
	}

	// Return the component wrapped in Provides with API endpoints.
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

// ReportIssue records a new or ongoing issue keyed by issue.Id. The caller is
// responsible for building the complete proto Issue (template lookup, field
// population). issue.IssueName is used as the issue-type key for telemetry and
// persistence.
func (h *healthPlatformImpl) ReportIssue(issue *healthplatform.Issue) error {
	if issue == nil {
		return errors.New("issue cannot be nil")
	}
	if issue.Id == "" {
		return errors.New("issue id cannot be empty")
	}
	if issue.IssueName == "" {
		return errors.New("issue name cannot be empty")
	}

	h.issuesMux.RLock()
	previousIssue := h.issues[issue.Id]
	h.issuesMux.RUnlock()

	h.handleIssueStateChange(issue.Source, previousIssue, issue)
	h.storeIssue(issue.IssueName, issue)
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
			result[checkID] = proto.Clone(issue).(*healthplatform.Issue)
			count++
		} else {
			result[checkID] = nil
		}
	}
	return count, result
}

// GetIssue returns the issue for a specific check (nil if no issue)
func (h *healthPlatformImpl) GetIssue(checkID string) *healthplatform.Issue {
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

// ResolveIssue clears the issue for a specific check (useful when issue is resolved)
func (h *healthPlatformImpl) ResolveIssue(issueID string) {
	h.issuesMux.Lock()

	// Only log and update persistence if there was actually an issue to clear
	existed := false
	if _, ok := h.issues[issueID]; ok {
		existed = true
		h.log.Info("Cleared issue: " + issueID)
	}
	delete(h.issues, issueID)

	// Remove from name index
	if persisted := h.persistedIssues[issueID]; persisted != nil {
		delete(h.issuesByName[persisted.IssueType], issueID)
	}

	// Update persisted issue status to resolved
	if persisted := h.persistedIssues[issueID]; persisted != nil {
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

// ResolveAllIssues clears all issues (useful for testing or when all issues are resolved)
func (h *healthPlatformImpl) ResolveAllIssues() {
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
	h.issuesByName = make(map[string]map[string]struct{})
	h.log.Info("Cleared all issues")

	h.issuesMux.Unlock()

	// Persist to disk
	if err := h.saveToDisk(); err != nil {
		h.log.Warn("Failed to persist issues to disk: " + err.Error())
	}
}

// GetActiveIssueIDsByIssueName returns the IDs of all currently active issues
// with the given IssueName. Used by bundle.go to compute the initial set of
// issue IDs for the scheduler after an agent restart.
func (h *healthPlatformImpl) GetActiveIssueIDsByIssueName(issueName string) []string {
	h.issuesMux.RLock()
	defer h.issuesMux.RUnlock()
	ids := h.issuesByName[issueName]
	result := make([]string, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	return result
}

// ============================================================================
// Internal Helper Methods
// ============================================================================

// handleIssueStateChange detects state changes and logs appropriately.
// source is the reporting integration/component name, used for log context.
func (h *healthPlatformImpl) handleIssueStateChange(source string, oldIssue, newIssue *healthplatform.Issue) {
	if oldIssue == nil && newIssue == nil {
		return
	}

	if newIssue != nil && oldIssue == nil {
		h.log.Info("Health platform: NEW issue from " + source + ": " + newIssue.Title + " (" + newIssue.Severity + ")")
		return
	}

	if newIssue == nil && oldIssue != nil {
		h.log.Info("Health platform: issue RESOLVED from " + source)
		return
	}

	if oldIssue.Title != newIssue.Title ||
		oldIssue.Severity != newIssue.Severity ||
		oldIssue.Description != newIssue.Description {
		h.log.Info("Health platform: issue CHANGED from " + source + ": " + newIssue.Title + " (" + newIssue.Severity + ")")
	}
}

// storeIssue stores an issue keyed by issue.Id (the unique instance key set by ReportIssue).
// issueType is the template identifier, used for telemetry tagging and persistence.
func (h *healthPlatformImpl) storeIssue(issueType string, issue *healthplatform.Issue) {
	h.issuesMux.Lock()

	issueID := issue.Id
	now := time.Now().Format(time.RFC3339)
	issue.DetectedAt = now
	h.metrics.issuesCounter.Add(1, issueType)

	h.issues[issueID] = issue
	if h.issuesByName[issueType] == nil {
		h.issuesByName[issueType] = make(map[string]struct{})
	}
	h.issuesByName[issueType][issueID] = struct{}{}

	existing := h.persistedIssues[issueID]
	if existing == nil {
		h.persistedIssues[issueID] = &PersistedIssue{
			IssueType: issueType,
			State:     IssueStateNew,
			FirstSeen: now,
			LastSeen:  now,
		}
	} else if existing.State == IssueStateResolved {
		existing.IssueType = issueType
		existing.State = IssueStateNew
		existing.FirstSeen = now
		existing.LastSeen = now
		existing.ResolvedAt = ""
	} else if existing.IssueType != issueType {
		h.log.Warnf("health platform: issue %s changed type from %s to %s; resetting to new", issueID, existing.IssueType, issueType)
		existing.IssueType = issueType
		existing.State = IssueStateNew
		existing.FirstSeen = now
		existing.LastSeen = now
		existing.ResolvedAt = ""
	} else {
		existing.State = IssueStateOngoing
		existing.LastSeen = now
	}

	if persisted := h.persistedIssues[issueID]; persisted != nil {
		issue.PersistedIssue = persistedIssueToProto(persisted)
		persisted.IssueName = issue.IssueName
		persisted.Title = issue.Title
		persisted.Description = issue.Description
		persisted.Category = issue.Category
		persisted.Location = issue.Location
		persisted.Severity = issue.Severity
		persisted.Source = issue.Source
		persisted.Tags = issue.Tags
		if issue.Extra != nil {
			if raw, err := json.Marshal(issue.Extra); err == nil {
				persisted.Extra = raw
			} else {
				h.log.Warnf("health platform: failed to serialize Extra for issue %s: %v", issueID, err)
			}
		}
		if issue.Remediation != nil {
			if raw, err := json.Marshal(issue.Remediation); err == nil {
				persisted.Remediation = raw
			} else {
				h.log.Warnf("health platform: failed to serialize Remediation for issue %s: %v", issueID, err)
			}
		}
	}

	h.issuesMux.Unlock()

	if err := h.saveToDisk(); err != nil {
		h.log.Warn("Failed to persist issues to disk: " + err.Error())
	}
}

// ============================================================================
// Persistence Methods
// ============================================================================

// loadFromDisk loads persisted issues via the persistence layer.
// Files whose version field differs from persistedStateVersion are ignored.
func (h *healthPlatformImpl) loadFromDisk() error {
	state, err := h.persistence.load()
	if err != nil {
		return err
	}
	if state == nil {
		return nil
	}

	if state.Version != persistedStateVersion {
		h.log.Warnf("Incompatible health-platform persistence file (version %d, expected %d); ignoring and starting fresh",
			state.Version, persistedStateVersion)
		return nil
	}

	h.issuesMux.Lock()
	defer h.issuesMux.Unlock()

	h.persistedIssues = state.Issues
	pruneOldResolvedIssues(h.persistedIssues)
	activeCount := 0
	for issueID, persisted := range state.Issues {
		if persisted.State == IssueStateResolved || persisted.IssueType == "" {
			continue
		}
		var issue *healthplatform.Issue
		if persisted.Title != "" || persisted.Source != "" {
			issue = &healthplatform.Issue{
				Id:          issueID,
				IssueName:   persisted.IssueName,
				Title:       persisted.Title,
				Description: persisted.Description,
				Category:    persisted.Category,
				Location:    persisted.Location,
				Severity:    persisted.Severity,
				Source:      persisted.Source,
				Tags:        persisted.Tags,
			}
			if len(persisted.Extra) > 0 {
				issue.Extra = &structpb.Struct{}
				if err := json.Unmarshal(persisted.Extra, issue.Extra); err != nil {
					h.log.Warnf("health platform: failed to restore Extra for issue %s: %v", issueID, err)
					issue.Extra = nil
				}
			}
			if len(persisted.Remediation) > 0 {
				issue.Remediation = &healthplatform.Remediation{}
				if err := json.Unmarshal(persisted.Remediation, issue.Remediation); err != nil {
					h.log.Warnf("health platform: failed to restore Remediation for issue %s: %v", issueID, err)
					issue.Remediation = nil
				}
			}
		} else {
			// Version-2 files always cache proto fields; this handles the edge
			// case of a file written before caching was introduced.
			issue = &healthplatform.Issue{
				Id:        issueID,
				IssueName: persisted.IssueType,
				Source:    persisted.Source,
			}
		}
		issue.Id = issueID
		issue.PersistedIssue = persistedIssueToProto(persisted)
		h.issues[issueID] = issue
		// Prefer IssueName (written by current code); fall back to IssueType for old JSON files.
		nameKey := persisted.IssueName
		if nameKey == "" {
			nameKey = persisted.IssueType
		}
		if h.issuesByName[nameKey] == nil {
			h.issuesByName[nameKey] = make(map[string]struct{})
		}
		h.issuesByName[nameKey][issueID] = struct{}{}
		activeCount++
	}

	h.log.Info(fmt.Sprintf("Loaded %d persisted issues (%d active)", len(state.Issues), activeCount))
	return nil
}

// saveToDisk persists the current issue state via the persistence layer
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

	// Prune resolved issues older than the TTL before saving
	pruneOldResolvedIssues(issuesCopy)

	state := PersistedState{
		Version:   persistedStateVersion,
		UpdatedAt: time.Now().Format(time.RFC3339),
		Issues:    issuesCopy,
	}

	return h.persistence.save(&state)
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
func (h *healthPlatformImpl) fillFlare(_ context.Context, fb flaretypes.FlareBuilder) error {
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
		Service:   h.agentFlavor,
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

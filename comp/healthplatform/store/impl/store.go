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
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
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

// healthPlatformImpl implements the health platform component.
type healthPlatformImpl struct {
	// Core dependencies
	config           config.Component
	log              log.Component
	telemetry        telemetry.Component
	hostnameProvider hostnameinterface.Component
	agentFlavor      string

	// Issue tracking: lean proto (no Extra/Remediation in hot path)
	issues       map[string]*healthplatform.Issue // IssueID → active Issue
	issuesByName map[string][]string              // IssueName → active IssueIDs
	issuesMux    sync.RWMutex

	// Persistence: slim state metadata + separate JSON maps for lazy hydration
	persistedIssues map[string]*PersistedIssue  // IssueID → lifecycle state
	extraJSON       map[string]json.RawMessage   // IssueID → Extra as raw JSON
	remediationJSON map[string]json.RawMessage   // IssueID → Remediation as raw JSON
	persistence     issuesPersistence

	// Metrics
	metrics telemetryMetrics
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

// PersistedIssue tracks the lifecycle state of an issue in memory.
// Proto payload fields (Title, Description, Extra, Remediation, etc.) are not stored here;
// active issues carry them in h.issues, and Extra/Remediation live in h.extraJSON /
// h.remediationJSON for lazy hydration at read time.
type PersistedIssue struct {
	IssueType  string
	State      IssueState
	FirstSeen  string
	LastSeen   string
	ResolvedAt string
}

// diskIssue is the on-disk representation of a single issue. It is a superset of
// PersistedIssue: proto payload fields are included so that active issues can be fully
// restored after an agent restart without re-running health checks.
// Custom JSON marshaling keeps the state field as a human-readable string.
// The on-disk JSON schema is identical to the former PersistedIssue, so no version bump
// is needed.
type diskIssue struct {
	IssueType   string          `json:"issue_type"`
	State       IssueState      `json:"state"`
	FirstSeen   string          `json:"first_seen"`
	LastSeen    string          `json:"last_seen"`
	ResolvedAt  string          `json:"resolved_at,omitempty"`
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

func (d *diskIssue) MarshalJSON() ([]byte, error) {
	type Alias diskIssue
	return json.Marshal(&struct {
		*Alias
		State string `json:"state"`
	}{
		Alias: (*Alias)(d),
		State: issueStateToString[d.State],
	})
}

func (d *diskIssue) UnmarshalJSON(data []byte) error {
	type Alias diskIssue
	aux := &struct {
		*Alias
		State string `json:"state"`
	}{
		Alias: (*Alias)(d),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	d.State = issueStateFromString(aux.State)
	return nil
}

// persistedIssueToProto converts a PersistedIssue to the proto PersistedIssue embedded
// in Issue payloads. The proto type uses *string for ResolvedAt.
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
// Version must equal persistedStateVersion; files with a different version are ignored on load.
type PersistedState struct {
	Version   int                   `json:"version"`
	UpdatedAt string                `json:"updated_at"`
	Issues    map[string]*diskIssue `json:"issues"`
}

// pruneOldResolvedIssues removes resolved issues older than resolvedIssueTTL from the map.
func pruneOldResolvedIssues(issues map[string]*diskIssue) {
	now := time.Now()
	for id, d := range issues {
		if d == nil || d.State != IssueStateResolved || d.ResolvedAt == "" {
			continue
		}
		resolvedAt, err := time.Parse(time.RFC3339, d.ResolvedAt)
		if err != nil {
			continue
		}
		if now.Sub(resolvedAt) > resolvedIssueTTL {
			delete(issues, id)
		}
	}
}

// appendUnique appends id to ids only if not already present.
func appendUnique(ids []string, id string) []string {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

// removeID removes the first occurrence of id from ids.
func removeID(ids []string, id string) []string {
	for i, existing := range ids {
		if existing == id {
			return append(ids[:i], ids[i+1:]...)
		}
	}
	return ids
}

// ============================================================================
// Constructor
// ============================================================================

// NewComponent creates a new health-platform component.
func NewComponent(reqs Requires) (Provides, error) {
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

	comp := &healthPlatformImpl{
		config:           reqs.Config,
		log:              reqs.Log,
		telemetry:        reqs.Telemetry,
		hostnameProvider: reqs.Hostname,
		agentFlavor:      flavor.GetFlavor(),

		issues:       make(map[string]*healthplatform.Issue),
		issuesByName: make(map[string][]string),
		issuesMux:    sync.RWMutex{},

		persistedIssues: make(map[string]*PersistedIssue),
		extraJSON:       make(map[string]json.RawMessage),
		remediationJSON: make(map[string]json.RawMessage),
		persistence:     persistence,
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: comp.start,
		OnStop:  comp.stop,
	})

	comp.metrics = telemetryMetrics{
		issuesCounter: reqs.Telemetry.NewCounter(
			"health_platform",
			"issues_detected",
			[]string{"issue_type"},
			"Number of health issues detected",
		),
	}

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

func (h *healthPlatformImpl) start(_ context.Context) error {
	h.log.Info("Starting health platform component")
	if err := h.loadFromDisk(); err != nil {
		h.log.Warn("Failed to load persisted issues: " + err.Error())
	}
	return nil
}

func (h *healthPlatformImpl) stop(_ context.Context) error {
	h.log.Info("Stopping health platform component")
	return nil
}

// ============================================================================
// Core Public API
// ============================================================================

// ReportIssue records a new or ongoing issue keyed by issue.Id.
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

// GetAllIssues returns the count and all issues from all checks (indexed by check ID).
func (h *healthPlatformImpl) GetAllIssues() (int, map[string]*healthplatform.Issue) {
	h.issuesMux.RLock()
	defer h.issuesMux.RUnlock()

	count := 0
	result := make(map[string]*healthplatform.Issue)
	for checkID, issue := range h.issues {
		if issue != nil {
			clone := proto.Clone(issue).(*healthplatform.Issue)
			h.hydrateIssue(clone)
			result[checkID] = clone
			count++
		} else {
			result[checkID] = nil
		}
	}
	return count, result
}

// GetIssue returns the issue for a specific check (nil if no issue).
func (h *healthPlatformImpl) GetIssue(checkID string) *healthplatform.Issue {
	h.issuesMux.RLock()
	defer h.issuesMux.RUnlock()

	issue := h.issues[checkID]
	if issue == nil {
		return nil
	}
	clone := proto.Clone(issue).(*healthplatform.Issue)
	h.hydrateIssue(clone)
	return clone
}

// hydrateIssue populates Extra and Remediation on a cloned issue from the JSON maps.
// The hot store keeps issues without these fields; they are reconstructed on demand at read time.
func (h *healthPlatformImpl) hydrateIssue(issue *healthplatform.Issue) {
	if raw := h.extraJSON[issue.Id]; len(raw) > 0 {
		issue.Extra = &structpb.Struct{}
		if err := json.Unmarshal(raw, issue.Extra); err != nil {
			h.log.Warnf("health platform: failed to hydrate Extra for issue %s: %v", issue.Id, err)
			issue.Extra = nil
		}
	}
	if raw := h.remediationJSON[issue.Id]; len(raw) > 0 {
		issue.Remediation = &healthplatform.Remediation{}
		if err := json.Unmarshal(raw, issue.Remediation); err != nil {
			h.log.Warnf("health platform: failed to hydrate Remediation for issue %s: %v", issue.Id, err)
			issue.Remediation = nil
		}
	}
}

// ============================================================================
// Clear Methods
// ============================================================================

// ResolveIssue marks an issue as resolved and removes it from the active set.
func (h *healthPlatformImpl) ResolveIssue(issueID string) {
	h.issuesMux.Lock()

	existed := false
	if _, ok := h.issues[issueID]; ok {
		existed = true
		h.log.Info("Cleared issue: " + issueID)
	}
	delete(h.issues, issueID)
	delete(h.extraJSON, issueID)
	delete(h.remediationJSON, issueID)

	if persisted := h.persistedIssues[issueID]; persisted != nil {
		h.issuesByName[persisted.IssueType] = removeID(h.issuesByName[persisted.IssueType], issueID)
		persisted.State = IssueStateResolved
		persisted.ResolvedAt = time.Now().Format(time.RFC3339)
	}

	h.issuesMux.Unlock()

	if existed {
		if err := h.saveToDisk(); err != nil {
			h.log.Warn("Failed to persist issues to disk: " + err.Error())
		}
	}
}

// ResolveAllIssues clears all active issues.
func (h *healthPlatformImpl) ResolveAllIssues() {
	h.issuesMux.Lock()

	now := time.Now().Format(time.RFC3339)
	for _, persisted := range h.persistedIssues {
		if persisted != nil && persisted.State != IssueStateResolved {
			persisted.State = IssueStateResolved
			persisted.ResolvedAt = now
		}
	}

	h.issues = make(map[string]*healthplatform.Issue)
	h.issuesByName = make(map[string][]string)
	h.extraJSON = make(map[string]json.RawMessage)
	h.remediationJSON = make(map[string]json.RawMessage)
	h.log.Info("Cleared all issues")

	h.issuesMux.Unlock()

	if err := h.saveToDisk(); err != nil {
		h.log.Warn("Failed to persist issues to disk: " + err.Error())
	}
}

// GetActiveIssueIDsByIssueName returns the IDs of all currently active issues with the given IssueName.
func (h *healthPlatformImpl) GetActiveIssueIDsByIssueName(issueName string) []string {
	h.issuesMux.RLock()
	defer h.issuesMux.RUnlock()
	ids := h.issuesByName[issueName]
	result := make([]string, len(ids))
	copy(result, ids)
	return result
}

// ============================================================================
// Internal Helper Methods
// ============================================================================

func (h *healthPlatformImpl) handleIssueStateChange(source string, oldIssue, newIssue *healthplatform.Issue) {
	if oldIssue == nil && newIssue == nil {
		return
	}
	if newIssue != nil && oldIssue == nil {
		h.log.Info("Health platform: NEW issue from " + source + ": " + newIssue.Title + " (" + newIssue.Severity.String() + ")")
		return
	}
	if newIssue == nil && oldIssue != nil {
		h.log.Info("Health platform: issue RESOLVED from " + source)
		return
	}
	if oldIssue.Title != newIssue.Title ||
		oldIssue.Severity != newIssue.Severity ||
		oldIssue.Description != newIssue.Description {
		h.log.Info("Health platform: issue CHANGED from " + source + ": " + newIssue.Title + " (" + newIssue.Severity.String() + ")")
	}
}

// storeIssue stores an issue keyed by issue.Id.
func (h *healthPlatformImpl) storeIssue(issueType string, issue *healthplatform.Issue) {
	h.issuesMux.Lock()

	issueID := issue.Id
	now := time.Now().Format(time.RFC3339)
	issue.DetectedAt = now
	h.metrics.issuesCounter.Add(1, issueType)

	// Clone before storing so nilling Extra/Remediation on the stored copy doesn't
	// mutate the caller's proto (reporters may reuse the same *Issue across calls).
	stored := proto.Clone(issue).(*healthplatform.Issue)
	h.issues[issueID] = stored
	h.issuesByName[issueType] = appendUnique(h.issuesByName[issueType], issueID)

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

	stored.PersistedIssue = persistedIssueToProto(h.persistedIssues[issueID])

	// Keep Extra and Remediation as raw JSON; strip from the stored lean proto.
	if issue.Extra != nil {
		if raw, err := json.Marshal(issue.Extra); err == nil {
			h.extraJSON[issueID] = raw
		} else {
			h.log.Warnf("health platform: failed to serialize Extra for issue %s: %v", issueID, err)
			delete(h.extraJSON, issueID)
		}
	} else {
		delete(h.extraJSON, issueID)
	}
	if issue.Remediation != nil {
		if raw, err := json.Marshal(issue.Remediation); err == nil {
			h.remediationJSON[issueID] = raw
		} else {
			h.log.Warnf("health platform: failed to serialize Remediation for issue %s: %v", issueID, err)
			delete(h.remediationJSON, issueID)
		}
	} else {
		delete(h.remediationJSON, issueID)
	}
	stored.Extra = nil
	stored.Remediation = nil

	h.issuesMux.Unlock()

	if err := h.saveToDisk(); err != nil {
		h.log.Warn("Failed to persist issues to disk: " + err.Error())
	}
}

// ============================================================================
// Persistence Methods
// ============================================================================

// loadFromDisk restores issue state from the persistence layer.
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

	pruneOldResolvedIssues(state.Issues)

	h.issuesMux.Lock()
	defer h.issuesMux.Unlock()

	activeCount := 0
	for issueID, entry := range state.Issues {
		if entry == nil {
			continue
		}

		h.persistedIssues[issueID] = &PersistedIssue{
			IssueType:  entry.IssueType,
			State:      entry.State,
			FirstSeen:  entry.FirstSeen,
			LastSeen:   entry.LastSeen,
			ResolvedAt: entry.ResolvedAt,
		}

		if entry.State == IssueStateResolved || entry.IssueType == "" {
			continue
		}

		var issue *healthplatform.Issue
		if entry.Title != "" || entry.Source != "" {
			issue = &healthplatform.Issue{
				Id:          issueID,
				IssueName:   entry.IssueName,
				Title:       entry.Title,
				Description: entry.Description,
				Category:    entry.Category,
				Location:    entry.Location,
				Severity:    healthplatform.IssueSeverity(healthplatform.IssueSeverity_value[entry.Severity]),
				Source:      entry.Source,
				Tags:        entry.Tags,
			}
		} else {
			// Fallback for files written before proto fields were cached (pre-v2).
			issue = &healthplatform.Issue{
				Id:        issueID,
				IssueName: entry.IssueType,
				Source:    entry.Source,
			}
		}
		issue.PersistedIssue = persistedIssueToProto(h.persistedIssues[issueID])
		h.issues[issueID] = issue

		if len(entry.Extra) > 0 {
			h.extraJSON[issueID] = entry.Extra
		}
		if len(entry.Remediation) > 0 {
			h.remediationJSON[issueID] = entry.Remediation
		}

		nameKey := entry.IssueName
		if nameKey == "" {
			nameKey = entry.IssueType
		}
		h.issuesByName[nameKey] = append(h.issuesByName[nameKey], issueID)
		activeCount++
	}

	h.log.Info(fmt.Sprintf("Loaded %d persisted issues (%d active)", len(state.Issues), activeCount))
	return nil
}

// saveToDisk persists the current issue state via the persistence layer.
// For each active issue, proto payload fields are pulled from h.issues and the JSON maps
// so that the on-disk format remains complete for restart recovery.
func (h *healthPlatformImpl) saveToDisk() error {
	h.issuesMux.RLock()
	entries := make(map[string]*diskIssue, len(h.persistedIssues))
	for id, p := range h.persistedIssues {
		if p == nil {
			continue
		}
		entry := &diskIssue{
			IssueType:  p.IssueType,
			State:      p.State,
			FirstSeen:  p.FirstSeen,
			LastSeen:   p.LastSeen,
			ResolvedAt: p.ResolvedAt,
		}
		if issue := h.issues[id]; issue != nil {
			entry.IssueName = issue.IssueName
			entry.Title = issue.Title
			entry.Description = issue.Description
			entry.Category = issue.Category
			entry.Location = issue.Location
			entry.Severity = issue.Severity.String()
			entry.Source = issue.Source
			entry.Tags = issue.Tags
			entry.Extra = h.extraJSON[id]
			entry.Remediation = h.remediationJSON[id]
		}
		entries[id] = entry
	}
	h.issuesMux.RUnlock()

	pruneOldResolvedIssues(entries)

	state := PersistedState{
		Version:   persistedStateVersion,
		UpdatedAt: time.Now().Format(time.RFC3339),
		Issues:    entries,
	}
	return h.persistence.save(&state)
}

// ============================================================================
// HTTP API Handlers
// ============================================================================

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

func (h *healthPlatformImpl) fillFlare(_ context.Context, fb flaretypes.FlareBuilder) error {
	count, issues := h.GetAllIssues()
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

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
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/selfident"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	noopimpl "github.com/DataDog/datadog-agent/comp/healthplatform/store/noop-impl"
	configenv "github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// Requires defines the dependencies for the health-platform component
type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    config.Component
	Log       log.Component
	Telemetry telemetry.Component
	Hostname  hostnameinterface.Component
	// Workloadmeta resolves this agent's own DaemonSet/cluster identity (see
	// selfident). Every binary that wires this bundle must also wire
	// workloadmeta's fx module, since option.Option[workloadmeta.Component]
	// is provided by that module, not this one.
	Workloadmeta option.Option[workloadmeta.Component]
}

// Provides defines the output of the health-platform component
type Provides struct {
	compdef.Out
	Comp          healthplatformdef.Component
	APIGetIssues  api.AgentEndpointProvider
	FlareProvider flaretypes.Provider
}

// storedIssue is the in-memory record for an active issue.
// Extra and Remediation are kept as raw JSON to avoid structpb heap allocations
// for the process lifetime; they are rehydrated on demand at read time.
type storedIssue struct {
	issue           *healthplatform.Issue // lean proto — Extra and Remediation are nil
	extraJSON       json.RawMessage
	remediationJSON json.RawMessage
}

// healthPlatformImpl implements the health platform component.
// It aggregates health issues reported by various agent components and integrations.
// The component provides methods to report issues, retrieve them, and manage the health monitoring lifecycle.
type healthPlatformImpl struct {
	// Core dependencies
	config           config.Component            // Config component for accessing configuration
	log              log.Component               // Logger for health platform operations
	telemetry        telemetry.Component         // Telemetry component for metrics collection
	hostnameProvider hostnameinterface.Component // Hostname provider for runtime resolution
	agentFlavor      string                      // Agent flavor captured at construction time
	selfIdent        *selfident.SelfIdent        // Resolves this agent's DaemonSet/cluster identity

	// Issue tracking: dehydrated at ReportIssue, rehydrated on GetAllIssues/GetIssue.
	issues       map[string]*storedIssue // IssueID → active issue (lean proto + raw JSON)
	issuesByName map[string][]string     // IssueName → active IssueIDs
	issuesMux    sync.RWMutex

	// Persistence: lifecycle state only — proto payload is not stored here.
	persistedIssues map[string]*PersistedIssue // IssueID → lifecycle state
	persistence     issuesPersistence

	// Issue observers: receive issue events outside issuesMux.
	observersMu sync.RWMutex
	observers   []healthplatformdef.IssuesObserver

	// Metrics
	metrics telemetryMetrics
}

type telemetryMetrics struct {
	issuesCounter telemetry.Counter
}

// IssueState is a type alias for the proto enum healthplatform.IssueState.
type IssueState = healthplatform.IssueState

const (
	IssueStateActive   = healthplatform.IssueState_ISSUE_STATE_ACTIVE
	IssueStateResolved = healthplatform.IssueState_ISSUE_STATE_RESOLVED

	// resolvedIssueTTL is the time after which resolved issues are pruned from the persistence file.
	resolvedIssueTTL = 24 * time.Hour

	// persistedStateVersion is the on-disk schema version written by this binary.
	// loadFromDisk refuses to load files with a different version (no migration).
	persistedStateVersion = 2
)

var issueStateToString = map[IssueState]string{
	IssueStateActive:   "active",
	IssueStateResolved: "resolved",
}

func issueStateFromString(s string) IssueState {
	if s == "resolved" {
		return IssueStateResolved
	}
	return IssueStateActive
}

// PersistedIssue tracks the lifecycle state of an issue.
// It is both the in-memory and on-disk representation; proto payload fields are
// intentionally omitted because IssueIDs are deterministic — when the agent
// restarts, health checks re-run and call ReportIssue with the same ID, at which
// point storeIssue picks up the existing firstSeen/state from this struct.
//
// IssueType (this struct) is a legacy name for the issue's IssueName, kept as-is
// for on-disk compatibility — it is not the proto Issue.IssueType field. ProtoIssueType
// carries that proto field so resolved tombstones (ResolveIssue, ResolveAllIssues,
// loadFromDisk) can forward it same as they already do for IssueName.
type PersistedIssue struct {
	IssueID        string     `json:"issue_id"`
	IssueType      string     `json:"issue_type"`
	ProtoIssueType string     `json:"proto_issue_type,omitempty"`
	State          IssueState `json:"state"`
	FirstSeen      string     `json:"first_seen"`
	LastSeen       string     `json:"last_seen"`
	ResolvedAt     string     `json:"resolved_at,omitempty"`
}

// MarshalJSON serialises State as a human-readable string.
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

// UnmarshalJSON parses the string state back to the proto enum.
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

// persistedIssueToProto converts a PersistedIssue to the proto PersistedIssue
// embedded in Issue payloads.
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
	Version   int                        `json:"version"`
	UpdatedAt string                     `json:"updated_at"`
	Issues    map[string]*PersistedIssue `json:"issues"`
}

// pruneOldResolvedIssues removes resolved issues older than resolvedIssueTTL from the given map.
// It modifies the map in place.
func pruneOldResolvedIssues(issues map[string]*PersistedIssue) {
	now := time.Now()
	for id, p := range issues {
		if p == nil || p.State != IssueStateResolved || p.ResolvedAt == "" {
			continue
		}
		resolvedAt, err := time.Parse(time.RFC3339, p.ResolvedAt)
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
// It initializes the component with its dependencies and configures telemetry metrics.
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
		config:           reqs.Config,
		log:              reqs.Log,
		telemetry:        reqs.Telemetry,
		hostnameProvider: reqs.Hostname,
		agentFlavor:      flavor.GetFlavor(),
		selfIdent:        selfident.New(reqs.Workloadmeta),

		issues:       make(map[string]*storedIssue),
		issuesByName: make(map[string][]string),
		issuesMux:    sync.RWMutex{},

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

// RegisterIssuesObserver appends an observer. Observers registered after
// OnStart will miss events that occurred before registration.
func (h *healthPlatformImpl) RegisterIssuesObserver(obs healthplatformdef.IssuesObserver) {
	h.observersMu.Lock()
	h.observers = append(h.observers, obs)
	h.observersMu.Unlock()
}

// notifyResolved writes a resolved tombstone to each observer's ResolvedCh.
// Must be called outside issuesMux.
func (h *healthPlatformImpl) notifyResolved(resolved *healthplatform.Issue) {
	h.observersMu.RLock()
	obs := h.observers
	h.observersMu.RUnlock()
	for _, o := range obs {
		if o.ResolvedCh != nil {
			select {
			case o.ResolvedCh <- resolved:
			default:
				h.log.Warnf("health platform: resolved channel full, %s recoverable from disk", resolved.Id)
			}
		}
	}
}

// ============================================================================
// Core Public API
// ============================================================================

// ReportIssue records a new or ongoing issue keyed by issue.Id. The caller is
// responsible for building the complete proto Issue (template lookup, field
// population), including issue.IssueType. issue.IssueName is used as the
// issue-type key for telemetry and persistence.
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

	h.enrichWithClusterIdentity(issue)

	h.issuesMux.RLock()
	var previousIssue *healthplatform.Issue
	if prev := h.issues[issue.Id]; prev != nil {
		previousIssue = prev.issue
	}
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
	for checkID, stored := range h.issues {
		if stored != nil {
			clone := proto.Clone(stored.issue).(*healthplatform.Issue)
			hydrateIssue(clone, stored)
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

	stored := h.issues[checkID]
	if stored == nil {
		return nil
	}
	clone := proto.Clone(stored.issue).(*healthplatform.Issue)
	hydrateIssue(clone, stored)
	return clone
}

// hydrateIssue populates Extra and Remediation on a cloned issue from the storedIssue JSON.
// The hot store keeps issues without these fields; they are reconstructed on demand at read time.
func hydrateIssue(issue *healthplatform.Issue, stored *storedIssue) {
	if len(stored.extraJSON) > 0 {
		issue.Extra = &structpb.Struct{}
		if err := json.Unmarshal(stored.extraJSON, issue.Extra); err != nil {
			issue.Extra = nil
		}
	}
	if len(stored.remediationJSON) > 0 {
		issue.Remediation = &healthplatform.Remediation{}
		if err := json.Unmarshal(stored.remediationJSON, issue.Remediation); err != nil {
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

	stateChanged := false
	var resolved *healthplatform.Issue

	if _, ok := h.issues[issueID]; ok {
		h.log.Info("Cleared issue: " + issueID)
		delete(h.issues, issueID)
		stateChanged = true
	}

	if persisted := h.persistedIssues[issueID]; persisted != nil {
		h.issuesByName[persisted.IssueType] = removeID(h.issuesByName[persisted.IssueType], issueID)
		if persisted.State != IssueStateResolved {
			persisted.State = IssueStateResolved
			persisted.ResolvedAt = time.Now().Format(time.RFC3339)
			stateChanged = true
		}

		resolved = &healthplatform.Issue{
			Id:             issueID,
			IssueName:      persisted.IssueType,
			IssueType:      persisted.ProtoIssueType,
			PersistedIssue: persistedIssueToProto(persisted),
		}
	}

	h.issuesMux.Unlock()

	if resolved != nil {
		h.notifyResolved(resolved)
	}

	if stateChanged {
		if err := h.saveToDisk(); err != nil {
			h.log.Warn("Failed to persist issues to disk: " + err.Error())
		}
	}
}

// ResolveAllIssues marks every active issue as resolved.
func (h *healthPlatformImpl) ResolveAllIssues() {
	h.issuesMux.Lock()

	now := time.Now().Format(time.RFC3339)
	var resolved []*healthplatform.Issue

	for _, persisted := range h.persistedIssues {
		if persisted != nil && persisted.State != IssueStateResolved {
			persisted.State = IssueStateResolved
			persisted.ResolvedAt = now
			resolved = append(resolved, &healthplatform.Issue{
				Id:             persisted.IssueID,
				IssueName:      persisted.IssueType,
				IssueType:      persisted.ProtoIssueType,
				PersistedIssue: persistedIssueToProto(persisted),
			})
		}
	}

	h.issues = make(map[string]*storedIssue)
	h.issuesByName = make(map[string][]string)
	h.log.Info("Cleared all issues")

	h.issuesMux.Unlock()

	for _, t := range resolved {
		h.notifyResolved(t)
	}

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
	result := make([]string, len(ids))
	copy(result, ids)
	return result
}

// IssueDiscriminator returns the identifier issue ids should be scoped by;
// see the Component interface doc for the collapse rationale.
func (h *healthPlatformImpl) IssueDiscriminator(hostID string) string {
	return h.selfIdent.IssueDiscriminator(hostID)
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

// enrichWithClusterIdentity stamps deployment_id/cluster_id into the issue's
// Extra and Tags, so that when a module keys issue.Id by deployment_id
// (collapsing issues across a DaemonSet), the UI can still identify which
// DaemonSet/cluster the collapsed issue came from. No-op on non-Kubernetes
// agents, where both resolve to empty.
func (h *healthPlatformImpl) enrichWithClusterIdentity(issue *healthplatform.Issue) {
	deploymentID := h.selfIdent.DeploymentID()
	clusterID := h.selfIdent.ClusterID()
	if deploymentID == "" && clusterID == "" {
		return
	}

	if issue.Extra == nil {
		issue.Extra = &structpb.Struct{}
	}
	if issue.Extra.Fields == nil {
		issue.Extra.Fields = make(map[string]*structpb.Value)
	}
	if deploymentID != "" {
		issue.Extra.Fields["deployment_id"] = structpb.NewStringValue(deploymentID)
		issue.Tags = appendUnique(issue.Tags, "deployment_id:"+deploymentID)
	}
	if clusterID != "" {
		issue.Extra.Fields["cluster_id"] = structpb.NewStringValue(clusterID)
		issue.Tags = appendUnique(issue.Tags, "cluster_id:"+clusterID)
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

	h.issuesByName[issueType] = appendUnique(h.issuesByName[issueType], issueID)

	existing := h.persistedIssues[issueID]
	if existing == nil {
		h.persistedIssues[issueID] = &PersistedIssue{
			IssueID:   issueID,
			IssueType: issueType,
			State:     IssueStateActive,
			FirstSeen: now,
			LastSeen:  now,
		}
	} else if existing.State == IssueStateResolved {
		existing.IssueID = issueID
		existing.IssueType = issueType
		existing.State = IssueStateActive
		existing.FirstSeen = now
		existing.LastSeen = now
		existing.ResolvedAt = ""
	} else if existing.IssueType != issueType {
		h.log.Warnf("health platform: issue %s changed type from %s to %s; resetting", issueID, existing.IssueType, issueType)
		existing.IssueID = issueID
		existing.IssueType = issueType
		existing.State = IssueStateActive
		existing.FirstSeen = now
		existing.LastSeen = now
		existing.ResolvedAt = ""
	} else {
		existing.LastSeen = now
	}
	h.persistedIssues[issueID].ProtoIssueType = issue.IssueType

	// Clone before storing to avoid external mutations (reporters may reuse the same *Issue).
	// Serialize Extra/Remediation to raw JSON and strip from the lean clone so that
	// structpb heap allocations are not retained for the process lifetime.
	si := &storedIssue{}
	if issue.Extra != nil {
		if raw, err := json.Marshal(issue.Extra); err == nil {
			si.extraJSON = raw
		} else {
			h.log.Warnf("health platform: failed to serialize Extra for issue %s: %v", issueID, err)
		}
	}
	if issue.Remediation != nil {
		if raw, err := json.Marshal(issue.Remediation); err == nil {
			si.remediationJSON = raw
		} else {
			h.log.Warnf("health platform: failed to serialize Remediation for issue %s: %v", issueID, err)
		}
	}
	lean := proto.Clone(issue).(*healthplatform.Issue)
	lean.Extra = nil
	lean.Remediation = nil
	lean.PersistedIssue = persistedIssueToProto(h.persistedIssues[issueID])
	si.issue = lean
	h.issues[issueID] = si

	h.issuesMux.Unlock()

	if err := h.saveToDisk(); err != nil {
		h.log.Warn("Failed to persist issues to disk: " + err.Error())
	}
}

// ============================================================================
// Persistence Methods
// ============================================================================

// loadFromDisk restores lifecycle state from the persistence layer.
// Proto payload (issue title, description, etc.) is not stored on disk — IssueIDs are
// deterministic, so health checks re-running after restart will call ReportIssue with the
// same ID and storeIssue will pick up firstSeen/state from the restored PersistedIssue.
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

	activeCount := 0
	var resolvedIssues []*healthplatform.Issue

	for issueID, persisted := range state.Issues {
		if persisted == nil {
			continue
		}
		persisted.IssueID = issueID
		h.persistedIssues[issueID] = persisted

		if persisted.IssueType == "" {
			continue
		}

		if persisted.State == IssueStateResolved {
			resolvedIssues = append(resolvedIssues, &healthplatform.Issue{
				Id:             issueID,
				IssueName:      persisted.IssueType,
				IssueType:      persisted.ProtoIssueType,
				PersistedIssue: persistedIssueToProto(persisted),
			})
			continue
		}

		h.issuesByName[persisted.IssueType] = append(h.issuesByName[persisted.IssueType], issueID)
		activeCount++
	}

	h.issuesMux.Unlock()

	h.log.Info(fmt.Sprintf("Loaded %d persisted issues (%d active, %d resolved pending send)",
		len(state.Issues), activeCount, len(resolvedIssues)))

	for _, t := range resolvedIssues {
		h.notifyResolved(t)
	}

	return nil
}

// saveToDisk persists the current lifecycle state via the persistence layer.
// Only state metadata is written; proto payload fields are omitted because they are
// repopulated by health checks on the next agent start.
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

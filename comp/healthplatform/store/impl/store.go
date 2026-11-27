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
	"sync/atomic"
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
	"github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/utils/selfident"
	issuesmod "github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	noopimpl "github.com/DataDog/datadog-agent/comp/healthplatform/store/noop-impl"
	remotedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/remote/def"
	configenv "github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// Requires defines the dependencies for the health-platform component
type Requires struct {
	Lifecycle    compdef.Lifecycle
	Config       config.Component
	Log          log.Component
	Telemetry    telemetry.Component
	Hostname     hostnameinterface.Component
	RemoteIssues remotedef.Component
	// Workloadmeta resolves this agent's own DaemonSet/cluster identity (see
	// selfident). Optional because only selfident's kubeapiserver build reads
	// it: on flavors without that tag (iot, heroku, the cloudfoundry cluster
	// agent, serverless-init) selfident is a no-op that ignores it entirely, so
	// requiring it would make those binaries depend on a component they have no
	// use for. Every binary that wires this bundle happens to provide it today,
	// so in practice it is non-nil wherever it is actually read.
	Workloadmeta workloadmeta.Component `optional:"true"`
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
//
// The store cannot know which of its callers are on a critical path (e.g.
// Autodiscovery reports while holding its config lock), so mutations never
// block: they are queued for a single worker that applies them in order, and
// lifecycle state is persisted on a timer rather than on every change.
type healthPlatformImpl struct {
	// Core dependencies
	config           config.Component            // Config component for accessing configuration
	log              log.Component               // Logger for health platform operations
	telemetry        telemetry.Component         // Telemetry component for metrics collection
	hostnameProvider hostnameinterface.Component // Hostname provider for runtime resolution
	agentFlavor      string                      // Agent flavor captured at construction time
	selfIdent        *selfident.SelfIdent        // Resolves this agent's DaemonSet/cluster identity

	// Issue tracking: dehydrated when stored, rehydrated on GetAllIssues/GetIssue.
	issues       map[string]*storedIssue // IssueID → active issue (lean proto + raw JSON)
	issuesByName map[string][]string     // IssueName → active IssueIDs
	issuesMux    sync.RWMutex

	// Mutation queue: ReportIssue/ResolveIssue/ResolveAllIssues push here and
	// return; the worker applies ops in order. Full queue → op dropped and counted.
	ops          chan func()
	workerStop   chan struct{}
	workerDone   chan struct{}
	dropLogLimit *pkglog.Limit

	// closeMu makes send's check of closing and its send one step relative
	// to stop. An atomic bool would not: a sender could read closing == false,
	// stop could then set it and the worker drain and exit, and the send would
	// land in a channel nobody reads, counted as accepted but never applied.
	// send holds the read lock across the check and the non-blocking send,
	// so callers still proceed concurrently; stop takes the write lock once to
	// set closing, after which every enqueue drops.
	closeMu sync.RWMutex
	closing bool

	// Persistence: lifecycle state only — proto payload is not stored here.
	persistedIssues map[string]*PersistedIssue // IssueID → lifecycle state
	persistence     issuesPersistence
	unsavedChanges  atomic.Bool // persistedIssues changed since the last save
	persistInterval time.Duration
	persisterStop   chan struct{}
	persisterDone   chan struct{}

	// Issue observers: receive issue events outside issuesMux.
	observersMu sync.RWMutex
	observers   []healthplatformdef.IssuesObserver

	// Metrics
	metrics telemetryMetrics
}

type telemetryMetrics struct {
	issuesCounter     telemetry.Counter
	activeIssues      telemetry.Gauge
	issuesResolved    telemetry.Counter
	droppedOperations telemetry.Counter
}

// IssueState is a type alias for the proto enum healthplatform.IssueState.
type IssueState = healthplatform.IssueState

const (
	IssueStateActive   = healthplatform.IssueState_ISSUE_STATE_ACTIVE
	IssueStateResolved = healthplatform.IssueState_ISSUE_STATE_RESOLVED

	// resolvedIssueTTL is the time after which resolved issues are pruned from the persistence file.
	resolvedIssueTTL = 24 * time.Hour

	// persistedStateVersion is the on-disk schema version written by this binary.
	// restorePersistedState refuses to load files with a different version (no migration).
	persistedStateVersion = 2

	// opsQueueSize bounds the mutations waiting for the worker. Callers drop
	// (and count) further mutations rather than wait for room.
	opsQueueSize = 1000

	// persistInterval is how often lifecycle state is saved when it changed.
	// A crash loses at most this much state; health checks re-report on restart.
	persistInterval = 10 * time.Second
)

// wrapPersistence lets tests wrap the persistence of the stores NewComponent
// creates, see SetSaveDelay.
var wrapPersistence = func(p issuesPersistence) issuesPersistence { return p }

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
// It is both the in-memory and persisted representation; detailed proto payload fields are
// intentionally omitted because IssueIDs are deterministic — when the agent
// restarts, health checks re-run and call ReportIssue with the same ID, at which
// point storeIssue picks up the existing firstSeen/state from this struct.
//
// IssueType (this struct) is a legacy name for the issue's IssueName, kept as-is
// for on-disk compatibility — it is not the proto Issue.IssueType field. ProtoIssueType
// carries that proto field so resolved tombstones (ResolveIssue, ResolveAllIssues,
// restorePersistedState) can forward it same as they already do for IssueName.
type PersistedIssue struct {
	IssueID        string     `json:"issue_id"`
	IssueType      string     `json:"issue_type"`
	ProtoIssueType string     `json:"proto_issue_type,omitempty"`
	Category       string     `json:"category,omitempty"`
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

// PersistedState is the full lifecycle state stored by a persistence implementation.
// Version must equal persistedStateVersion; state with a different version is ignored on load.
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

	agentFlavor := flavor.GetFlavor()
	selfIdent := selfident.New(reqs.Workloadmeta)
	isKubernetes := configenv.IsKubernetes()
	persistOnKubernetes := reqs.Config.GetBool("health_platform.persist_on_kubernetes")
	persistRemotelyOnKubernetes := reqs.Config.GetBool("health_platform.persist_remotely_on_kubernetes")
	var persistence issuesPersistence = &noopPersistence{}
	switch {
	case isKubernetes && persistRemotelyOnKubernetes:
		if persistOnKubernetes {
			reqs.Log.Warn("Both health_platform.persist_on_kubernetes and health_platform.persist_remotely_on_kubernetes are enabled; remote persistence takes precedence")
		}
		reqs.Log.Info("Running on Kubernetes: remote health platform persistence enabled")
		persistence = newRemotePersistence(reqs.RemoteIssues)
	case !isKubernetes || persistOnKubernetes:
		runPath := reqs.Config.GetString("run_path")
		persistencePath := filepath.Join(runPath, "health-platform", "issues.json")
		persistence = newDiskPersistence(persistencePath, reqs.Log)
	default:
		reqs.Log.Info("Running on Kubernetes: health platform persistence disabled")
	}

	persistence = wrapPersistence(persistence)

	// Initialize the health platform implementation
	comp := newHealthPlatform(reqs.Log, reqs.Telemetry, reqs.Hostname, agentFlavor, selfIdent, persistence)
	comp.config = reqs.Config

	// Register lifecycle hooks for component start/stop
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: comp.start,
		OnStop:  comp.stop,
	})

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

// newHealthPlatform builds the store with its telemetry and an empty queue.
// Nothing is applied until start runs the worker.
func newHealthPlatform(
	logger log.Component,
	tel telemetry.Component,
	hostname hostnameinterface.Component,
	agentFlavor string,
	selfIdent *selfident.SelfIdent,
	persistence issuesPersistence,
) *healthPlatformImpl {
	return &healthPlatformImpl{
		log:              logger,
		telemetry:        tel,
		hostnameProvider: hostname,
		agentFlavor:      agentFlavor,
		selfIdent:        selfIdent,

		issues:       make(map[string]*storedIssue),
		issuesByName: make(map[string][]string),

		ops:          make(chan func(), opsQueueSize),
		workerStop:   make(chan struct{}),
		workerDone:   make(chan struct{}),
		dropLogLimit: pkglog.NewLogLimit(10, 10*time.Minute),

		persistedIssues: make(map[string]*PersistedIssue),
		persistence:     persistence,
		persistInterval: persistInterval,
		persisterStop:   make(chan struct{}),
		persisterDone:   make(chan struct{}),

		metrics: telemetryMetrics{
			issuesCounter: tel.NewCounter(
				"health_platform",
				"issues_detected",
				[]string{"issue_type", "severity"},
				"Number of health issues detected",
			),
			activeIssues: tel.NewGauge(
				"health_platform", "active_issues", []string{"issue_type"}, "Number of active health issue instances"),
			issuesResolved: tel.NewCounter(
				"health_platform", "issues_resolved", []string{"issue_type"}, "Number of health issue instances resolved"),
			droppedOperations: tel.NewCounter(
				"health_platform",
				"operations_dropped",
				[]string{"operation"},
				"Number of issue reports and resolutions dropped because the store queue was full",
			),
		},
	}
}

// ============================================================================
// Lifecycle Methods
// ============================================================================

// start restores persisted state, then starts the worker so mutations queued
// before start apply on top of the restored state.
func (h *healthPlatformImpl) start(ctx context.Context) error {
	h.log.Info("Starting health platform component")
	if err := h.loadPersistedState(ctx); err != nil {
		h.log.Warn("Failed to restore persisted issues: " + err.Error())
	}
	// Start resolving this agent's identity now so the first issues get stamped.
	h.selfIdent.DeploymentIDNonBlocking()
	h.selfIdent.ClusterIDNonBlocking()
	go h.runWorker()
	go h.runPersister()
	return nil
}

// stop applies every queued mutation, then saves state one last time.
func (h *healthPlatformImpl) stop(ctx context.Context) error {
	h.log.Info("Stopping health platform component")
	// Close admission before signalling the worker so that any goroutine still
	// holding its enqueue read-lock completes its channel send before the worker
	// begins draining. After Unlock() every subsequent enqueue drops immediately,
	// and the worker's drain loop is guaranteed to see all accepted ops.
	h.closeMu.Lock()
	h.closing = true
	h.closeMu.Unlock()
	close(h.workerStop)
	select {
	case <-h.workerDone:
	case <-ctx.Done():
		// Return on time, but keep the final save after the drain: saving now
		// would persist a snapshot missing resolutions the worker has yet to
		// apply, and those issues would be active again after a restart.
		go func() {
			<-h.workerDone
			close(h.persisterStop)
		}()
		return ctx.Err()
	}
	close(h.persisterStop)
	select {
	case <-h.persisterDone:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// runWorker applies queued mutations in order. On stop it drains what is
// still queued so a clean shutdown loses nothing.
func (h *healthPlatformImpl) runWorker() {
	defer close(h.workerDone)
	for {
		select {
		case op := <-h.ops:
			h.apply(op)
		case <-h.workerStop:
			for {
				select {
				case op := <-h.ops:
					h.apply(op)
				default:
					return
				}
			}
		}
	}
}

// apply runs op, recovering a panic so one bad issue can't stop the worker.
func (h *healthPlatformImpl) apply(op func()) {
	defer func() {
		if r := recover(); r != nil {
			h.log.Errorf("health platform: panic while applying an issue update: %v", r)
		}
	}()
	op()
}

// enqueue hands op to the worker without waiting. Returns true when the op was
// queued, false when it was dropped (queue full or shutdown in progress).
// Losing an update beats stalling a caller that may be on a critical path.
func (h *healthPlatformImpl) enqueue(operation string, op func()) bool {
	if h.send(op) {
		return true
	}
	h.metrics.droppedOperations.Inc(operation)
	// Log after closeMu is released: a stalled log write must not hold up
	// stop, which needs the write lock.
	if h.dropLogLimit.ShouldLog() {
		h.log.Warnf("health platform: update queue full or stopping (capacity %d), dropping %s", cap(h.ops), operation)
	}
	return false
}

// send queues op unless the store is stopping or the queue is full.
func (h *healthPlatformImpl) send(op func()) bool {
	h.closeMu.RLock()
	defer h.closeMu.RUnlock()
	if h.closing {
		return false
	}
	select {
	case h.ops <- op:
		return true
	default:
		return false
	}
}

// runPersister saves lifecycle state every persistInterval if it changed, and
// once more on stop, so a burst of reports costs one write instead of one each.
func (h *healthPlatformImpl) runPersister() {
	defer close(h.persisterDone)
	ticker := time.NewTicker(h.persistInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			h.persistIfChanged()
		case <-h.persisterStop:
			h.persistIfChanged()
			return
		}
	}
}

// persistIfChanged saves lifecycle state if it changed since the last save.
func (h *healthPlatformImpl) persistIfChanged() {
	if !h.unsavedChanges.Swap(false) {
		return
	}
	if err := h.saveToDisk(); err != nil {
		// Retry on the next tick even if nothing else changes.
		h.unsavedChanges.Store(true)
		h.log.Warn("Failed to persist issues to disk: " + err.Error())
	}
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
//
// It only validates and queues the issue: the worker enriches and stores it
// shortly after. An error means either the issue is invalid (nil, empty ID, or
// missing IssueName) or, with ErrQueueFull, that the queue is full and the
// report was dropped.
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

	// Clone so the worker owns its copy: reporters may reuse the same *Issue.
	issue = proto.Clone(issue).(*healthplatform.Issue)
	if !h.enqueue("report_issue", func() { h.applyReport(issue) }) {
		return healthplatformdef.ErrQueueFull
	}
	return nil
}

// applyReport enriches and stores a queued issue. Runs on the worker.
func (h *healthPlatformImpl) applyReport(issue *healthplatform.Issue) {
	h.enrichWithClusterIdentity(issue)

	h.issuesMux.RLock()
	var previousIssue *healthplatform.Issue
	if prev := h.issues[issue.Id]; prev != nil {
		previousIssue = prev.issue
	}
	h.issuesMux.RUnlock()

	h.handleIssueStateChange(issue.Source, previousIssue, issue)
	h.storeIssue(issue.IssueName, issue)
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
			// Re-enrich at read time so that issues first stored before cluster
			// identity resolved still carry up-to-date deployment_id/cluster_id.
			// appendUnique makes tag enrichment idempotent.
			h.enrichWithClusterIdentity(clone)
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
	// Like GetAllIssues: stamp identity resolved after the issue was stored.
	h.enrichWithClusterIdentity(clone)
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

// ResolveIssue queues the issue to be marked resolved and removed from the
// active set, without waiting for the worker to apply it. It is dropped, and
// counted, when the queue is full.
func (h *healthPlatformImpl) ResolveIssue(issueID string) {
	if issueID == "" {
		return
	}
	h.enqueue("resolve_issue", func() { h.resolveIssue(issueID) })
}

// resolveIssue marks an issue as resolved and removes it from the active set.
// Runs on the worker.
func (h *healthPlatformImpl) resolveIssue(issueID string) {
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
			h.recordResolution(persisted)
			stateChanged = true
		}
		h.setActiveGauge(persisted.ProtoIssueType)

		resolved = &healthplatform.Issue{
			Id:             issueID,
			IssueName:      persisted.IssueType,
			IssueType:      persisted.ProtoIssueType,
			Category:       persisted.Category,
			PersistedIssue: persistedIssueToProto(persisted),
		}
	}

	h.issuesMux.Unlock()

	if stateChanged {
		h.unsavedChanges.Store(true)
	}

	if resolved != nil {
		h.notifyResolved(resolved)
	}
}

// setActiveGauge counts persisted active instances by proto type while issuesMux is held.
func (h *healthPlatformImpl) setActiveGauge(issueType string) {
	count := 0
	for _, persisted := range h.persistedIssues {
		if persisted != nil && persisted.IssueType != "" && persisted.State != IssueStateResolved && persisted.ProtoIssueType == issueType {
			count++
		}
	}
	h.metrics.activeIssues.Set(float64(count), metricIssueType(issueType))
}

// metricIssueType keeps telemetry tags non-empty for issues reported without a proto IssueType.
func metricIssueType(issueType string) string {
	if issueType == "" {
		return "unknown"
	}
	return issueType
}

func (h *healthPlatformImpl) recordResolution(persisted *PersistedIssue) {
	h.metrics.issuesResolved.Inc(metricIssueType(persisted.ProtoIssueType))
}

// ResolveAllIssues queues every active issue to be marked resolved, without
// waiting for the worker to apply it. It counts as a resolution in telemetry,
// so do not call it on Agent shutdown.
func (h *healthPlatformImpl) ResolveAllIssues() {
	h.enqueue("resolve_all_issues", h.resolveAllIssues)
}

// resolveAllIssues marks every active issue as resolved. Runs on the worker.
func (h *healthPlatformImpl) resolveAllIssues() {
	h.issuesMux.Lock()

	now := time.Now().Format(time.RFC3339)
	var resolved []*healthplatform.Issue
	activeTypes := make(map[string]struct{})

	for _, persisted := range h.persistedIssues {
		if persisted != nil && persisted.State != IssueStateResolved {
			persisted.State = IssueStateResolved
			persisted.ResolvedAt = now
			h.recordResolution(persisted)
			activeTypes[persisted.ProtoIssueType] = struct{}{}
			resolved = append(resolved, &healthplatform.Issue{
				Id:             persisted.IssueID,
				IssueName:      persisted.IssueType,
				IssueType:      persisted.ProtoIssueType,
				Category:       persisted.Category,
				PersistedIssue: persistedIssueToProto(persisted),
			})
		}
	}

	h.issues = make(map[string]*storedIssue)
	h.issuesByName = make(map[string][]string)
	for issueType := range activeTypes {
		h.setActiveGauge(issueType)
	}
	h.log.Info("Cleared all issues")

	h.issuesMux.Unlock()

	if len(resolved) > 0 {
		h.unsavedChanges.Store(true)
	}

	for _, t := range resolved {
		h.notifyResolved(t)
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
// see the Component interface doc for the collapse rationale. selfIdent only
// reports this agent's DaemonSet uid, or "" when there is none, so the per-host
// fallback lives in issuesmod.IssueDiscriminator — shared with the issue
// modules that scope their own ids.
func (h *healthPlatformImpl) IssueDiscriminator(hostID string) string {
	return issuesmod.IssueDiscriminator(h.selfIdent, hostID)
}

// ResourceIdentity returns the resource_type/resource_id describing the
// entity this agent process represents; see the Component interface doc.
func (h *healthPlatformImpl) ResourceIdentity(hostname string) (string, string) {
	return issuesmod.ResourceIdentity(h.agentFlavor, h.selfIdent, hostname)
}

// ResourceID returns the resource_id outbound reports carry for this agent;
// see the Component interface doc.
func (h *healthPlatformImpl) ResourceID() string {
	hostname, err := h.hostnameProvider.Get(context.Background())
	if err != nil {
		hostname = ""
	}
	_, resourceID := h.ResourceIdentity(hostname)
	return resourceID
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
// agents, where both resolve to empty, and while identity is still unresolved.
func (h *healthPlatformImpl) enrichWithClusterIdentity(issue *healthplatform.Issue) {
	deploymentID := h.selfIdent.DeploymentIDNonBlocking()
	clusterID := h.selfIdent.ClusterIDNonBlocking()
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
// It takes ownership of issue. Runs on the worker.
func (h *healthPlatformImpl) storeIssue(issueType string, issue *healthplatform.Issue) {
	h.issuesMux.Lock()

	issueID := issue.Id
	now := time.Now().Format(time.RFC3339)
	issue.DetectedAt = now
	h.metrics.issuesCounter.Add(1, issueType, issue.Severity.String())

	h.issuesByName[issueType] = appendUnique(h.issuesByName[issueType], issueID)

	existing := h.persistedIssues[issueID]
	previousIssueType := issue.IssueType
	if existing != nil {
		previousIssueType = existing.ProtoIssueType
	}
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
	h.persistedIssues[issueID].Category = issue.Category
	h.setActiveGauge(issue.IssueType)
	if previousIssueType != issue.IssueType {
		h.setActiveGauge(previousIssueType)
	}

	// Serialize Extra/Remediation to raw JSON and strip them from the stored
	// issue so that structpb heap allocations are not retained for the process lifetime.
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
	issue.Extra = nil
	issue.Remediation = nil
	issue.PersistedIssue = persistedIssueToProto(h.persistedIssues[issueID])
	si.issue = issue
	h.issues[issueID] = si

	h.issuesMux.Unlock()

	h.unsavedChanges.Store(true)
}

// ============================================================================
// Persistence Methods
// ============================================================================

// loadPersistedState restores lifecycle state from the persistence layer.
func (h *healthPlatformImpl) loadPersistedState(ctx context.Context) error {
	state, err := h.persistence.load(ctx)
	if err != nil {
		return err
	}
	return h.restorePersistedState(state)
}

// restorePersistedState restores lifecycle state into the in-memory store.
// Proto payload (issue title, description, etc.) is not stored in lifecycle state —
// IssueIDs are deterministic, so health checks re-running after restart will call
// ReportIssue with the same ID and storeIssue will pick up firstSeen/state from the
// restored PersistedIssue.
func (h *healthPlatformImpl) restorePersistedState(state *PersistedState) error {
	if state == nil {
		return nil
	}

	if state.Version != persistedStateVersion {
		h.log.Warnf("Incompatible health-platform persistence state (version %d, expected %d); ignoring and starting fresh",
			state.Version, persistedStateVersion)
		return nil
	}

	pruneOldResolvedIssues(state.Issues)

	h.issuesMux.Lock()

	activeCount := 0
	var resolvedIssues []*healthplatform.Issue
	activeTypes := make(map[string]struct{})

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
				Category:       persisted.Category,
				PersistedIssue: persistedIssueToProto(persisted),
			})
			continue
		}

		h.issuesByName[persisted.IssueType] = append(h.issuesByName[persisted.IssueType], issueID)
		activeTypes[persisted.ProtoIssueType] = struct{}{}
		activeCount++
	}
	for issueType := range activeTypes {
		h.setActiveGauge(issueType)
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
// repopulated by health checks on the next agent start. Only the persister
// calls it outside tests, so saves never overlap.
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
	resourceType, resourceID := h.ResourceIdentity(hostname)

	agentVersion := version.AgentVersion
	report := &healthplatform.HealthReport{
		EventType: "agent.health",
		EmittedAt: time.Now().UTC().Format(time.RFC3339),
		Service:   h.agentFlavor,
		Host: &healthplatform.HostInfo{
			Hostname:     hostname,
			AgentVersion: &agentVersion,
			ResourceType: resourceType,
			ResourceId:   resourceID,
		},
		Issues: issues,
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	return fb.AddFile("health-platform-issues.json", data)
}

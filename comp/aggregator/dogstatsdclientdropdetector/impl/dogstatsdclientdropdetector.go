// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package dogstatsdclientdropdetectorimpl implements the DogStatsD client drop detector.
package dogstatsdclientdropdetectorimpl

import (
	"context"
	"math"
	"strings"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"

	dogstatsdclientdropdetector "github.com/DataDog/datadog-agent/comp/aggregator/dogstatsdclientdropdetector/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	dogstatsdclientdrops "github.com/DataDog/datadog-agent/comp/healthplatform/issues/dogstatsdclientdrops"
	healthplatformstore "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	hostuuid "github.com/DataDog/datadog-agent/pkg/util/uuid"
)

const (
	enabledConfig                     = "dogstatsd_client_drop_detection.enabled"
	droppedRatioThresholdConfig       = "dogstatsd_client_drop_detection.dropped_ratio_threshold"
	unhealthyConfirmationWindowConfig = "dogstatsd_client_drop_detection.unhealthy_confirmation_window"
	recoveryConfirmationWindowConfig  = "dogstatsd_client_drop_detection.recovery_confirmation_window"
)

// Requires defines the dependencies for the DogStatsD client drop detector.
type Requires struct {
	Lifecycle      compdef.Lifecycle
	Config         config.Component
	Log            log.Component
	Hostname       hostnameinterface.Component
	HealthPlatform healthplatformstore.Component
}

// Provides defines the values provided by the DogStatsD client drop detector.
type Provides struct {
	Comp dogstatsdclientdropdetector.Component
}

type disabledComponent struct{}

func (*disabledComponent) ObserveClientBytes(string, string, dogstatsdclientdropdetector.ClientByteMetric, float64) {
}

func (*disabledComponent) CompleteFinalDogStatsDSerieFlush() {}

// clientByteStats holds client byte totals for one serializer-flush window.
type clientByteStats struct {
	sent          float64
	dropped       float64
	droppedQueue  float64
	droppedWriter float64
}

type clientState struct {
	library       dogstatsdclientdrops.ClientLibrary
	transport     dogstatsdclientdrops.ClientTransport
	stats         clientByteStats
	issueID       string
	issueActive   bool
	issueSeverity healthplatformpayload.IssueSeverity
	// issueNeedsRefresh marks restored active lifecycle state whose full issue
	// payload must be reported again after an Agent restart.
	issueNeedsRefresh bool
	// staleIssueIDs are active IDs for this client and transport that differ from the current host ID.
	staleIssueIDs []string
	// confirmationPending means unhealthy confirmation when inactive and recovery when active.
	confirmationPending bool
	pendingSince        time.Time
	// pendingStats accumulates the unhealthy windows used to construct a new issue.
	pendingStats clientByteStats
}

type component struct {
	clients        map[clientKey]*clientState
	logger         log.Component
	healthPlatform healthplatformstore.Component
	hostname       string
	hostUUID       string
	// startupReconciled is closed after persisted issue state has been reconciled.
	startupReconciled             chan struct{}
	droppedRatioThreshold         float64
	unhealthyConfirmationDuration time.Duration
	recoveryConfirmationDuration  time.Duration
	// now is replaceable so confirmation periods can be tested without sleeping.
	now func() time.Time
}

type clientKey struct {
	library   dogstatsdclientdrops.ClientLibrary
	transport dogstatsdclientdrops.ClientTransport
}

// NewComponent creates the DogStatsD client drop detector.
func NewComponent(req Requires) Provides {
	if !req.Config.GetBool(enabledConfig) {
		req.Lifecycle.Append(compdef.Hook{OnStart: func(context.Context) error {
			resolveActiveIssues(req.HealthPlatform)
			return nil
		}})
		return Provides{Comp: &disabledComponent{}}
	}

	detector := &component{
		clients:                       make(map[clientKey]*clientState),
		logger:                        req.Log,
		healthPlatform:                req.HealthPlatform,
		hostname:                      req.Hostname.GetSafe(context.Background()),
		hostUUID:                      hostuuid.GetUUID(),
		startupReconciled:             make(chan struct{}),
		droppedRatioThreshold:         req.Config.GetFloat64(droppedRatioThresholdConfig),
		unhealthyConfirmationDuration: req.Config.GetDuration(unhealthyConfirmationWindowConfig),
		recoveryConfirmationDuration:  req.Config.GetDuration(recoveryConfirmationWindowConfig),
		now:                           time.Now,
	}
	req.Lifecycle.Append(compdef.Hook{OnStart: func(context.Context) error {
		detector.reconcileIssueState()
		close(detector.startupReconciled)
		return nil
	}})
	return Provides{Comp: detector}
}

func resolveActiveIssues(healthPlatform healthplatformstore.Component) {
	for _, library := range dogstatsdclientdrops.ClientLibraries() {
		for _, issueID := range healthPlatform.GetActiveIssueIDsByIssueName(dogstatsdclientdrops.UDSIssueName(library)) {
			healthPlatform.ResolveIssue(issueID)
		}
	}
}

// ObserveClientBytes adds one validated UDS client byte total to the current window.
func (d *component) ObserveClientBytes(clientLibrary, clientTransport string, metric dogstatsdclientdropdetector.ClientByteMetric, bytes float64) {
	select {
	case <-d.startupReconciled:
	default:
		return
	}

	library := dogstatsdclientdrops.NormalizeClientLibrary(clientLibrary)
	transport := dogstatsdclientdrops.NormalizeClientTransport(clientTransport)
	if !dogstatsdclientdrops.IsSupportedClientLibrary(library) || !dogstatsdclientdrops.IsSupportedClientTransport(transport) {
		return
	}
	state := d.clientState(library, transport)
	switch metric {
	case dogstatsdclientdropdetector.ClientByteMetricSent:
		state.stats.sent += bytes
	case dogstatsdclientdropdetector.ClientByteMetricDropped:
		state.stats.dropped += bytes
	case dogstatsdclientdropdetector.ClientByteMetricDroppedQueue:
		state.stats.droppedQueue += bytes
	case dogstatsdclientdropdetector.ClientByteMetricDroppedWriter:
		state.stats.droppedWriter += bytes
	}
}

// CompleteFinalDogStatsDSerieFlush evaluates and resets the detector after all
// DogStatsD workers have contributed to the serializer-flush window.
func (d *component) CompleteFinalDogStatsDSerieFlush() {
	// Ignore flushes until persisted issue state has been reconciled during startup.
	select {
	case <-d.startupReconciled:
	default:
		return
	}

	for _, state := range d.clients {
		d.completeWindow(state)
	}
}

func (d *component) clientState(clientLibrary dogstatsdclientdrops.ClientLibrary, clientTransport dogstatsdclientdrops.ClientTransport) *clientState {
	key := clientKey{library: clientLibrary, transport: clientTransport}
	state, found := d.clients[key]
	if found {
		return state
	}
	state = &clientState{
		library:   clientLibrary,
		transport: clientTransport,
		issueID:   dogstatsdclientdrops.UDSIssueIDForHost(clientLibrary, clientTransport, d.hostUUID, d.hostname),
	}
	d.clients[key] = state
	return state
}

func (d *component) takeWindow(state *clientState) clientByteStats {
	stats := state.stats
	state.stats = clientByteStats{}
	return stats
}

func (d *component) completeWindow(state *clientState) {
	stats := d.takeWindow(state)
	// Drop-reason breakdowns alone cannot establish the sent/drop ratio.
	if stats.sent == 0 && stats.dropped == 0 {
		return
	}
	_, violated := droppedRatio(stats, d.droppedRatioThreshold)
	if violated {
		d.handleUnhealthyWindow(state, stats)
		return
	}
	d.handleHealthyWindow(state)
}

func (d *component) handleUnhealthyWindow(state *clientState, stats clientByteStats) {
	if state.issueActive {
		ratio, _ := droppedRatio(stats, d.droppedRatioThreshold)
		if state.issueNeedsRefresh || dogstatsdclientdrops.SeverityForDroppedRatio(ratio) != state.issueSeverity {
			d.reportIssue(state, stats, ratio)
		}
		d.resetPendingTransition(state)
		return
	}

	now := d.now()
	if !state.confirmationPending {
		state.confirmationPending = true
		state.pendingSince = now
		state.pendingStats = stats
		return
	}

	state.pendingStats.add(stats)
	if now.Sub(state.pendingSince) < d.unhealthyConfirmationDuration {
		return
	}

	ratio, _ := droppedRatio(state.pendingStats, d.droppedRatioThreshold)
	d.reportIssue(state, state.pendingStats, ratio)
	if state.issueActive {
		d.resetPendingTransition(state)
	}
}

func (d *component) handleHealthyWindow(state *clientState) {
	if !state.issueActive {
		d.resetPendingTransition(state)
		return
	}

	now := d.now()
	if !state.confirmationPending {
		state.confirmationPending = true
		state.pendingSince = now
		state.pendingStats = clientByteStats{}
		return
	}

	if now.Sub(state.pendingSince) < d.recoveryConfirmationDuration {
		return
	}

	d.resolveIssue(state)
	d.resetPendingTransition(state)
}

func (d *component) resetPendingTransition(state *clientState) {
	state.confirmationPending = false
	state.pendingSince = time.Time{}
	state.pendingStats = clientByteStats{}
}

func (s *clientByteStats) add(other clientByteStats) {
	s.sent += other.sent
	s.dropped += other.dropped
	s.droppedQueue += other.droppedQueue
	s.droppedWriter += other.droppedWriter
}

func (s clientByteStats) dropReasonBreakdown() (float64, bool) {
	classified := s.droppedQueue + s.droppedWriter
	unclassified := math.Max(s.dropped-classified, 0)
	tolerance := math.Max(s.dropped*1e-9, 1e-9)
	return unclassified, math.Abs(classified-s.dropped) <= tolerance
}

func (d *component) reconcileIssueState() {
	for _, library := range dogstatsdclientdrops.ClientLibraries() {
		activeIDs := d.healthPlatform.GetActiveIssueIDsByIssueName(dogstatsdclientdrops.UDSIssueName(library))
		for _, transport := range dogstatsdclientdrops.ClientTransports() {
			prefix := dogstatsdclientdrops.UDSIssueIDPrefix(library, transport) + ":"
			state := d.clientState(library, transport)
			for _, activeID := range activeIDs {
				if !strings.HasPrefix(activeID, prefix) {
					continue
				}
				state.issueActive = true
				state.issueNeedsRefresh = true
				if activeID != state.issueID {
					state.staleIssueIDs = append(state.staleIssueIDs, activeID)
				}
			}
			if state.issueActive && d.reportRestoredIssue(state) {
				d.resolveStaleIssues(state)
			}
		}
	}
}

func (d *component) reportRestoredIssue(state *clientState) bool {
	issue, err := dogstatsdclientdrops.BuildRestoredUDSIssue(state.library, state.transport, d.hostname)
	if issue == nil {
		d.logger.Warnf("failed to build restored DogStatsD client payload drop health issue: %v", err)
		return false
	}
	if err != nil {
		d.logger.Warnf("reporting restored DogStatsD client payload drop health issue without additional diagnostic details: %v", err)
	}
	issue.Id = state.issueID
	if err := d.healthPlatform.ReportIssue(issue); err != nil {
		d.logger.Warnf("failed to restore DogStatsD client payload drop health issue after restart: %v", err)
		return false
	}
	state.issueSeverity = issue.Severity
	return true
}

func (d *component) reportIssue(state *clientState, stats clientByteStats, ratio float64) {
	unclassified, breakdownComplete := stats.dropReasonBreakdown()
	issue, err := dogstatsdclientdrops.BuildUDSIssue(dogstatsdclientdrops.UDSDetectionContext{
		ClientLibrary:               state.library,
		ClientTransport:             state.transport,
		AgentHostname:               d.hostname,
		DroppedRatio:                ratio,
		Threshold:                   d.droppedRatioThreshold,
		BytesSent:                   stats.sent,
		BytesDropped:                stats.dropped,
		BytesDroppedQueue:           stats.droppedQueue,
		BytesDroppedWriter:          stats.droppedWriter,
		BytesDroppedUnclassified:    unclassified,
		DropReasonBreakdownComplete: breakdownComplete,
	})
	if issue == nil {
		d.logger.Warnf("failed to build DogStatsD client payload drop health issue: %v", err)
		return
	}
	if err != nil {
		d.logger.Warnf("reporting DogStatsD client payload drop health issue without additional diagnostic details: %v", err)
	}
	issue.Id = state.issueID
	if err := d.healthPlatform.ReportIssue(issue); err != nil {
		d.logger.Warnf("failed to report DogStatsD client payload drop health issue: %v", err)
		return
	}
	state.issueActive = true
	state.issueSeverity = issue.Severity
	state.issueNeedsRefresh = false
	d.resolveStaleIssues(state)
}

func (d *component) resolveIssue(state *clientState) {
	d.healthPlatform.ResolveIssue(state.issueID)
	d.resolveStaleIssues(state)
	state.issueActive = false
	state.issueNeedsRefresh = false
}

func (d *component) resolveStaleIssues(state *clientState) {
	for _, issueID := range state.staleIssueIDs {
		d.healthPlatform.ResolveIssue(issueID)
	}
	state.staleIssueIDs = nil
}

func droppedRatio(stats clientByteStats, threshold float64) (float64, bool) {
	total := stats.dropped + stats.sent
	if total == 0 {
		return 0, false
	}
	ratio := stats.dropped / total
	return ratio, ratio > threshold
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package dogstatsdclientdropdetectorimpl implements the DogStatsD client drop detector.
package dogstatsdclientdropdetectorimpl

import (
	"context"
	"math"
	"time"

	dogstatsdclientdropdetector "github.com/DataDog/datadog-agent/comp/aggregator/dogstatsdclientdropdetector/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	dogstatsdclientdrops "github.com/DataDog/datadog-agent/comp/healthplatform/issues/dogstatsdclientdrops"
	healthplatformstore "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

const (
	droppedBytesIssueThreshold        = 0.01
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

// clientByteStats holds client byte totals for one serializer-flush window.
type clientByteStats struct {
	sent          float64
	dropped       float64
	droppedQueue  float64
	droppedWriter float64
}

// pendingTransition identifies a state change awaiting continuous confirmation.
type pendingTransition uint8

const (
	noPendingTransition pendingTransition = iota
	pendingUnhealthy
	pendingRecovery
)

type component struct {
	stats          clientByteStats
	logger         log.Component
	healthPlatform healthplatformstore.Component
	hostname       string
	issueID        string
	issueActive    bool
	// issueNeedsRefresh marks restored active lifecycle state whose full issue
	// payload must be reported again after an Agent restart.
	issueNeedsRefresh bool
	pending           pendingTransition
	pendingSince      time.Time
	// pendingStats accumulates the unhealthy windows used to construct a new issue.
	pendingStats                  clientByteStats
	unhealthyConfirmationDuration time.Duration
	recoveryConfirmationDuration  time.Duration
	// now is replaceable so confirmation periods can be tested without sleeping.
	now func() time.Time
}

// NewComponent creates the DogStatsD client drop detector.
func NewComponent(req Requires) Provides {
	hostname := req.Hostname.GetSafe(context.Background())
	detector := &component{
		logger:                        req.Log,
		healthPlatform:                req.HealthPlatform,
		hostname:                      hostname,
		issueID:                       dogstatsdclientdrops.UDSIssueIDForHostname(hostname),
		unhealthyConfirmationDuration: req.Config.GetDuration(unhealthyConfirmationWindowConfig),
		recoveryConfirmationDuration:  req.Config.GetDuration(recoveryConfirmationWindowConfig),
		now:                           time.Now,
	}
	req.Lifecycle.Append(compdef.Hook{OnStart: func(context.Context) error {
		detector.reconcileIssueState()
		return nil
	}})
	return Provides{Comp: detector}
}

// ObserveClientBytes adds one validated UDS client byte total to the current window.
func (d *component) ObserveClientBytes(metric dogstatsdclientdropdetector.ClientByteMetric, bytes float64) {
	switch metric {
	case dogstatsdclientdropdetector.ClientByteMetricSent:
		d.stats.sent += bytes
	case dogstatsdclientdropdetector.ClientByteMetricDropped:
		d.stats.dropped += bytes
	case dogstatsdclientdropdetector.ClientByteMetricDroppedQueue:
		d.stats.droppedQueue += bytes
	case dogstatsdclientdropdetector.ClientByteMetricDroppedWriter:
		d.stats.droppedWriter += bytes
	}
}

// CompleteFinalDogStatsDSerieFlush evaluates and resets the detector after all
// DogStatsD workers have contributed to the serializer-flush window.
func (d *component) CompleteFinalDogStatsDSerieFlush() {
	d.completeWindow()
}

func (d *component) takeWindow() clientByteStats {
	stats := d.stats
	d.stats = clientByteStats{}
	return stats
}

func (d *component) completeWindow() {
	stats := d.takeWindow()
	// Drop-reason breakdowns alone cannot establish the sent/drop ratio.
	if stats.sent == 0 && stats.dropped == 0 {
		d.resetPendingTransition()
		return
	}
	_, violated := droppedRatio(stats)
	if violated {
		d.handleUnhealthyWindow(stats)
		return
	}
	d.handleHealthyWindow()
}

func (d *component) handleUnhealthyWindow(stats clientByteStats) {
	if d.issueActive {
		if d.issueNeedsRefresh {
			ratio, _ := droppedRatio(stats)
			d.reportIssue(stats, ratio)
		}
		d.resetPendingTransition()
		return
	}

	now := d.now()
	if d.pending != pendingUnhealthy {
		d.pending = pendingUnhealthy
		d.pendingSince = now
		d.pendingStats = stats
		return
	}

	d.pendingStats.add(stats)
	if now.Sub(d.pendingSince) < d.unhealthyConfirmationDuration {
		return
	}

	ratio, _ := droppedRatio(d.pendingStats)
	d.reportIssue(d.pendingStats, ratio)
	if d.issueActive {
		d.resetPendingTransition()
	}
}

func (d *component) handleHealthyWindow() {
	if !d.issueActive {
		d.resetPendingTransition()
		return
	}

	now := d.now()
	if d.pending != pendingRecovery {
		d.pending = pendingRecovery
		d.pendingSince = now
		d.pendingStats = clientByteStats{}
		return
	}

	if now.Sub(d.pendingSince) < d.recoveryConfirmationDuration {
		return
	}

	d.resolveIssue()
	d.resetPendingTransition()
}

func (d *component) resetPendingTransition() {
	d.pending = noPendingTransition
	d.pendingSince = time.Time{}
	d.pendingStats = clientByteStats{}
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
	for _, activeID := range d.healthPlatform.GetActiveIssueIDsByIssueName(dogstatsdclientdrops.UDSIssueName) {
		if activeID == d.issueID {
			d.issueActive = true
			d.issueNeedsRefresh = true
			continue
		}
		// The issue is scoped to this Agent's current hostname. Resolve lifecycle
		// state left under an earlier hostname instead of leaving it active forever.
		d.healthPlatform.ResolveIssue(activeID)
	}
	if d.issueActive {
		d.reportRestoredIssue()
	}
}

func (d *component) reportRestoredIssue() {
	issue, err := dogstatsdclientdrops.BuildRestoredUDSIssue(d.hostname)
	if err != nil {
		d.logger.Warnf("failed to rebuild DogStatsD client payload drop health issue after restart: %v", err)
		return
	}
	issue.Id = d.issueID
	if err := d.healthPlatform.ReportIssue(issue); err != nil {
		d.logger.Warnf("failed to restore DogStatsD client payload drop health issue after restart: %v", err)
	}
}

func (d *component) reportIssue(stats clientByteStats, ratio float64) {
	if d.issueActive && !d.issueNeedsRefresh {
		return
	}
	unclassified, breakdownComplete := stats.dropReasonBreakdown()
	issue, err := dogstatsdclientdrops.BuildUDSIssue(dogstatsdclientdrops.UDSDetectionContext{
		Hostname:                    d.hostname,
		DroppedRatio:                ratio,
		Threshold:                   droppedBytesIssueThreshold,
		BytesSent:                   stats.sent,
		BytesDropped:                stats.dropped,
		BytesDroppedQueue:           stats.droppedQueue,
		BytesDroppedWriter:          stats.droppedWriter,
		BytesDroppedUnclassified:    unclassified,
		DropReasonBreakdownComplete: breakdownComplete,
	})
	if err != nil {
		d.logger.Warnf("failed to build DogStatsD client payload drop health issue: %v", err)
		return
	}
	issue.Id = d.issueID
	if err := d.healthPlatform.ReportIssue(issue); err != nil {
		d.logger.Warnf("failed to report DogStatsD client payload drop health issue: %v", err)
		return
	}
	d.issueActive = true
	d.issueNeedsRefresh = false
}

func (d *component) resolveIssue() {
	d.healthPlatform.ResolveIssue(d.issueID)
	d.issueActive = false
	d.issueNeedsRefresh = false
}

func droppedRatio(stats clientByteStats) (float64, bool) {
	total := stats.dropped + stats.sent
	if total == 0 {
		return 0, false
	}
	ratio := stats.dropped / total
	return ratio, ratio > droppedBytesIssueThreshold
}

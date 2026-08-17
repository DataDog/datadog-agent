// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package dogstatsdclienttelemetryimpl implements DogStatsD client telemetry.
package dogstatsdclienttelemetryimpl

import (
	"context"
	"math"
	"strings"
	"time"

	dogstatsdclienttelemetry "github.com/DataDog/datadog-agent/comp/aggregator/dogstatsdclienttelemetry/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	dogstatsdclientdrops "github.com/DataDog/datadog-agent/comp/healthplatform/issues/dogstatsdclientdrops"
	healthplatformstore "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const (
	dogStatsDClientBytesSentMetric          = "datadog.dogstatsd.client.bytes_sent"
	dogStatsDClientBytesDroppedMetric       = "datadog.dogstatsd.client.bytes_dropped"
	dogStatsDClientBytesDroppedQueueMetric  = "datadog.dogstatsd.client.bytes_dropped_queue"
	dogStatsDClientBytesDroppedWriterMetric = "datadog.dogstatsd.client.bytes_dropped_writer"
	dogStatsDClientLibraryTagPrefix         = "client:"
	dogStatsDClientTransportTagPrefix       = "client_transport:"
	unknownTagValue                         = "unknown"
	dogStatsDClientUDSTransportTag          = "client_transport:uds"
	dogStatsDClientUDSStreamTransportTag    = "client_transport:uds-stream"
	droppedBytesIssueThreshold              = 0.01
	unhealthyConfirmationWindowConfig       = "dogstatsd_client_drop_detection.unhealthy_confirmation_window"
	recoveryConfirmationWindowConfig        = "dogstatsd_client_drop_detection.recovery_confirmation_window"
)

type clientByteMetric uint8

const (
	clientByteMetricSent clientByteMetric = iota
	clientByteMetricDropped
	clientByteMetricDroppedQueue
	clientByteMetricDroppedWriter
)

// Requires defines the dependencies for the DogStatsD client telemetry component.
type Requires struct {
	Lifecycle      compdef.Lifecycle
	Telemetry      telemetry.Component
	Config         config.Component
	Log            log.Component
	Hostname       hostnameinterface.Component
	HealthPlatform healthplatformstore.Component
}

// Provides defines the values provided by the DogStatsD client telemetry component.
type Provides struct {
	Comp     dogstatsdclienttelemetry.Component
	Observer aggregator.FinalDogStatsDSerieObserver `group:"dogstatsd_final_serie_observers"`
}

type component struct {
	bytesSent          telemetry.Counter
	bytesDropped       telemetry.Counter
	bytesDroppedQueue  telemetry.Counter
	bytesDroppedWriter telemetry.Counter
	detector           droppedMetricsDetector
}

// clientByteStats holds client byte totals reconstructed from final rate series.
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

// droppedMetricsDetector evaluates UDS client telemetry observed by one Agent,
// confirms state transitions across serializer-flush windows, and synchronizes
// the resulting issue with Agent Health.
type droppedMetricsDetector struct {
	// The demultiplexer flushes DogStatsD workers sequentially before completing
	// the window, so detector state is confined to that serialized flush path.
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

// NewComponent creates the DogStatsD client telemetry component.
func NewComponent(req Requires) Provides {
	hostname := req.Hostname.GetSafe(context.Background())
	tags := []string{"client", "client_transport"}
	component := &component{
		bytesSent:          req.Telemetry.NewCounter("dogstatsd_client", "bytes_sent", tags, "Total bytes sent by DogStatsD clients"),
		bytesDropped:       req.Telemetry.NewCounter("dogstatsd_client", "bytes_dropped", tags, "Total bytes dropped by DogStatsD clients"),
		bytesDroppedQueue:  req.Telemetry.NewCounter("dogstatsd_client", "bytes_dropped_queue", tags, "Total bytes dropped because the DogStatsD client sender queue is full"),
		bytesDroppedWriter: req.Telemetry.NewCounter("dogstatsd_client", "bytes_dropped_writer", tags, "Total bytes dropped because the DogStatsD client writer cannot send"),
		detector: droppedMetricsDetector{
			logger:                        req.Log,
			healthPlatform:                req.HealthPlatform,
			hostname:                      hostname,
			issueID:                       dogstatsdclientdrops.UDSIssueIDForHostname(hostname),
			unhealthyConfirmationDuration: req.Config.GetDuration(unhealthyConfirmationWindowConfig),
			recoveryConfirmationDuration:  req.Config.GetDuration(recoveryConfirmationWindowConfig),
			now:                           time.Now,
		},
	}
	req.Lifecycle.Append(compdef.Hook{OnStart: func(context.Context) error {
		component.detector.reconcileIssueState()
		return nil
	}})
	return Provides{Comp: component, Observer: component}
}

// ObserveFinalDogStatsDSerie mirrors valid client-byte rate buckets into the
// corresponding internal counter and records UDS totals for drop detection.
func (c *component) ObserveFinalDogStatsDSerie(serie *metrics.Serie) {
	if serie.MType != metrics.APIRateType || serie.Interval <= 0 {
		return
	}
	var counter telemetry.Counter
	var metric clientByteMetric
	switch serie.Name {
	case dogStatsDClientBytesSentMetric:
		counter = c.bytesSent
		metric = clientByteMetricSent
	case dogStatsDClientBytesDroppedMetric:
		counter = c.bytesDropped
		metric = clientByteMetricDropped
	case dogStatsDClientBytesDroppedQueueMetric:
		counter = c.bytesDroppedQueue
		metric = clientByteMetricDroppedQueue
	case dogStatsDClientBytesDroppedWriterMetric:
		counter = c.bytesDroppedWriter
		metric = clientByteMetricDroppedWriter
	default:
		return
	}
	client, transport := clientTelemetryTags(serie)
	var totalBytes float64
	for _, point := range serie.Points {
		bytes := point.Value * float64(serie.Interval)
		if !(bytes >= 0 && bytes < math.MaxUint64) {
			continue
		}
		counter.Add(bytes, client, transport)
		totalBytes += bytes
	}

	if totalBytes > 0 && (transport == "uds" || transport == "uds-stream") {
		c.detector.observe(metric, totalBytes)
	}
}

func clientTelemetryTags(serie *metrics.Serie) (string, string) {
	client := unknownTagValue
	transport := unknownTagValue
	serie.Tags.Find(func(tag string) bool {
		if strings.HasPrefix(tag, dogStatsDClientLibraryTagPrefix) {
			client = normalizeClientLibrary(strings.TrimPrefix(tag, dogStatsDClientLibraryTagPrefix))
		} else if strings.HasPrefix(tag, dogStatsDClientTransportTagPrefix) {
			transport = normalizeClientTransport(strings.TrimPrefix(tag, dogStatsDClientTransportTagPrefix))
		}
		return client != unknownTagValue && transport != unknownTagValue
	})
	return client, transport
}

func normalizeClientLibrary(client string) string {
	switch client {
	case "go", "py", "java", "ruby", "csharp", "php", "rust":
		return client
	default:
		return unknownTagValue
	}
}

func normalizeClientTransport(transport string) string {
	switch transport {
	case "udp", "uds", "uds-stream", "uds-datagram", "pipe", "namedpipe", "named_pipe", "custom", "http":
		return transport
	default:
		return unknownTagValue
	}
}

func isUDSTransportTag(tag string) bool {
	return tag == dogStatsDClientUDSTransportTag || tag == dogStatsDClientUDSStreamTransportTag
}

// CompleteFinalDogStatsDSerieFlush evaluates and resets the detector after all
// DogStatsD workers have contributed to the serializer-flush window.
func (c *component) CompleteFinalDogStatsDSerieFlush() {
	c.detector.completeWindow()
}

func (d *droppedMetricsDetector) observe(metric clientByteMetric, bytes float64) {
	switch metric {
	case clientByteMetricSent:
		d.stats.sent += bytes
	case clientByteMetricDropped:
		d.stats.dropped += bytes
	case clientByteMetricDroppedQueue:
		d.stats.droppedQueue += bytes
	case clientByteMetricDroppedWriter:
		d.stats.droppedWriter += bytes
	}
}

func (d *droppedMetricsDetector) takeWindow() clientByteStats {
	stats := d.stats
	d.stats = clientByteStats{}
	return stats
}

func (d *droppedMetricsDetector) completeWindow() {
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

func (d *droppedMetricsDetector) handleUnhealthyWindow(stats clientByteStats) {
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

func (d *droppedMetricsDetector) handleHealthyWindow() {
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

func (d *droppedMetricsDetector) resetPendingTransition() {
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

func (d *droppedMetricsDetector) reconcileIssueState() {
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

func (d *droppedMetricsDetector) reportRestoredIssue() {
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

func (d *droppedMetricsDetector) reportIssue(stats clientByteStats, ratio float64) {
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

func (d *droppedMetricsDetector) resolveIssue() {
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

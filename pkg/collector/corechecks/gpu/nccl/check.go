// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package nccl contains the NCCL collective communication check implementation.
package nccl

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// collKey is used to group collective events across ranks for divergence detection.
type collKey struct {
	commHash   string
	collSN     int64
	collective string
}

// Check represents the NCCL check that collects metrics from NCCL Inspector output
type Check struct {
	core.CheckBase
	config            *checkConfig
	tagger            tagger.Component
	telemetry         telemetry.Component
	wmeta             workloadmeta.Component
	parser            *Parser
	socketListener    *SocketListener
	processTagger     *ProcessTagger
	containerProvider proccontainers.ContainerProvider
	checkTelemetry    *ncclCheckTelemetry
	lastSeenRank      map[string]rankStalenessEntry // "rank:N" → last event + timestamp for hang detection
}

// rankStalenessEntry tracks the last event seen for a rank, used for hang detection.
// Storing the ParsedEvent allows emitStalenessMetrics to emit the same pod/container
// tags as other NCCL metrics.
type rankStalenessEntry struct {
	lastSeen time.Time
	parsed   ParsedEvent
}

// checkConfig holds the configuration for the NCCL check
type checkConfig struct {
	JSONDir       string `yaml:"json_dir"`
	FileRetention string `yaml:"file_retention"`
	SocketPath    string `yaml:"socket_path"`
}

type ncclCheckTelemetry struct {
	eventsProcessed telemetry.Counter
	parseErrors     telemetry.Counter
	filesProcessed  telemetry.Counter
	metricsSent     telemetry.Counter
}

func newCheckTelemetry(tm telemetry.Component) *ncclCheckTelemetry {
	return &ncclCheckTelemetry{
		eventsProcessed: tm.NewCounter(CheckName, "events_processed", nil, "Number of NCCL events processed"),
		parseErrors:     tm.NewCounter(CheckName, "parse_errors", nil, "Number of JSON parse errors"),
		filesProcessed:  tm.NewCounter(CheckName, "files_processed", nil, "Number of JSON files processed"),
		metricsSent:     tm.NewCounter(CheckName, "metrics_sent", []string{"metric_name"}, "Number of metrics sent"),
	}
}

// Factory creates a new check factory
func Factory(tagger tagger.Component, telemetry telemetry.Component, wmeta workloadmeta.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(tagger, telemetry, wmeta)
	})
}

func newCheck(tagger tagger.Component, telemetry telemetry.Component, wmeta workloadmeta.Component) check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(CheckName),
		tagger:    tagger,
		telemetry: telemetry,
		wmeta:     wmeta,
	}
}

// Configure parses the check configuration and initializes the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	// Check if NCCL check is enabled
	if !pkgconfigsetup.Datadog().GetBool("nccl.enabled") {
		return errors.New("NCCL check is disabled")
	}

	if err := c.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}

	// Parse instance config
	c.config = &checkConfig{
		JSONDir:       defaultJSONDir,
		FileRetention: defaultFileRetention,
		SocketPath:    defaultSocketPath,
	}

	if err := yaml.Unmarshal(config, c.config); err != nil {
		return fmt.Errorf("failed to parse check config: %w", err)
	}

	// Override with global config if set
	if globalDir := pkgconfigsetup.Datadog().GetString("nccl.json_dir"); globalDir != "" {
		c.config.JSONDir = globalDir
	}

	// Initialize parser (file-based fallback)
	c.parser = NewParser(c.config.JSONDir)

	// Start socket listener — preferred path, falls back to file parser if unavailable
	if sl, err := newSocketListener(c.config.SocketPath); err != nil {
		log.Infof("NCCL socket listener unavailable (%v), using file-based collection", err)
	} else {
		c.socketListener = sl
	}

	// Initialize container provider for PID -> container mapping
	if c.containerProvider == nil {
		containerProvider, err := proccontainers.GetSharedContainerProvider()
		if err != nil {
			log.Warnf("failed to get shared container provider: %v", err)
		}
		c.containerProvider = containerProvider
	}

	// Initialize process tagger for PID -> container -> pod correlation
	c.processTagger = NewProcessTagger(c.tagger, c.wmeta, c.containerProvider)

	// Initialize hang detection state
	c.lastSeenRank = make(map[string]rankStalenessEntry)

	// Initialize telemetry
	c.checkTelemetry = newCheckTelemetry(c.telemetry)

	return nil
}

// Run executes the check
func (c *Check) Run() error {
	snd, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %w", err)
	}
	defer snd.Commit()

	// Lazy-init containerProvider: GetSharedContainerProvider fails at Configure time
	// (startup ordering) but is ready by the first Run call. Re-initialize processTagger
	// so it has a valid provider reference.
	if c.containerProvider == nil {
		if p, err := proccontainers.GetSharedContainerProvider(); err == nil {
			c.containerProvider = p
			c.processTagger = NewProcessTagger(c.tagger, c.wmeta, c.containerProvider)
		}
	}

	// Refresh process tagger to get fresh PID -> container mappings
	if c.processTagger != nil {
		c.processTagger.Refresh()
	}

	// Collect events: socket (preferred) + file fallback
	var events []ParsedEvent
	if c.socketListener != nil {
		// Socket path: drain events buffered since last Run()
		events = c.socketListener.Drain()
	} else {
		// File path: read new lines appended since last check
		var err error
		events, err = c.parser.ParseNewEvents()
		if err != nil {
			log.Warnf("error parsing NCCL Inspector JSON: %v", err)
			c.checkTelemetry.parseErrors.Inc()
		}
	}

	// Process events and emit per-rank metrics
	for _, parsed := range events {
		if err := c.processEvent(snd, parsed); err != nil {
			log.Debugf("error processing NCCL event: %v", err)
		}
		c.checkTelemetry.eventsProcessed.Inc()
	}

	// Hang detection: update last-seen timestamps and emit staleness metrics.
	// Key by rank only (not rank+PID) so that training restarts reset staleness
	// for the rank rather than accumulating a new stale entry per PID.
	now := time.Now()
	for _, parsed := range events {
		rankKey := fmt.Sprintf("rank:%d", parsed.Event.Rank)
		c.lastSeenRank[rankKey] = rankStalenessEntry{lastSeen: now, parsed: parsed}
	}
	c.emitStalenessMetrics(snd, now)

	// Intra-node straggler detection: emit divergence across ranks seen this run
	emitRankDivergence(snd, events)

	// Network transfer time: aggregate ProxyOp events per (rank, direction)
	emitNetworkTransferMetrics(snd, events)

	// Cleanup old files if retention is configured
	if c.config.FileRetention != "" {
		retention, err := time.ParseDuration(c.config.FileRetention)
		if err == nil && retention > 0 {
			if err := c.parser.CleanupOldFiles(retention); err != nil {
				log.Debugf("error cleaning up old NCCL files: %v", err)
			}
		}
	}

	return nil
}

// processEvent emits metrics for a single NCCL collective event.
// ProxyOp events are not processed here — they are aggregated by emitNetworkTransferMetrics.
func (c *Check) processEvent(snd sender.Sender, parsed ParsedEvent) error {
	event := parsed.Event
	perf := event.CollPerf
	if perf == nil {
		return nil
	}

	// Build tags from PID -> container -> pod correlation
	tags := c.buildTags(parsed)

	// Add NCCL-specific tags.
	tags = append(tags,
		fmt.Sprintf("rank:%d", event.Rank),
		"collective:"+perf.Collective,
	)
	if event.NRanks > 0 {
		tags = append(tags, fmt.Sprintf("n_ranks:%d", event.NRanks))
	}
	if event.GlobalRank >= 0 {
		tags = append(tags, fmt.Sprintf("global_rank:%d", event.GlobalRank))
	}

	if event.Hostname != "" {
		tags = append(tags, "nccl_hostname:"+event.Hostname)
	}

	if event.GPUUUID != "" {
		tags = append(tags, "gpu_uuid:"+event.GPUUUID)
	}

	if perf.TimingSource != "" {
		tags = append(tags, "timing_source:"+perf.TimingSource)
	}

	// Emit metrics
	// Execution time (the key metric for straggler detection)
	snd.Gauge(ncclMetricsNs+"collective.exec_time_us", perf.ExecTimeUS, "", tags)
	c.checkTelemetry.metricsSent.Inc("exec_time_us")

	// Bandwidth metrics
	if perf.AlgoBandwidthGB > 0 {
		snd.Gauge(ncclMetricsNs+"collective.algo_bandwidth_gbps", perf.AlgoBandwidthGB, "", tags)
		c.checkTelemetry.metricsSent.Inc("algo_bandwidth_gbps")
	}

	if perf.BusBandwidthGB > 0 {
		snd.Gauge(ncclMetricsNs+"collective.bus_bandwidth_gbps", perf.BusBandwidthGB, "", tags)
		c.checkTelemetry.metricsSent.Inc("bus_bandwidth_gbps")
	}

	// Message size
	if perf.MsgSizeBytes > 0 {
		snd.Gauge(ncclMetricsNs+"collective.msg_size_bytes", float64(perf.MsgSizeBytes), "", tags)
		c.checkTelemetry.metricsSent.Inc("msg_size_bytes")
	}

	return nil
}

// networkTransferKey groups proxy operations by rank and direction for aggregation.
type networkTransferKey struct {
	rank      int
	direction string // "send" or "recv"
}

// emitNetworkTransferMetrics aggregates ProxyOp events from this check interval
// and emits one nccl.network.max_transfer_time_us per (rank, direction).
// Cardinality: 2N (one send + one recv per rank observed on this node).
func emitNetworkTransferMetrics(snd sender.Sender, events []ParsedEvent) {
	maxTimes := make(map[networkTransferKey]int64)
	for _, parsed := range events {
		proxyOp := parsed.Event.ProxyOp
		if proxyOp == nil {
			continue
		}
		dir := "recv"
		if proxyOp.IsSend != 0 {
			dir = "send"
		}
		key := networkTransferKey{rank: parsed.Event.Rank, direction: dir}
		if proxyOp.NetTimeUS > maxTimes[key] {
			maxTimes[key] = proxyOp.NetTimeUS
		}
	}
	for key, maxTime := range maxTimes {
		tags := []string{
			fmt.Sprintf("rank:%d", key.rank),
			"direction:" + key.direction,
		}
		snd.Gauge(ncclMetricsNs+networkMaxTransferTimeMetric, float64(maxTime), "", tags)
	}
}

// buildTags correlates PID to container/pod and builds tags.
// When the event arrived via socket, parsed.HostPID is set to the kernel-provided
// host-namespace PID (via SO_PEERCRED), which works correctly with workloadmeta.
// For file-based events, parsed.HostPID is 0 and we fall back to event.PID
// (which may be a container-namespace PID and fail to resolve).
func (c *Check) buildTags(parsed ParsedEvent) []string {
	event := parsed.Event
	var tags []string

	// Prefer HostPID (from SO_PEERCRED, always a host-namespace PID).
	// Fall back to event.PID for file-based collection.
	lookupPID := parsed.HostPID
	if lookupPID == 0 {
		lookupPID = event.PID
	}

	if c.processTagger != nil && lookupPID > 0 {
		workloadTags, err := c.processTagger.GetTagsForPID(lookupPID)
		if err != nil {
			log.Debugf("failed to get workload tags for PID %d: %v", lookupPID, err)
		}
		tags = append(tags, workloadTags...)
	} else {
		tags = append(tags, fmt.Sprintf("pid:%d", event.PID))
	}

	return tags
}

// Cancel stops the check
func (c *Check) Cancel() {
	if c.socketListener != nil {
		c.socketListener.Stop()
	}
	c.CheckBase.Cancel()
}

// emitStalenessMetrics emits nccl.rank.seconds_since_last_event for every rank
// that has ever produced events.  Callers pass the current time so tests can inject
// a fixed instant without real sleeps.
// lastSeenTime is keyed by "rank:N" so training restarts reset the staleness for a
// rank rather than accumulating a stale entry per PID.
// Entries older than rankStalenessMaxAge are evicted: once staleness exceeds ~5
// minutes the job is either finished or has been alarmed on, and keeping the entry
// would cause false-positive hang signals if a new job runs on a different node.
func (c *Check) emitStalenessMetrics(snd sender.Sender, now time.Time) {
	for rankKey, entry := range c.lastSeenRank {
		staleness := now.Sub(entry.lastSeen)
		if staleness > rankStalenessMaxAge {
			delete(c.lastSeenRank, rankKey)
			continue
		}
		tags := c.buildTags(entry.parsed)
		tags = append(tags, rankKey) // "rank:N"
		if entry.parsed.Event.Hostname != "" {
			tags = append(tags, "nccl_hostname:"+entry.parsed.Event.Hostname)
		}
		snd.Gauge(ncclMetricsNs+hangDetectionMetric, staleness.Seconds(), "", tags)
	}
}

// emitRankDivergence groups the supplied events by (commHash, collSN, collective) and,
// whenever 2+ ranks are present, emits nccl.intra_node_rank_divergence_us with the
// max−min exec_time_us across those ranks.  This only fires when multiple ranks write
// to the same node (intra-node divergence detection).
func emitRankDivergence(snd sender.Sender, events []ParsedEvent) {
	type rankTiming struct {
		rank     int
		execTime float64
	}
	collTimings := make(map[collKey][]rankTiming)
	for _, parsed := range events {
		perf := parsed.Event.CollPerf
		if perf == nil {
			continue
		}
		key := collKey{parsed.Event.ID, perf.CollSN, perf.Collective}
		collTimings[key] = append(collTimings[key], rankTiming{parsed.Event.Rank, perf.ExecTimeUS})
	}
	for key, timings := range collTimings {
		if len(timings) < 2 {
			continue
		}
		minT, maxT := timings[0].execTime, timings[0].execTime
		for _, t := range timings[1:] {
			if t.execTime < minT {
				minT = t.execTime
			}
			if t.execTime > maxT {
				maxT = t.execTime
			}
		}
		divTags := []string{
			"collective:" + key.collective,
			fmt.Sprintf("n_ranks_observed:%d", len(timings)),
		}
		snd.Gauge(ncclMetricsNs+intraNodeDivergenceMetric, maxT-minT, "", divTags)
	}
}

// extractRankFromFilename parses the rank number from a filename.
// Handles both file format ("nccl-rank<N>-pid<P>.jsonl") and socket format ("socket:rank<N>-pid<P>").
// Returns 0 if the rank cannot be determined.
func extractRankFromFilename(filename string) int {
	base := filepath.Base(filename)
	var rank int
	// File format: nccl-rank0-pid123.jsonl
	if n, _ := fmt.Sscanf(base, "nccl-rank%d-", &rank); n == 1 {
		return rank
	}
	// Socket format: socket:rank0-pid123
	if n, _ := fmt.Sscanf(base, "socket:rank%d-", &rank); n == 1 {
		return rank
	}
	return 0
}

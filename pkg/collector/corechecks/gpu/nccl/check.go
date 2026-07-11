// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

// Package nccl contains the NCCL collective communication check implementation.
package nccl

import (
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	pkgos "github.com/DataDog/datadog-agent/pkg/util/os"
)

// Check represents the NCCL check that collects metrics from NCCL Inspector output
type Check struct {
	core.CheckBase
	tagger                     tagger.Component
	telemetry                  telemetry.Component
	wmeta                      workloadmeta.Component
	socketListener             *SocketListener
	processTagger              *ProcessTagger
	containerProvider          proccontainers.ContainerProvider
	containerProviderWarnLimit *log.Limit
	checkTelemetry             *ncclCheckTelemetry
	lastSeenRank               map[string]rankStalenessEntry // "<commID>:rank:<N>" → last event + timestamp for hang detection
	isProcessAlive             func(pid int) bool            // injectable for testing; defaults to pkgos.PidExists
}

// rankStalenessEntry tracks the last event seen for a rank, used for hang detection.
// Storing the ParsedEvent allows emitStalenessMetrics to emit the same pod/container
// tags as other NCCL metrics.
type rankStalenessEntry struct {
	lastSeen time.Time
	parsed   ParsedEvent
}

// rankKey is the lastSeenRank map key. Keying by commID + rank prevents
// concurrent jobs with overlapping rank numbers (both having rank 0..N)
// from overwriting each other's entries.
func rankKey(parsed ParsedEvent) string {
	return fmt.Sprintf("%s:rank:%d", parsed.Event.ID, parsed.Event.Rank)
}

type ncclCheckTelemetry struct {
	eventsProcessed telemetry.Counter
	parseErrors     telemetry.Counter
	eventsDropped   telemetry.Counter
	metricsSent     telemetry.Counter
}

func newCheckTelemetry(tm telemetry.Component) *ncclCheckTelemetry {
	return &ncclCheckTelemetry{
		eventsProcessed: tm.NewCounter(CheckName, "events_processed", nil, "Number of NCCL events processed"),
		parseErrors:     tm.NewCounter(CheckName, "parse_errors", nil, "Number of JSON parse errors"),
		eventsDropped:   tm.NewCounter(CheckName, "events_dropped", nil, "Number of NCCL events dropped due to the in-memory buffer cap (per-check-interval)"),
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
		CheckBase:                  core.NewCheckBase(CheckName),
		tagger:                     tagger,
		telemetry:                  telemetry,
		wmeta:                      wmeta,
		containerProviderWarnLimit: log.NewLogLimit(1, 10*time.Minute),
	}
}

// Configure parses the check configuration and initializes the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source, provider string) error {
	// Check if NCCL check is enabled
	if !pkgconfigsetup.Datadog().GetBool("gpu.nccl.enabled") {
		return errors.New("NCCL check is disabled")
	}

	if err := c.CommonConfigure(senderManager, initConfig, config, source, provider); err != nil {
		return err
	}

	socketPath := pkgconfigsetup.Datadog().GetString("gpu.nccl.socket_path")

	// Start socket listener. If the socket is unavailable (e.g. permissions issue),
	// log a warning and continue — matching DogStatsD/APM behaviour. The check
	// will run with no socket events; restart the agent after fixing the socket.
	sl, err := newSocketListener(socketPath)
	if err != nil {
		log.Warnf("NCCL check: socket listener failed to start, no metrics will be collected: %v", err)
	} else {
		c.socketListener = sl
	}

	// Initialize process tagger. containerProvider is acquired lazily in Run
	// (single code path) since GetSharedContainerProvider can fail here due to
	// component startup ordering.
	c.processTagger = NewProcessTagger(c.tagger, c.wmeta, nil, c.telemetry)

	// Initialize hang detection state
	c.lastSeenRank = make(map[string]rankStalenessEntry)
	c.isProcessAlive = pkgos.PidExists

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
	// (startup ordering) but is ready by the first Run call. Inject the provider
	// into the existing processTagger via SetContainerProvider. Failures are
	// rate-limited to avoid log spam (first immediately, then once per 10 min)
	// while still surfacing long-term unhealthy states.
	if c.containerProvider == nil {
		p, err := proccontainers.GetSharedContainerProvider()
		if err != nil {
			if c.containerProviderWarnLimit != nil && c.containerProviderWarnLimit.ShouldLog() {
				log.Warnf("NCCL check: shared container provider unavailable; pod tags will be missing until it initializes: %v", err)
			}
		} else {
			c.containerProvider = p
			if c.processTagger != nil {
				c.processTagger.SetContainerProvider(p)
			}
			log.Infof("NCCL check: container provider lazy-init succeeded")
		}
	}

	// Refresh process tagger to get fresh PID -> container mappings
	if c.processTagger != nil {
		c.processTagger.Refresh()
	}

	// Collect events from socket listener
	if c.socketListener == nil {
		return nil
	}
	events := c.socketListener.Drain()
	if n := c.socketListener.DrainParseErrors(); n > 0 {
		c.checkTelemetry.parseErrors.Add(float64(n))
	}
	if n := c.socketListener.DrainDropped(); n > 0 {
		c.checkTelemetry.eventsDropped.Add(float64(n))
		log.Warnf("NCCL check: dropped %d events due to in-memory buffer cap (%d). Increase check interval or investigate event volume.", n, maxPendingEvents)
	}

	// Process events and emit per-rank metrics
	for _, parsed := range events {
		if parsed.Event.CollPerf != nil {
			log.Tracef("NCCL coll_perf: rank=%d coll=%s exec_time_us=%.1f algobw=%.3f busbw=%.3f msg_bytes=%d timing=%s tags=%v",
				parsed.Event.Rank, parsed.Event.CollPerf.Collective, parsed.Event.CollPerf.ExecTimeUS,
				parsed.Event.CollPerf.AlgoBandwidthGB, parsed.Event.CollPerf.BusBandwidthGB,
				parsed.Event.CollPerf.MsgSizeBytes, parsed.Event.CollPerf.TimingSource, c.buildTags(parsed))
		}
		if err := c.processEvent(snd, parsed); err != nil {
			log.Debugf("error processing NCCL event: %v", err)
		}
		c.checkTelemetry.eventsProcessed.Inc()
	}
	log.Debugf("NCCL check: %d events", len(events))

	// Hang detection: update last-seen timestamps and emit staleness metrics.
	// Key by commID+rank so that concurrent jobs with overlapping rank numbers
	// (both having rank:0..N) don't collide and overwrite each other's entries.
	now := time.Now()
	for _, parsed := range events {
		c.lastSeenRank[rankKey(parsed)] = rankStalenessEntry{lastSeen: now, parsed: parsed}
	}
	c.emitStalenessMetrics(snd, now)

	return nil
}

// processEvent emits metrics for a single NCCL collective event.
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

	// Emit metrics as Distribution (DDSketch) so the agent computes
	// min/max/avg/p50/p95/p99 server-side across all events in the flush window.
	// Gauge would collapse 570 events/s to a single last-write-wins value, losing
	// straggler signal. Distribution preserves the tail latency.
	// Note: hang-detection staleness (seconds_since_last_event) stays as Gauge —
	// it is a point-in-time rank-health signal, not a distribution of samples.
	snd.Distribution(ncclMetricsNs+"collective.exec_time_us", perf.ExecTimeUS, "", tags)
	c.checkTelemetry.metricsSent.Inc("exec_time_us")

	// Bandwidth metrics
	if perf.AlgoBandwidthGB > 0 {
		snd.Distribution(ncclMetricsNs+"collective.algo_bandwidth_gbps", perf.AlgoBandwidthGB, "", tags)
		c.checkTelemetry.metricsSent.Inc("algo_bandwidth_gbps")
	}

	if perf.BusBandwidthGB > 0 {
		snd.Distribution(ncclMetricsNs+"collective.bus_bandwidth_gbps", perf.BusBandwidthGB, "", tags)
		c.checkTelemetry.metricsSent.Inc("bus_bandwidth_gbps")
	}

	// Message size
	if perf.MsgSizeBytes > 0 {
		snd.Distribution(ncclMetricsNs+"collective.msg_size_bytes", float64(perf.MsgSizeBytes), "", tags)
		c.checkTelemetry.metricsSent.Inc("msg_size_bytes")
	}

	return nil
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
			log.Tracef("failed to get workload tags for PID %d: %v", lookupPID, err)
		}
		tags = append(tags, workloadTags...)
	} else {
		tags = append(tags, fmt.Sprintf("pid:%d", event.PID))
	}

	// Append plugin-discovered extra tags (e.g. ray_job_id, ray_node_id)
	for k, v := range event.ExtraTags {
		tags = append(tags, k+":"+v)
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
// lastSeenRank is keyed by "<commID>:rank:<N>" so concurrent jobs with overlapping
// rank numbers don't collide and overwrite each other's staleness entries.
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

		// If the rank's process is gone, the job finished — evict immediately
		// to avoid a false-positive staleness spike.
		// Only use HostPID (from SO_PEERCRED): it is always a host-namespace PID
		// and can be reliably checked against /proc. event.PID may be a
		// container-namespace PID which would not exist on the host, causing
		// spurious eviction and silently disabling hang detection.
		if staleness > 0 && c.isProcessAlive != nil && entry.parsed.HostPID > 0 {
			if !c.isProcessAlive(entry.parsed.HostPID) {
				log.Debugf("NCCL hang detection: evicting %s — process %d no longer exists", rankKey, entry.parsed.HostPID)
				delete(c.lastSeenRank, rankKey)
				continue
			}
		}

		tags := c.buildTags(entry.parsed)
		tags = append(tags, fmt.Sprintf("rank:%d", entry.parsed.Event.Rank))
		if entry.parsed.Event.Hostname != "" {
			tags = append(tags, "nccl_hostname:"+entry.parsed.Event.Hostname)
		}
		snd.Gauge(ncclMetricsNs+hangDetectionMetric, staleness.Seconds(), "", tags)
	}
}

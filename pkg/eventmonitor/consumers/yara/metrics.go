// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Metrics emitted by the YARA consumer. They follow the event monitor metrics of system-probe
// (datadog.runtime_security.event_monitoring.*).
//
// Counters, emitted as the delta since the previous flush (nothing is sent for a zero delta):
//
//	datadog.runtime_security.event_monitoring.yara.execs_received   exec events handled by the consumer
//	datadog.runtime_security.event_monitoring.yara.identity_hits    execs skipped on a fresh file identity, no I/O
//	datadog.runtime_security.event_monitoring.yara.sha_hits         files read but skipped on an already claimed sha256
//	datadog.runtime_security.event_monitoring.yara.reads            files read and hashed
//	datadog.runtime_security.event_monitoring.yara.read_errors      files that could not be read, tagged reason:not_found|permission|other
//	datadog.runtime_security.event_monitoring.yara.too_big          files skipped for being over max_file_size
//	datadog.runtime_security.event_monitoring.yara.not_regular      files skipped for not being regular files
//	datadog.runtime_security.event_monitoring.yara.scans            scans run
//	datadog.runtime_security.event_monitoring.yara.matches          individual rule matches (a file matching 3 rules counts 3)
//	datadog.runtime_security.event_monitoring.yara.scan_errors      scans that failed
//	datadog.runtime_security.event_monitoring.yara.scan_timeouts    scans that hit scan_timeout
//	datadog.runtime_security.event_monitoring.yara.queue_drops      scans dropped because the scan queue was full
//
// Gauges, sampled at every flush:
//
//	datadog.runtime_security.event_monitoring.yara.identity_cache_size   identities held by the Deduper
//	datadog.runtime_security.event_monitoring.yara.sha_set_size          sha256 sums held by the Deduper (scanned or in flight)
//	datadog.runtime_security.event_monitoring.yara.queue_depth           jobs waiting for a scan worker
//
// Distribution, sent on every scan through Stats.ObserveScanDuration:
//
//	datadog.runtime_security.event_monitoring.yara.scan_duration   scan duration, in seconds
//
// Every metric also carries the tags given in MetricsOptions.Tags.
//
// Exec events dropped because the consumer channel was full are not emitted here: the probe
// already counts them as datadog.runtime_security.event_monitoring.events.dropped, tagged
// consumer_id:<the consumer ID()>.
const (
	metricPrefix = "datadog.runtime_security.event_monitoring.yara."

	metricExecsReceived = metricPrefix + "execs_received"
	metricIdentityHits  = metricPrefix + "identity_hits"
	metricShaHits       = metricPrefix + "sha_hits"
	metricReads         = metricPrefix + "reads"
	metricReadErrors    = metricPrefix + "read_errors"
	metricTooBig        = metricPrefix + "too_big"
	metricNotRegular    = metricPrefix + "not_regular"
	metricScans         = metricPrefix + "scans"
	metricMatches       = metricPrefix + "matches"
	metricScanErrors    = metricPrefix + "scan_errors"
	metricScanTimeouts  = metricPrefix + "scan_timeouts"
	metricQueueDrops    = metricPrefix + "queue_drops"

	metricIdentityCacheSize = metricPrefix + "identity_cache_size"
	metricShaSetSize        = metricPrefix + "sha_set_size"
	metricQueueDepth        = metricPrefix + "queue_depth"

	metricScanDuration = metricPrefix + "scan_duration"
)

// defaultMetricsInterval is the default flush interval of Metrics
const defaultMetricsInterval = 10 * time.Second

// NewScanDurationObserver returns a hook sending every scan duration to the scan_duration
// distribution. Install it before the pipeline starts:
//
//	stats := &Stats{ObserveScanDuration: NewScanDurationObserver(client, tags)}
//
// It is independent of Metrics so that it can be installed before the Deduper and ScanPool that
// Metrics samples are built.
func NewScanDurationObserver(client statsd.ClientInterface, tags []string) func(time.Duration) {
	tags = slices.Clone(tags)
	return func(d time.Duration) {
		_ = client.Distribution(metricScanDuration, d.Seconds(), tags, 1.0)
	}
}

// MetricsOptions configures Metrics
type MetricsOptions struct {
	// Interval is the flush interval. Zero means defaultMetricsInterval.
	Interval time.Duration
	// Tags are added to every metric
	Tags []string
}

type counterMetric struct {
	name  string
	tags  []string
	value *atomic.Int64
	last  int64
}

// Metrics periodically reads Stats, the Deduper and the ScanPool, and sends their values to
// statsd. It does not count anything itself.
type Metrics struct {
	client   statsd.ClientInterface
	deduper  Deduper
	pool     ScanPool
	interval time.Duration
	tags     []string

	mu       sync.Mutex // serializes flushes
	counters []*counterMetric

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewMetrics returns a Metrics reading stats. deduper and pool are sampled for gauges, and may be
// nil to skip their gauges.
func NewMetrics(client statsd.ClientInterface, stats *Stats, deduper Deduper, pool ScanPool, opts MetricsOptions) *Metrics {
	if opts.Interval <= 0 {
		opts.Interval = defaultMetricsInterval
	}
	tags := slices.Clone(opts.Tags)

	m := &Metrics{
		client:   client,
		deduper:  deduper,
		pool:     pool,
		interval: opts.Interval,
		tags:     tags,
	}

	add := func(name string, value *atomic.Int64, extraTags ...string) {
		m.counters = append(m.counters, &counterMetric{
			name:  name,
			tags:  append(slices.Clone(tags), extraTags...),
			value: value,
		})
	}
	add(metricExecsReceived, &stats.ExecsReceived)
	add(metricIdentityHits, &stats.IdentityHits)
	add(metricShaHits, &stats.ShaHits)
	add(metricReads, &stats.Reads)
	for reason := ReadErrorReason(0); reason < readErrorReasonCount; reason++ {
		add(metricReadErrors, &stats.ReadErrors[reason], "reason:"+reason.String())
	}
	add(metricTooBig, &stats.TooBig)
	add(metricNotRegular, &stats.NotRegular)
	add(metricScans, &stats.Scans)
	add(metricMatches, &stats.Matches)
	add(metricScanErrors, &stats.ScanErrors)
	add(metricScanTimeouts, &stats.ScanTimeouts)
	add(metricQueueDrops, &stats.QueueDrops)

	return m
}

// Start starts flushing every interval until ctx is done or Stop is called. It must be called at
// most once, and not concurrently with Stop.
func (m *Metrics) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.flush()
			}
		}
	}()
}

// Stop stops the flush goroutine, waits for it to exit, then flushes one last time so that the
// counts since the previous flush are not lost. It is safe to call without Start.
func (m *Metrics) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
	m.flush()
}

// flush sends counter deltas since the previous flush, and the current gauge values
func (m *Metrics) flush() {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs int
	for _, c := range m.counters {
		cur := c.value.Load()
		delta := cur - c.last
		if delta == 0 {
			continue
		}
		c.last = cur
		if err := m.client.Count(c.name, delta, c.tags, 1.0); err != nil {
			errs++
		}
	}

	if m.deduper != nil {
		identities, hashes := m.deduper.Sizes()
		if err := m.client.Gauge(metricIdentityCacheSize, float64(identities), m.tags, 1.0); err != nil {
			errs++
		}
		if err := m.client.Gauge(metricShaSetSize, float64(hashes), m.tags, 1.0); err != nil {
			errs++
		}
	}
	if m.pool != nil {
		if err := m.client.Gauge(metricQueueDepth, float64(m.pool.QueueDepth()), m.tags, 1.0); err != nil {
			errs++
		}
	}

	if errs > 0 {
		log.Debugf("yara: failed to send %d metrics", errs)
	}
}

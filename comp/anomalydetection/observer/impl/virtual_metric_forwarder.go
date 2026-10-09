// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/checksfit"
	"github.com/DataDog/datadog-agent/comp/anomalydetection/checksfit/fitcore"
	anomalydetectionconfig "github.com/DataDog/datadog-agent/comp/anomalydetection/config"
	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logging"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

const (
	// counterBucketInterval is the width of the counter aggregation window.
	// It matches the whole-second buckets the analysis process stores samples
	// in, so each flushed record carries the complete count for one second.
	counterBucketInterval = time.Second
	// summaryInterval is how often the forwarder reports its counters.
	summaryInterval = time.Minute
	// pendingCapacity bounds the queue between the log ingest path and the
	// forwarder. Beyond it, observations are dropped and counted.
	pendingCapacity = 1024
	// tagSeparator joins tags in a series key. Log tags cannot contain it.
	tagSeparator = "\x1f"
)

// virtualMetricForwarder sends the virtual metrics produced by log metrics
// extractors (log pattern counts and any other log-derived series) to the
// isolated anomaly detection process over FIT, using the DDCHECKS v1 protocol.
//
// Series identity is preserved end to end: the metric name travels verbatim
// (e.g. "log.log_pattern_extractor.<hash>.count", which carries no "datadog."
// prefix and is therefore not telemetry), the resolved host travels in the
// record's hostname field, and the series tags are the log tags plus the
// observer source tag.
//
// Counters are aggregated per series and per second before sending, so each
// record holds the full count for one second. The analysis process stores
// samples in whole-second buckets and detects on their average, so one record
// per series per second keeps its series values identical to the counts.
// Non-counter series (log field gauges) are forwarded as they are observed.
type virtualMetricForwarder struct {
	endpoint      string
	retryInterval time.Duration
	debugLogs     bool

	pending chan observerdef.VirtualMetric

	cancel context.CancelFunc
	done   chan struct{}

	submitted atomic.Uint64
	dropped   atomic.Uint64

	// warnOnce guards the queue-full warning so a stalled reader cannot flood
	// the agent log.
	warnOnce sync.Once
}

// newVirtualMetricForwarder creates a forwarder that connects to the configured
// FIT endpoint. It returns a nil forwarder without an error when this platform
// has no FIT transport, so the observer keeps running without forwarding.
func newVirtualMetricForwarder(cfg anomalydetectionconfig.LogPatternForwardingConfig) (*virtualMetricForwarder, error) {
	if !checksfit.Supported {
		logging.Warnf("%s is enabled but FIT is unavailable on this platform; log pattern metrics are not forwarded",
			anomalydetectionconfig.LogPatternForwardingEnabledConfigKey)
		return nil, nil
	}
	endpoint, err := fitcore.ParseEndpoint(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid FIT endpoint %q: %w", cfg.Endpoint, err)
	}

	forwarder := &virtualMetricForwarder{
		endpoint:      cfg.Endpoint,
		retryInterval: cfg.RetryInterval,
		debugLogs:     cfg.DebugLogs,
		pending:       make(chan observerdef.VirtualMetric, pendingCapacity),
		done:          make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	forwarder.cancel = cancel
	go forwarder.run(ctx, fitcore.NewProducerConfig(endpoint))
	return forwarder, nil
}

// ObserveVirtualMetric implements observerdef.VirtualMetricSink. It never
// blocks the log ingest path: when the pending queue is full the observation is
// dropped and counted.
func (f *virtualMetricForwarder) ObserveVirtualMetric(metric observerdef.VirtualMetric) {
	select {
	case f.pending <- metric:
	default:
		f.dropped.Add(1)
		f.warnOnce.Do(func() {
			logging.Warnf("log pattern metric forwarding queue is full; dropping observations until the FIT session catches up")
		})
	}
}

// run connects to the analysis process and forwards until the context ends,
// reconnecting after a broken session.
func (f *virtualMetricForwarder) run(ctx context.Context, config fitcore.ProducerConfig) {
	defer close(f.done)
	for {
		producer, err := checksfit.ConnectContext(ctx, config)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logging.Warnf("log pattern metric forwarding cannot connect to %s (retrying in %s): %v",
				f.endpoint, f.retryInterval, err)
			if !sleepUntil(ctx, f.retryInterval) {
				return
			}
			continue
		}
		logging.Infof("log pattern metric forwarding connected to %s (session %d)", f.endpoint, producer.SessionID())

		f.serve(ctx, producer)
		_ = producer.Close()
		if ctx.Err() != nil {
			return
		}
		if !sleepUntil(ctx, f.retryInterval) {
			return
		}
	}
}

// serve forwards observations over one established session until the context
// ends or the session breaks.
func (f *virtualMetricForwarder) serve(ctx context.Context, producer *checksfit.Producer) {
	counters := map[counterKey]float64{}
	gauges := make([]checksfit.Metric, 0, len(f.pending))
	flush := time.NewTicker(counterBucketInterval)
	defer flush.Stop()
	summary := time.NewTicker(summaryInterval)
	defer summary.Stop()
	reportedSubmitted, reportedDropped := uint64(0), uint64(0)

	for {
		select {
		case <-ctx.Done():
			return
		case metric := <-f.pending:
			f.observe(counters, &gauges, metric)
		case <-flush.C:
			if !f.send(ctx, producer, counters, &gauges) {
				return
			}
		case <-summary.C:
			f.logSummary(&reportedSubmitted, &reportedDropped)
		}
	}
}

// observe folds one virtual metric into the pending batch.
func (f *virtualMetricForwarder) observe(counters map[counterKey]float64, gauges *[]checksfit.Metric, metric observerdef.VirtualMetric) {
	if f.debugLogs {
		logging.Debugf("[forward] %s = %v host=%q tags=[%s]",
			metric.Name, metric.Value, metric.Host, strings.Join(f.tagsFor(metric), " "))
	}
	if isLogCountMetric(metric.Name) {
		counters[counterKeyOf(metric, f.tagsFor(metric))] += metric.Value
		return
	}
	*gauges = append(*gauges, checksfit.Metric{
		MetricType: checksfit.MetricTypeGauge,
		Name:       metric.Name,
		Value:      metric.Value,
		Timestamp:  metric.Timestamp,
		Tags:       f.tagsFor(metric),
		Hostname:   metric.Host,
	})
}

// send publishes the pending batch. It reports false when the session broke, so
// the caller reconnects.
func (f *virtualMetricForwarder) send(
	ctx context.Context,
	producer *checksfit.Producer,
	counters map[counterKey]float64,
	gauges *[]checksfit.Metric,
) bool {
	batch := make([]checksfit.Metric, 0, len(counters)+len(*gauges))
	for key, value := range counters {
		batch = append(batch, key.metric(value))
		delete(counters, key)
	}
	batch = append(batch, *gauges...)
	*gauges = (*gauges)[:0]
	if len(batch) == 0 {
		return true
	}

	outcome, err := producer.SendMetrics(batch)
	if err != nil {
		f.dropped.Add(uint64(len(batch)))
		if ctx.Err() == nil {
			logging.Warnf("log pattern metric forwarding session failed, reconnecting: %v", err)
		}
		return false
	}
	f.submitted.Add(uint64(outcome.Accepted))
	if dropped := len(batch) - outcome.Accepted; dropped > 0 {
		f.dropped.Add(uint64(dropped))
		logging.Warnf("log pattern metric forwarding dropped %d of %d records: the analysis process is behind (%s)",
			dropped, len(batch), outcome.QueueRejection)
	}
	if outcome.EncodingError != nil {
		// Encoding only fails on oversized payloads, which the extractors do
		// not produce. Report it rather than silently losing data.
		f.dropped.Add(1)
		logging.Warnf("log pattern metric forwarding could not encode a record: %v", outcome.EncodingError)
	}
	return true
}

// logSummary reports progress since the previous summary, staying quiet when
// nothing was forwarded.
func (f *virtualMetricForwarder) logSummary(reportedSubmitted, reportedDropped *uint64) {
	submitted, dropped := f.submitted.Load(), f.dropped.Load()
	if submitted == *reportedSubmitted && dropped == *reportedDropped {
		return
	}
	logging.Infof("log pattern metric forwarding: %d records submitted, %d dropped since the last summary",
		submitted-*reportedSubmitted, dropped-*reportedDropped)
	*reportedSubmitted, *reportedDropped = submitted, dropped
}

// tagsFor returns the series tags, appending a "host:" tag when the resolved
// host is not already represented. This mirrors the pattern clusterer's
// grouping key, which splits clusters on the host dimension.
func (f *virtualMetricForwarder) tagsFor(metric observerdef.VirtualMetric) []string {
	// Read-only view: the forwarder never mutates or retains it.
	tags := metric.Tags.UnsafeToReadOnlySliceString()
	if metric.Host == "" {
		return tags
	}
	for _, tag := range tags {
		if strings.HasPrefix(tag, "host:") {
			return tags
		}
	}
	// Copy before appending: the caller owns tags and may reuse its backing array.
	out := make([]string, 0, len(tags)+1)
	out = append(out, tags...)
	return append(out, "host:"+metric.Host)
}

// close stops the forwarder and releases the FIT session.
func (f *virtualMetricForwarder) close() error {
	if f == nil {
		return nil
	}
	if f.cancel != nil {
		f.cancel()
		<-f.done
	}
	return nil
}

// stats returns the number of accepted and dropped records. Used by tests.
func (f *virtualMetricForwarder) stats() (submitted, dropped uint64) {
	return f.submitted.Load(), f.dropped.Load()
}

// counterKey identifies one series and one second. Counters for the same series
// and second are summed into a single record.
type counterKey struct {
	name         string
	host         string
	tags         string
	timestampSec uint64
}

// counterKeyOf builds the aggregation key of one counter observation. The tags
// are the resolved series tags, so the key and the emitted record agree on the
// host dimension.
func counterKeyOf(metric observerdef.VirtualMetric, tags []string) counterKey {
	return counterKey{
		name:         metric.Name,
		host:         metric.Host,
		tags:         strings.Join(tags, tagSeparator),
		timestampSec: metric.Timestamp,
	}
}

// metric renders the aggregated counter as a DDCHECKS counter record.
func (k counterKey) metric(value float64) checksfit.Metric {
	return checksfit.Metric{
		MetricType: checksfit.MetricTypeCounter,
		Name:       k.name,
		Value:      value,
		Timestamp:  k.timestampSec,
		Tags:       splitTags(k.tags),
		Hostname:   k.host,
	}
}

// splitTags reverses the key's tag encoding. An empty tag set encodes as one
// empty field, which must not become a tag.
func splitTags(encoded string) []string {
	if encoded == "" {
		return nil
	}
	return strings.Split(encoded, tagSeparator)
}

// isLogCountMetric reports whether a virtual metric is a log count, following
// the same ".count" convention as the log count bucketizer.
func isLogCountMetric(name string) bool {
	return strings.HasSuffix(name, ".count")
}

// sleepUntil waits for the duration or returns false when the context ended.
func sleepUntil(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package writer

import (
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/logutil"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const maxPendingStatsPayloads = 10

// StatsSyncWriter ingests stats buckets and flushes them to the API.
type StatsSyncWriter struct {
	in       <-chan []stats.Bucket
	hostname string
	env      string
	senders  []*sender
	stop     chan struct{}
	stats    *info.StatsWriterInfo
	payloads chan *payload // payloads buffered
	easylog  *logutil.ThrottledLogger
}

// NewStatsSyncWriter returns a new StatsWriter. It must be started using Run.
func NewStatsSyncWriter(cfg *config.AgentConfig, in <-chan []stats.Bucket) *StatsSyncWriter {
	sw := &StatsSyncWriter{
		in:       in,
		hostname: cfg.Hostname,
		env:      cfg.DefaultEnv,
		stats:    &info.StatsWriterInfo{},
		stop:     make(chan struct{}),
		payloads: make(chan *payload, maxPendingStatsPayloads),
		easylog:  logutil.NewThrottled(5, 10*time.Second), // no more than 5 messages every 10 seconds
	}
	climit := cfg.StatsWriter.ConnectionLimit
	if climit == 0 {
		// allow 1% of the connection limit to outgoing sends.
		climit = int(math.Max(1, float64(cfg.ConnectionLimit)/100))
	}
	qsize := cfg.StatsWriter.QueueSize
	if qsize == 0 {
		payloadSize := float64(maxEntriesPerPayload * bytesPerEntry)
		// default to 25% of maximum memory.
		maxmem := cfg.MaxMemory / 4
		if maxmem == 0 {
			// or 250MB if unbound
			maxmem = 250 * 1024 * 1024
		}
		qsize = int(math.Max(1, maxmem/payloadSize))
	}
	log.Debugf("Stats writer initialized (climit=%d qsize=%d)", climit, qsize)
	sw.senders = newSenders(cfg, sw, pathStats, climit, qsize)
	return sw
}

// Run starts the StatsWriter, making it ready to receive stats and report metrics.
func (w *StatsSyncWriter) Run() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	defer close(w.stop)
	for {
		select {
		case stats := <-w.in:
			w.addStats(stats)
		case <-t.C:
			w.report()
		case <-w.stop:
			return
		}
	}
}

// SyncFlush immediately sends all pending payloads
func (w *StatsSyncWriter) SyncFlush() {
	defer w.report()

	// Collect all pending payloads from the channel
	// and send them.
outer:
	for {
		select {
		case p := <-w.payloads:
			sendPayloads(w.senders, p)
		default:
			break outer
		}
	}
	// Wait for all the senders to finish
	wg := sync.WaitGroup{}
	for _, sender := range w.senders {
		s := sender
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.waitForInflight()
		}()
	}
	wg.Wait()
}

// Stop stops a running StatsWriter.
func (w *StatsSyncWriter) Stop() {
	w.stop <- struct{}{}
	<-w.stop
	stopSenders(w.senders)
}

func (w *StatsSyncWriter) addStats(s []stats.Bucket) {
	defer timing.Since("datadog.trace_agent.stats_writer.encode_ms", time.Now())

	payloads, bucketCount, entryCount := buildPayloads(s, maxEntriesPerPayload, w.hostname, w.env)
	switch n := len(payloads); {
	case n == 0:
		return
	case n > 1:
		atomic.AddInt64(&w.stats.Splits, 1)
	}
	atomic.AddInt64(&w.stats.StatsBuckets, int64(bucketCount))
	log.Debugf("Flushing %d entries (buckets=%d payloads=%v)", entryCount, bucketCount, len(payloads))

	for _, p := range payloads {
		req := newPayload(map[string]string{
			headerLanguages:    strings.Join(info.Languages(), "|"),
			"Content-Type":     "application/json",
			"Content-Encoding": "gzip",
		})
		if err := stats.EncodePayload(req.body, p); err != nil {
			log.Errorf("Stats encoding error: %v", err)
			return
		}
		atomic.AddInt64(&w.stats.Bytes, int64(req.body.Len()))
		// Add to payloads channel, unless it's full
		select {
		case w.payloads <- req:
		default:
			log.Errorf("Channel full. Discarding trace")
		}
	}
}

var _ eventRecorder = (*StatsWriter)(nil)

func (w *StatsSyncWriter) report() {
	metrics.Count("datadog.trace_agent.trace_sync_writer.payloads", atomic.SwapInt64(&w.stats.Payloads, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_sync_writer.stats_buckets", atomic.SwapInt64(&w.stats.StatsBuckets, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_sync_writer.bytes", atomic.SwapInt64(&w.stats.Bytes, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_sync_writer.retries", atomic.SwapInt64(&w.stats.Retries, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_sync_writer.splits", atomic.SwapInt64(&w.stats.Splits, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_sync_writer.errors", atomic.SwapInt64(&w.stats.Errors, 0), nil, 1)
}

// recordEvent implements eventRecorder.
func (w *StatsSyncWriter) recordEvent(t eventType, data *eventData) {
	if data != nil {
		metrics.Histogram("datadog.trace_agent.trace_sync_writer.connection_fill", data.connectionFill, nil, 1)
		metrics.Histogram("datadog.trace_agent.trace_sync_writer.queue_fill", data.queueFill, nil, 1)
	}
	switch t {
	case eventTypeRetry:
		log.Debugf("Retrying to flush stats payload (error: %q)", data.err)
		atomic.AddInt64(&w.stats.Retries, 1)

	case eventTypeSent:
		log.Debugf("Flushed stats to the API; time: %s, bytes: %d", data.duration, data.bytes)
		timing.Since("datadog.trace_agent.trace_sync_writer.flush_duration", time.Now().Add(-data.duration))
		atomic.AddInt64(&w.stats.Bytes, int64(data.bytes))
		atomic.AddInt64(&w.stats.Payloads, 1)

	case eventTypeRejected:
		log.Warnf("Stats writer payload rejected by edge: %v", data.err)
		atomic.AddInt64(&w.stats.Errors, 1)

	case eventTypeDropped:
		w.easylog.Warn("Stats writer queue full. Payload dropped (%.2fKB).", float64(data.bytes)/1024)
		metrics.Count("datadog.trace_agent.trace_sync_writer.dropped", 1, nil, 1)
		metrics.Count("datadog.trace_agent.trace_sync_writer.dropped_bytes", int64(data.bytes), nil, 1)
	}
}

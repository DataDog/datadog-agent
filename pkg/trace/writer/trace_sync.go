// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package writer

import (
	"compress/gzip"
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
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/gogo/protobuf/proto"
)

const maxPendingTracePayloads = 10

// TraceSyncWriter buffers traces and APM events, flushing them only when the Flush method
// is manually called.
type TraceSyncWriter struct {
	in <-chan *SampledSpans

	hostname string
	env      string
	senders  []*sender
	stop     chan struct{}
	stats    *info.TraceWriterInfo

	processedTraces sync.WaitGroup
	payloads        chan *payload // payloads buffered

	easylog *logutil.ThrottledLogger
}

// NewTraceSyncWriter returns a new TraceSyncWriter. It is created for the given agent configuration and
// will accept incoming spans via the in channel.
func NewTraceSyncWriter(cfg *config.AgentConfig, in <-chan *SampledSpans) *TraceSyncWriter {
	tw := &TraceSyncWriter{
		in:       in,
		hostname: cfg.Hostname,
		env:      cfg.DefaultEnv,
		stats:    &info.TraceWriterInfo{},
		stop:     make(chan struct{}),
		payloads: make(chan *payload, maxPendingTracePayloads),
		easylog:  logutil.NewThrottled(5, 10*time.Second), // no more than 5 messages every 10 seconds
	}
	climit := cfg.TraceWriter.ConnectionLimit
	if climit == 0 {
		// default to 10% of the connection limit to outgoing sends.
		climit = int(math.Max(1, float64(cfg.ConnectionLimit)/10))
	}
	qsize := cfg.TraceWriter.QueueSize
	if qsize == 0 {
		// default to 50% of maximum memory.
		maxmem := cfg.MaxMemory / 2
		if maxmem == 0 {
			// or 500MB if unbound
			maxmem = 500 * 1024 * 1024
		}
		qsize = int(math.Max(1, maxmem/float64(MaxPayloadSize)))
	}
	log.Debugf("Trace sync writer initialized (climit=%d qsize=%d)", climit, qsize)
	tw.senders = newSenders(cfg, tw, pathTraces, climit, qsize)
	return tw
}

// Stop stops the TraceWriter and attempts to flush whatever is left in the senders buffers.
func (w *TraceSyncWriter) Stop() {
	log.Debug("Exiting trace writer. Trying to flush whatever is left...")
	w.stop <- struct{}{}
	<-w.stop
	w.SyncFlush()
	stopSenders(w.senders)
}

// Run starts the TraceWriter.
func (w *TraceSyncWriter) Run() {
	defer close(w.stop)
	for {
		select {
		case pkg := <-w.in:
			w.processPayload(pkg)
		case <-w.stop:
			// drain the input channel before stopping
		outer:
			for {
				select {
				case pkg := <-w.in:
					w.processPayload(pkg)
				default:
					break outer
				}
			}
			return
		}
	}
}

// Flush writes any pending payloads synchronously
func (w *TraceSyncWriter) SyncFlush() {
	defer w.report()

	// Collect all pending payloads from the channel
	// and send them.
	pc := 0
outer:
	for {
		select {
		case p := <-w.payloads:
			pc++
			sendPayloads(w.senders, p)
		default:
			break outer
		}
	}
	log.Debugf("Payload count (payloads=%d)", pc)

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

func (w *TraceSyncWriter) processPayload(pkg *SampledSpans) {
	if len(pkg.Traces) == 0 && len(pkg.Events) == 0 {
		return
	}

	atomic.AddInt64(&w.stats.Spans, int64(len(pkg.Traces)))
	atomic.AddInt64(&w.stats.Traces, 1)
	atomic.AddInt64(&w.stats.Events, int64(len(pkg.Events)))

	defer timing.Since("datadog.trace_agent.trace_async_writer.encode_ms", time.Now())

	traces := pkg.Traces
	log.Debugf("Serializing %d traces and %d APM events.", len(traces), len(pkg.Events))

	tracePayload := pb.TracePayload{
		HostName:     w.hostname,
		Env:          w.env,
		Traces:       traces,
		Transactions: pkg.Events,
	}
	b, err := proto.Marshal(&tracePayload)
	if err != nil {
		log.Errorf("Failed to serialize payload, data dropped: %v", err)
		return
	}

	atomic.AddInt64(&w.stats.BytesUncompressed, int64(len(b)))

	defer timing.Since("datadog.trace_agent.trace_writer.compress_ms", time.Now())
	p := newPayload(map[string]string{
		"Content-Type":     "application/x-protobuf",
		"Content-Encoding": "gzip",
		headerLanguages:    strings.Join(info.Languages(), "|"),
	})
	gzipw, err := gzip.NewWriterLevel(p.body, gzip.BestSpeed)
	if err != nil {
		// it will never happen, unless an invalid compression is chosen;
		// we know gzip.BestSpeed is valid.
		log.Errorf("gzip.NewWriterLevel: %d", err)
		return
	}
	if _, err := gzipw.Write(b); err != nil {
		log.Errorf("Error gzipping trace payload: %v", err)
	}
	if err := gzipw.Close(); err != nil {
		log.Errorf("Error closing gzip stream when writing trace payload: %v", err)
	}
	// Add to payloads channel, unless it's full
	select {
	case w.payloads <- p:
	default:
		log.Errorf("Channel full. Discarding trace")
	}
}

func (w *TraceSyncWriter) report() {
	metrics.Count("datadog.trace_agent.trace_sync_writer.payloads", atomic.SwapInt64(&w.stats.Payloads, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_sync_writer.bytes_uncompressed", atomic.SwapInt64(&w.stats.BytesUncompressed, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_sync_writer.retries", atomic.SwapInt64(&w.stats.Retries, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_sync_writer.bytes_estimated", atomic.SwapInt64(&w.stats.BytesEstimated, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_sync_writer.bytes", atomic.SwapInt64(&w.stats.Bytes, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_sync_writer.errors", atomic.SwapInt64(&w.stats.Errors, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_sync_writer.traces", atomic.SwapInt64(&w.stats.Traces, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_sync_writer.events", atomic.SwapInt64(&w.stats.Events, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_sync_writer.spans", atomic.SwapInt64(&w.stats.Spans, 0), nil, 1)
}

var _ eventRecorder = (*TraceWriter)(nil)

// recordEvent implements eventRecorder.
func (w *TraceSyncWriter) recordEvent(t eventType, data *eventData) {
	if data != nil {
		metrics.Histogram("datadog.trace_agent.trace_sync_writer.connection_fill", data.connectionFill, nil, 1)
		metrics.Histogram("datadog.trace_agent.trace_sync_writer.queue_fill", data.queueFill, nil, 1)
	}
	switch t {
	case eventTypeRetry:
		log.Debugf("Retrying to flush trace payload; error: %s", data.err)
		atomic.AddInt64(&w.stats.Retries, 1)

	case eventTypeSent:
		log.Debugf("Flushed traces to the API; time: %s, bytes: %d", data.duration, data.bytes)
		timing.Since("datadog.trace_agent.trace_sync_writer.flush_duration", time.Now().Add(-data.duration))
		atomic.AddInt64(&w.stats.Bytes, int64(data.bytes))
		atomic.AddInt64(&w.stats.Payloads, 1)

	case eventTypeRejected:
		log.Warnf("Trace writer payload rejected by edge: %v", data.err)
		atomic.AddInt64(&w.stats.Errors, 1)

	case eventTypeDropped:
		w.easylog.Warn("Trace writer queue full. Payload dropped (%.2fKB).", float64(data.bytes)/1024)
		metrics.Count("datadog.trace_agent.trace_sync_writer.dropped", 1, nil, 1)
		metrics.Count("datadog.trace_agent.trace_sync_writer.dropped_bytes", int64(data.bytes), nil, 1)
	}
}

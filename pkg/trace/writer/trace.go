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

// pathTraces is the target host API path for delivering traces.
const pathTraces = "/api/v0.2/traces"

// MaxPayloadSize specifies the maximum accumulated payload size that is allowed before
// a flush is triggered; replaced in tests.
var MaxPayloadSize = 3200000 // 3.2MB is the maximum allowed by the Datadog API

// SampledSpans represents the result of a trace sampling operation.
type SampledSpans struct {
	// Trace will contain a trace if it was sampled or be empty if it wasn't.
	Traces []*pb.APITrace
	// Events contains all APM events extracted from a trace. If no events were extracted, it will be empty.
	Events []*pb.Span
	// Size represents the approximated message size in bytes.
	Size int
	// SpanCount specifies the total number of spans found in Traces.
	SpanCount int64
}

// TraceWriter buffers traces and APM events, flushing them to the Datadog API.
type TraceWriter struct {
	// In receives sampled spans to be processed by the trace writer.
	// Channel should only be received from when testing.
	In chan *SampledSpans

	hostname string
	env      string
	senders  []*sender
	stop     chan struct{}
	stats    *info.TraceWriterInfo
	wg       sync.WaitGroup // waits for gzippers
	tick     time.Duration  // flush frequency

	traces       []*pb.APITrace // traces buffered
	events       []*pb.Span     // events buffered
	bufferedSize int            // estimated buffer size

	easylog *logutil.ThrottledLogger
}

// NewTraceWriter returns a new TraceWriter. It is created for the given agent configuration and
// will accept incoming spans via the in channel.
func NewTraceWriter(cfg *config.AgentConfig) *TraceWriter {
	tw := &TraceWriter{
		In:       make(chan *SampledSpans, 1000),
		hostname: cfg.Hostname,
		env:      cfg.DefaultEnv,
		stats:    &info.TraceWriterInfo{},
		stop:     make(chan struct{}),
		tick:     5 * time.Second,
		easylog:  logutil.NewThrottled(5, 10*time.Second), // no more than 5 messages every 10 seconds
	}
	climit := cfg.TraceWriter.ConnectionLimit
	if climit == 0 {
		// Default to 10% of the connection limit to outgoing sends.
		// Since the connection limit was removed, keep this at 200
		// as it was when we had it (2k).
		climit = 200
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
	if s := cfg.TraceWriter.FlushPeriodSeconds; s != 0 {
		tw.tick = time.Duration(s*1000) * time.Millisecond
	}
	log.Debugf("Trace writer initialized (climit=%d qsize=%d)", climit, qsize)
	tw.senders = newSenders(cfg, tw, pathTraces, climit, qsize)
	return tw
}

// Stop stops the TraceWriter and attempts to flush whatever is left in the senders buffers.
func (w *TraceWriter) Stop() {
	log.Debug("Exiting trace writer. Trying to flush whatever is left...")
	w.stop <- struct{}{}
	<-w.stop
	w.wg.Wait()
	stopSenders(w.senders)
}

// Run starts the TraceWriter.
func (w *TraceWriter) Run() {
	t := time.NewTicker(w.tick)
	defer t.Stop()
	defer close(w.stop)
	for {
		select {
		case pkg := <-w.In:
			w.addSpans(pkg)
		case <-w.stop:
			// drain the input channel before stopping
		outer:
			for {
				select {
				case pkg := <-w.In:
					w.addSpans(pkg)
				default:
					break outer
				}
			}
			w.flush()
			return
		case <-t.C:
			w.report()
			w.flush()
		}
	}
}

func (w *TraceWriter) addSpans(pkg *SampledSpans) {
	atomic.AddInt64(&w.stats.Spans, pkg.SpanCount)
	atomic.AddInt64(&w.stats.Traces, int64(len(pkg.Traces)))
	atomic.AddInt64(&w.stats.Events, int64(len(pkg.Events)))

	size := pkg.Size
	if size+w.bufferedSize > MaxPayloadSize {
		// reached maximum allowed buffered size
		w.flush()
	}
	if len(pkg.Traces) > 0 {
		log.Tracef("Handling new trace with %d spans: %v", pkg.SpanCount, pkg.Traces)
		w.traces = append(w.traces, pkg.Traces...)
	}
	if len(pkg.Events) > 0 {
		log.Tracef("Handling new package with %d events: %v", len(pkg.Events), pkg.Events)
		w.events = append(w.events, pkg.Events...)
	}
	w.bufferedSize += size
}

func (w *TraceWriter) resetBuffer() {
	w.bufferedSize = 0
	w.traces = w.traces[:0]
	w.events = w.events[:0]
}

const headerLanguages = "X-Datadog-Reported-Languages"

func (w *TraceWriter) flush() {
	if len(w.traces) == 0 && len(w.events) == 0 {
		// nothing to do
		return
	}

	defer timing.Since("datadog.trace_agent.trace_writer.encode_ms", time.Now())
	defer w.resetBuffer()

	log.Debugf("Serializing %d traces and %d APM events.", len(w.traces), len(w.events))
	tracePayload := pb.TracePayload{
		HostName:     w.hostname,
		Env:          w.env,
		Traces:       w.traces,
		Transactions: w.events,
	}
	b, err := proto.Marshal(&tracePayload)
	if err != nil {
		log.Errorf("Failed to serialize payload, data dropped: %v", err)
		return
	}

	atomic.AddInt64(&w.stats.BytesUncompressed, int64(len(b)))
	atomic.AddInt64(&w.stats.BytesEstimated, int64(w.bufferedSize))

	w.wg.Add(1)
	go func() {
		defer timing.Since("datadog.trace_agent.trace_writer.compress_ms", time.Now())
		defer w.wg.Done()
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

		sendPayloads(w.senders, p)
	}()
}

func (w *TraceWriter) report() {
	metrics.Count("datadog.trace_agent.trace_writer.payloads", atomic.SwapInt64(&w.stats.Payloads, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.bytes_uncompressed", atomic.SwapInt64(&w.stats.BytesUncompressed, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.retries", atomic.SwapInt64(&w.stats.Retries, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.bytes_estimated", atomic.SwapInt64(&w.stats.BytesEstimated, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.bytes", atomic.SwapInt64(&w.stats.Bytes, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.errors", atomic.SwapInt64(&w.stats.Errors, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.traces", atomic.SwapInt64(&w.stats.Traces, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.events", atomic.SwapInt64(&w.stats.Events, 0), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.spans", atomic.SwapInt64(&w.stats.Spans, 0), nil, 1)
}

var _ eventRecorder = (*TraceWriter)(nil)

// recordEvent implements eventRecorder.
func (w *TraceWriter) recordEvent(t eventType, data *eventData) {
	if data != nil {
		metrics.Histogram("datadog.trace_agent.trace_writer.connection_fill", data.connectionFill, nil, 1)
		metrics.Histogram("datadog.trace_agent.trace_writer.queue_fill", data.queueFill, nil, 1)
	}
	switch t {
	case eventTypeRetry:
		log.Debugf("Retrying to flush trace payload; error: %s", data.err)
		atomic.AddInt64(&w.stats.Retries, 1)

	case eventTypeSent:
		log.Debugf("Flushed traces to the API; time: %s, bytes: %d", data.duration, data.bytes)
		timing.Since("datadog.trace_agent.trace_writer.flush_duration", time.Now().Add(-data.duration))
		atomic.AddInt64(&w.stats.Bytes, int64(data.bytes))
		atomic.AddInt64(&w.stats.Payloads, 1)

	case eventTypeRejected:
		log.Warnf("Trace writer payload rejected by edge: %v", data.err)
		atomic.AddInt64(&w.stats.Errors, 1)

	case eventTypeDropped:
		w.easylog.Warn("Trace writer queue full. Payload dropped (%.2fKB).", float64(data.bytes)/1024)
		metrics.Count("datadog.trace_agent.trace_writer.dropped", 1, nil, 1)
		metrics.Count("datadog.trace_agent.trace_writer.dropped_bytes", int64(data.bytes), nil, 1)
	}
}

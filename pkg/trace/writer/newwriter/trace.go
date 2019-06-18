package writer

import (
	"compress/gzip"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/gogo/protobuf/proto"
)

// pathTraces is the target host API path for delivering traces.
const pathTraces = "/api/v0.2/traces"

// payloadFlushThreshold specifies the maximum accumulated payload size that is allowed before
// a flush is triggered; replaced in tests.
var payloadFlushThreshold = 3200000 // 3.2MB is the maximum allowed by the Datadog API

// SampledSpans represents the result of a trace sampling operation.
type SampledSpans struct {
	// Trace will contain a trace if it was sampled or be empty if it wasn't.
	Trace pb.Trace
	// Events contains all APM events extracted from a trace. If no events were extracted, it will be empty.
	Events []*pb.Span
}

// Empty returns true if this TracePackage has no data.
func (ss *SampledSpans) Empty() bool {
	return len(ss.Trace) == 0 && len(ss.Events) == 0
}

// size returns the estimated size of the package.
func (ss *SampledSpans) size() int {
	// we use msgpack's Msgsize() heuristic because it is a good indication
	// of the weight of a span and the msgpack size is relatively close to
	// the protobuf size, which is expensive to compute.
	return ss.Trace.Msgsize() + pb.Trace(ss.Events).Msgsize()
}

// TraceWriter buffers traces and APM events, flushing them to the Datadog API.
type TraceWriter struct {
	in       <-chan *SampledSpans
	hostname string
	env      string
	senders  []*sender
	stop     chan struct{}
	stats    *info.TraceWriterInfo

	traces       []*pb.APITrace // traces buffered
	events       []*pb.Span     // events buffered
	bufferedSize int            // estimated buffer size
}

// NewTraceWriter returns a new TraceWriter. It is created for the given agent configuration and
// will accept incoming spans via the in channel.
func NewTraceWriter(cfg *config.AgentConfig, in <-chan *SampledSpans) *TraceWriter {
	tw := &TraceWriter{
		in:       in,
		hostname: cfg.Hostname,
		env:      cfg.DefaultEnv,
		stats:    &info.TraceWriterInfo{},
		stop:     make(chan struct{}),
	}
	// allow 10% of the connection limit to outgoing sends.
	climit := int(math.Max(1, float64(cfg.ConnectionLimit)/10))
	tw.senders = newSenders(cfg, tw, pathTraces, climit)
	return tw
}

// Stop stops the TraceWriter and attempts to flush whatever is left in the senders buffers.
func (w *TraceWriter) Stop() {
	log.Debug("Exiting trace writer. Trying to flush whatever is left...")
	w.stop <- struct{}{}
	<-w.stop
	stopSenders(w.senders)
}

// Run starts the TraceWriter.
func (w *TraceWriter) Run() {
	log.Debug("Starting trace writer")
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	defer close(w.stop)
	for {
		select {
		case pkg := <-w.in:
			w.addSpans(pkg)
		case <-w.stop:
			// drain the input channel before stopping
		outer:
			for {
				select {
				case pkg := <-w.in:
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
	if pkg.Empty() {
		return
	}

	atomic.AddInt64(&w.stats.Spans, int64(len(pkg.Trace)))
	atomic.AddInt64(&w.stats.Traces, 1)
	atomic.AddInt64(&w.stats.Events, int64(len(pkg.Events)))

	size := pkg.size()
	if size+w.bufferedSize > payloadFlushThreshold {
		// reached maximum allowed buffered size
		w.flush()
	}
	if len(pkg.Trace) > 0 {
		w.traces = append(w.traces, traceutil.APITrace(pkg.Trace))
	}
	w.events = append(w.events, pkg.Events...)
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
	payload := pb.TracePayload{
		HostName:     w.hostname,
		Env:          w.env,
		Traces:       w.traces,
		Transactions: w.events,
	}
	b, err := proto.Marshal(&payload)
	if err != nil {
		log.Errorf("Failed to serialize payload, data dropped: %s", err)
		return
	}
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
	gzipw.Write(b)
	gzipw.Close()

	atomic.AddInt64(&w.stats.BytesUncompressed, int64(len(b)))
	atomic.AddInt64(&w.stats.BytesEstimated, int64(w.bufferedSize))

	for _, sender := range w.senders {
		sender.Push(p)
	}
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
	switch t {
	case eventTypeRetry, eventTypeSent, eventTypeFailed:
		metrics.Histogram("datadog.trace_agent.trace_writer.connection_fill", data.connectionFill, nil, 1)
	}
	switch t {
	case eventTypeRetry:
		log.Errorf("Retrying to flush trace payload; error: %s", data.err)
		atomic.AddInt64(&w.stats.Retries, 1)

	case eventTypeFlushed:
		log.Debugf("Flushed queue of %d trace payload(s) to the API in %s.", data.count, data.duration)
		timing.Since("datadog.trace_agent.trace_writer.flush_queue", time.Now().Add(-data.duration))

	case eventTypeSent:
		log.Tracef("Flushed traces to the API; time: %s, bytes: %d", data.duration, data.bytes)
		timing.Since("datadog.trace_agent.trace_writer.flush_duration", time.Now().Add(-data.duration))
		atomic.AddInt64(&w.stats.Bytes, int64(data.bytes))
		atomic.AddInt64(&w.stats.Payloads, 1)

	case eventTypeFailed:
		log.Errorf("Failed to flush traces (url:%s, size:%d bytes, error: %q)", data.host, data.bytes, data.err)
		atomic.AddInt64(&w.stats.Errors, 1)

	case eventTypeDropped:
		metrics.Count("datadog.trace_agent.trace_writer.dropped", 1, nil, 1)
		metrics.Count("datadog.trace_agent.trace_writer.dropped_bytes", int64(data.bytes), nil, 1)
	}
}

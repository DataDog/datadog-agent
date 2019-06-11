package writer

import (
	"bytes"
	"compress/gzip"
	"strings"
	"sync/atomic"
	"time"

	"github.com/StackVista/stackstate-agent/pkg/trace/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/info"
	"github.com/StackVista/stackstate-agent/pkg/trace/metrics"
	"github.com/StackVista/stackstate-agent/pkg/trace/pb"
	"github.com/StackVista/stackstate-agent/pkg/trace/traceutil"
	"github.com/StackVista/stackstate-agent/pkg/trace/watchdog"
	writerconfig "github.com/StackVista/stackstate-agent/pkg/trace/writer/config"
	log "github.com/cihub/seelog"
	"github.com/golang/protobuf/proto"
)

const pathTraces = "/api/v0.2/traces"

// TracePackage represents the result of a trace sampling operation.
//
// NOTE: A TracePackage can be valid even if any of its fields is nil/empty. In particular, a common case is that of
// empty Trace but non-empty Events. This happens when events are extracted from a trace that wasn't sampled.
type TracePackage struct {
	// Trace will contain a trace if it was sampled or be empty if it wasn't.
	Trace pb.Trace
	// Events contains all APMEvents extracted from a trace. If no events were extracted, it will be empty.
	Events []*pb.Span
}

// Empty returns true if this TracePackage has no data.
func (s *TracePackage) Empty() bool {
	return len(s.Trace) == 0 && len(s.Events) == 0
}

// TraceWriter ingests sampled traces and flushes them to the API.
type TraceWriter struct {
	stats    info.TraceWriterInfo
	hostName string
	env      string
	conf     writerconfig.TraceWriterConfig
	in       <-chan *TracePackage

	traces        []*pb.APITrace
	events        []*pb.Span
	spansInBuffer int

	sender payloadSender
	exit   chan struct{}
}

// NewTraceWriter returns a new writer for traces.
func NewTraceWriter(conf *config.AgentConfig, in <-chan *TracePackage) *TraceWriter {
	cfg := conf.TraceWriterConfig
	endpoints := newEndpoints(conf, pathTraces)
	sender := newMultiSender(endpoints, cfg.SenderConfig)
	log.Infof("Trace writer initializing with config: %+v", cfg)

	return &TraceWriter{
		conf:     cfg,
		hostName: conf.Hostname,
		env:      conf.DefaultEnv,

		traces: []*pb.APITrace{},
		events: []*pb.Span{},

		in: in,

		sender: sender,
		exit:   make(chan struct{}),
	}
}

// Start starts the writer.
func (w *TraceWriter) Start() {
	w.sender.Start()
	go func() {
		defer watchdog.LogOnPanic()
		w.Run()
	}()
}

// Run runs the main loop of the writer goroutine. It sends traces to the payload constructor, flushing it periodically
// and collects stats which are also reported periodically.
func (w *TraceWriter) Run() {
	defer close(w.exit)

	// for now, simply flush every x seconds
	flushTicker := time.NewTicker(w.conf.FlushPeriod)
	defer flushTicker.Stop()

	updateInfoTicker := time.NewTicker(w.conf.UpdateInfoPeriod)
	defer updateInfoTicker.Stop()

	// Monitor sender for events
	go func() {
		for event := range w.sender.Monitor() {
			switch event.typ {
			case eventTypeSuccess:
				log.Infof("flushed trace payload to the API, time:%s, size:%d bytes", event.stats.sendTime,
					len(event.payload.bytes))
				tags := []string{"url:" + event.stats.host}
				metrics.Gauge("datadog.trace_agent.trace_writer.flush_duration",
					event.stats.sendTime.Seconds(), tags, 1)
				atomic.AddInt64(&w.stats.Payloads, 1)
			case eventTypeFailure:
				log.Errorf("failed to flush trace payload, host:%s, time:%s, size:%d bytes, error: %s",
					event.stats.host, event.stats.sendTime, len(event.payload.bytes), event.err)
				atomic.AddInt64(&w.stats.Errors, 1)
			case eventTypeRetry:
				log.Errorf("retrying flush trace payload, retryNum: %d, delay:%s, error: %s",
					event.retryNum, event.retryDelay, event.err)
				atomic.AddInt64(&w.stats.Retries, 1)
			default:
				log.Debugf("don't know how to handle event with type %T", event)
			}
		}
	}()

	log.Debug("starting trace writer")

	for {
		select {
		case sampledTrace := <-w.in:
			w.handleSampledTrace(sampledTrace)
		case <-flushTicker.C:
			log.Debug("Flushing current traces")
			w.flush()
		case <-updateInfoTicker.C:
			go w.updateInfo()
		case <-w.exit:
			log.Info("exiting trace writer, flushing all remaining traces")
			w.flush()
			w.updateInfo()
			log.Info("Flushed. Exiting")
			return
		}
	}
}

// Stop stops the main Run loop.
func (w *TraceWriter) Stop() {
	w.exit <- struct{}{}
	<-w.exit
	w.sender.Stop()
}

func (w *TraceWriter) handleSampledTrace(sampledTrace *TracePackage) {
	if sampledTrace == nil || sampledTrace.Empty() {
		log.Debug("Ignoring empty sampled trace")
		return
	}

	trace := sampledTrace.Trace
	events := sampledTrace.Events

	n := len(trace) + len(events)

	if w.spansInBuffer > 0 && w.spansInBuffer+n > w.conf.MaxSpansPerPayload {
		// If we have data pending and adding the new data would overflow max spans per payload, force a flush
		w.flushDueToMaxSpansPerPayload()
	}

	w.appendTrace(sampledTrace.Trace)
	w.appendEvents(sampledTrace.Events)

	if n > w.conf.MaxSpansPerPayload {
		// If what we just added already goes over the limit, report this but lets carry on and flush
		atomic.AddInt64(&w.stats.SingleMaxSpans, 1)
		w.flushDueToMaxSpansPerPayload()
	}
}

func (w *TraceWriter) appendTrace(trace pb.Trace) {
	numSpans := len(trace)

	if numSpans == 0 {
		return
	}

	log.Tracef("Handling new trace with %d spans: %v", numSpans, trace)

	w.traces = append(w.traces, traceutil.APITrace(trace))
	w.spansInBuffer += numSpans
}

func (w *TraceWriter) appendEvents(events []*pb.Span) {
	for _, event := range events {
		log.Tracef("Handling new APM event: %v", event)
		w.events = append(w.events, event)
	}

	w.spansInBuffer += len(events)
}

func (w *TraceWriter) flushDueToMaxSpansPerPayload() {
	log.Debugf("Flushing because we reached max per payload")
	w.flush()
}

func (w *TraceWriter) flush() {
	numTraces := len(w.traces)
	numEvents := len(w.events)

	// If no traces, we can't construct anything
	if numTraces == 0 && numEvents == 0 {
		return
	}

	atomic.AddInt64(&w.stats.Traces, int64(numTraces))
	atomic.AddInt64(&w.stats.Events, int64(numEvents))
	atomic.AddInt64(&w.stats.Spans, int64(w.spansInBuffer))

	tracePayload := pb.TracePayload{
		HostName:     w.hostName,
		Env:          w.env,
		Traces:       w.traces,
		Transactions: w.events,
	}

	log.Debugf("Trace payload: %s", proto.MarshalTextString(&tracePayload))

	serialized, err := proto.Marshal(&tracePayload)
	if err != nil {
		log.Errorf("failed to serialize trace payload, data got dropped, err: %s", err)
		w.resetBuffer()
		return
	}

	encoding := "identity"

	// Try to compress payload before sending
	compressionBuffer := bytes.Buffer{}
	gz, err := gzip.NewWriterLevel(&compressionBuffer, gzip.BestSpeed)
	if err != nil {
		log.Errorf("failed to get compressor, sending uncompressed: %s", err)
	} else {
		_, err := gz.Write(serialized)
		gz.Close()

		if err != nil {
			log.Errorf("failed to compress payload, sending uncompressed: %s", err)
		} else {
			serialized = compressionBuffer.Bytes()
			encoding = "gzip"
		}
	}

	atomic.AddInt64(&w.stats.Bytes, int64(len(serialized)))

	headers := map[string]string{
		languageHeaderKey:  strings.Join(info.Languages(), "|"),
		"Content-Type":     "application/x-protobuf",
		"Content-Encoding": encoding,
	}

	payload := newPayload(serialized, headers)

	log.Debugf("flushing traces=%v events=%v", len(w.traces), len(w.events))
	w.sender.Send(payload)
	w.resetBuffer()
}

func (w *TraceWriter) resetBuffer() {
	// Reset traces
	w.traces = w.traces[:0]
	w.events = w.events[:0]
	w.spansInBuffer = 0
}

func (w *TraceWriter) updateInfo() {
	// TODO(gbbr): Scope these stats per endpoint (see (config.AgentConfig).AdditionalEndpoints))
	var twInfo info.TraceWriterInfo

	// Load counters and reset them for the next flush
	twInfo.Payloads = atomic.SwapInt64(&w.stats.Payloads, 0)
	twInfo.Traces = atomic.SwapInt64(&w.stats.Traces, 0)
	twInfo.Events = atomic.SwapInt64(&w.stats.Events, 0)
	twInfo.Spans = atomic.SwapInt64(&w.stats.Spans, 0)
	twInfo.Bytes = atomic.SwapInt64(&w.stats.Bytes, 0)
	twInfo.Retries = atomic.SwapInt64(&w.stats.Retries, 0)
	twInfo.Errors = atomic.SwapInt64(&w.stats.Errors, 0)
	twInfo.SingleMaxSpans = atomic.SwapInt64(&w.stats.SingleMaxSpans, 0)

	metrics.Count("datadog.trace_agent.trace_writer.payloads", int64(twInfo.Payloads), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.traces", int64(twInfo.Traces), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.events", int64(twInfo.Events), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.spans", int64(twInfo.Spans), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.bytes", int64(twInfo.Bytes), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.retries", int64(twInfo.Retries), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.errors", int64(twInfo.Errors), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.single_max_spans", int64(twInfo.SingleMaxSpans), nil, 1)

	info.UpdateTraceWriterInfo(twInfo)
}

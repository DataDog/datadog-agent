package writer

import (
	"bytes"
	"compress/gzip"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	writerconfig "github.com/DataDog/datadog-agent/pkg/trace/writer/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/golang/protobuf/proto"
)

const pathTraces = "/api/v0.2/traces"

// payloadFlushThreshold specifies the maximum accumulated payload size that is allowed before
// a flush is triggered; replaced in tests.
var payloadFlushThreshold = 3200000 // 3.2MB is the maximum allowed by the Datadog API

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
func (p *TracePackage) Empty() bool {
	return len(p.Trace) == 0 && len(p.Events) == 0
}

// size returns the estimated size of the package.
func (p *TracePackage) size() int {
	// we use msgpack's Msgsize() heuristic because it is a good indication
	// of the weight of a span and the msgpack size is relatively close to
	// the protobuf size, which is expensive to compute.
	return p.Trace.Msgsize() + pb.Trace(p.Events).Msgsize()
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
	bytesInBuffer int // estimated size of next payload

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
				log.Debugf("Flushed trace payload to the API, time:%s, size:%d bytes", event.stats.sendTime,
					len(event.payload.bytes))
				tags := []string{"url:" + event.stats.host}
				metrics.Gauge("datadog.trace_agent.trace_writer.flush_duration",
					event.stats.sendTime.Seconds(), tags, 1)
				atomic.AddInt64(&w.stats.Payloads, 1)
			case eventTypeFailure:
				log.Errorf("Failed to flush trace payload, host:%s, time:%s, size:%d bytes, error: %s",
					event.stats.host, event.stats.sendTime, len(event.payload.bytes), event.err)
				atomic.AddInt64(&w.stats.Errors, 1)
			case eventTypeRetry:
				log.Errorf("Retrying flush trace payload, retryNum: %d, delay:%s, error: %s",
					event.retryNum, event.retryDelay, event.err)
				atomic.AddInt64(&w.stats.Retries, 1)
			default:
				log.Debugf("Unable to handle event with type %T", event)
			}
		}
	}()

	log.Debug("Starting trace writer")

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
			log.Info("Exiting trace writer, flushing all remaining traces")
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

func (w *TraceWriter) handleSampledTrace(pkg *TracePackage) {
	if pkg == nil || pkg.Empty() {
		log.Debug("Ignoring empty sampled trace")
		return
	}

	size := pkg.size()

	if w.bytesInBuffer > 0 && w.bytesInBuffer+size > payloadFlushThreshold {
		// this new package would push us over, flush
		log.Debug("Flushing... Reached size threshold.")
		w.flush()
	}

	if len(pkg.Trace) > 0 {
		log.Tracef("Handling new trace with %d spans: %v", len(pkg.Trace), pkg.Trace)
		w.traces = append(w.traces, traceutil.APITrace(pkg.Trace))
	}
	if len(pkg.Events) > 0 {
		log.Tracef("Handling new APM events: %v", pkg.Events)
		w.events = append(w.events, pkg.Events...)
	}

	w.bytesInBuffer += size
	w.spansInBuffer += len(pkg.Trace) + len(pkg.Events)

	if size > payloadFlushThreshold {
		// we've added a single package that surpasses our threshold, we should count this occurrence,
		// it could be an indication of "over instrumentation" on the client side, where too many spans
		// could be present in traces.
		atomic.AddInt64(&w.stats.SingleMaxSize, 1)
		log.Debugf("Flushing... Surpassed size threshold with a single package sized %d bytes", size)
		w.flush()
	}
}

func (w *TraceWriter) flush() {
	numTraces := len(w.traces)
	numEvents := len(w.events)

	if numTraces == 0 && numEvents == 0 {
		// nothing to flush
		return
	}

	defer timing.Since("datadog.trace_agent.trace_writer.encode_ms", time.Now())

	atomic.AddInt64(&w.stats.Traces, int64(numTraces))
	atomic.AddInt64(&w.stats.Events, int64(numEvents))
	atomic.AddInt64(&w.stats.Spans, int64(w.spansInBuffer))

	tracePayload := pb.TracePayload{
		HostName:     w.hostName,
		Env:          w.env,
		Traces:       w.traces,
		Transactions: w.events,
	}

	serialized, err := proto.Marshal(&tracePayload)
	if err != nil {
		log.Errorf("Failed to serialize trace payload, data got dropped, err: %s", err)
		w.resetBuffer()
		return
	}

	atomic.AddInt64(&w.stats.BytesUncompressed, int64(len(serialized)))

	encoding := "identity"

	// Try to compress payload before sending
	compressionBuffer := bytes.Buffer{}
	gz, err := gzip.NewWriterLevel(&compressionBuffer, gzip.BestSpeed)
	if err != nil {
		log.Errorf("Failed to get compressor, sending uncompressed: %s", err)
	} else {
		_, err := gz.Write(serialized)
		gz.Close()

		if err != nil {
			log.Errorf("Failed to compress payload, sending uncompressed: %s", err)
		} else {
			serialized = compressionBuffer.Bytes()
			encoding = "gzip"
		}
	}

	atomic.AddInt64(&w.stats.Bytes, int64(len(serialized)))
	atomic.AddInt64(&w.stats.BytesEstimated, int64(w.bytesInBuffer))

	headers := map[string]string{
		languageHeaderKey:  strings.Join(info.Languages(), "|"),
		"Content-Type":     "application/x-protobuf",
		"Content-Encoding": encoding,
	}

	payload := newPayload(serialized, headers)

	log.Debugf("Flushing traces=%v events=%v size=%d estimated=%d", len(w.traces), len(w.events), len(serialized), w.bytesInBuffer)
	w.sender.Send(payload)
	w.resetBuffer()
}

func (w *TraceWriter) resetBuffer() {
	w.traces = w.traces[:0]
	w.events = w.events[:0]
	w.bytesInBuffer = 0
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
	twInfo.BytesEstimated = atomic.SwapInt64(&w.stats.BytesEstimated, 0)
	twInfo.BytesUncompressed = atomic.SwapInt64(&w.stats.BytesUncompressed, 0)
	twInfo.Retries = atomic.SwapInt64(&w.stats.Retries, 0)
	twInfo.Errors = atomic.SwapInt64(&w.stats.Errors, 0)
	twInfo.SingleMaxSize = atomic.SwapInt64(&w.stats.SingleMaxSize, 0)

	metrics.Count("datadog.trace_agent.trace_writer.payloads", int64(twInfo.Payloads), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.traces", int64(twInfo.Traces), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.events", int64(twInfo.Events), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.spans", int64(twInfo.Spans), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.bytes", int64(twInfo.Bytes), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.bytes_uncompressed", int64(twInfo.BytesUncompressed), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.bytes_estimated", int64(twInfo.BytesEstimated), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.retries", int64(twInfo.Retries), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.errors", int64(twInfo.Errors), nil, 1)
	metrics.Count("datadog.trace_agent.trace_writer.single_max_size", int64(twInfo.SingleMaxSize), nil, 1)

	info.UpdateTraceWriterInfo(twInfo)
}

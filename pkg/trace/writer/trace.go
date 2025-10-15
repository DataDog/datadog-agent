// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package writer

import (
	"errors"
	"strings"
	"sync"
	"time"

	compression "github.com/DataDog/datadog-agent/comp/trace/compression/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"

	"github.com/DataDog/datadog-go/v5/statsd"
)

// pathTraces is the target host API path for delivering traces.
const pathTraces = "/api/v0.2/traces"

const defaultConnectionLimit = 5

// MaxPayloadSize specifies the maximum accumulated payload size that is allowed before
// a flush is triggered; replaced in tests.
var MaxPayloadSize = 3200000 // 3.2MB is the maximum allowed by the Datadog API

type samplerTPSReader interface {
	GetTargetTPS() float64
}

type samplerEnabledReader interface {
	IsEnabled() bool
}

// SampledChunks represents the result of a trace sampling operation.
type SampledChunks struct {
	// TracerPayload contains all the chunks that were sampled as part of processing a payload.
	TracerPayload *pb.TracerPayload
	// Size represents the approximated message size in bytes (upper bound).
	Size int
	// SpanCount specifies the number of spans that were sampled as part of a trace inside the TracerPayload.
	SpanCount int64
	// EventCount specifies the total number of events found in Traces.
	EventCount int64
}

// TraceWriter implements TraceWriter interface, and buffers traces and APM events, flushing them to the Datadog API.
type TraceWriter struct {
	flushTicker *time.Ticker

	prioritySampler samplerTPSReader
	errorsSampler   samplerTPSReader
	rareSampler     samplerEnabledReader

	hostname        string
	env             string
	senders         []*sender
	stop            chan struct{}
	stats           *info.TraceWriterInfo
	statsLastMinute *info.TraceWriterInfo // aggregated stats over the last minute. Shared with info package
	wg              sync.WaitGroup        // waits flusher + reporter + compressor
	tick            time.Duration         // flush frequency
	agentVersion    string

	tracerPayloads []*pb.TracerPayload // tracer payloads buffered
	bufferedSize   int                 // estimated buffer size

	// syncMode reports whether the writer should flush on its own or only when FlushSync is called
	syncMode  bool
	flushChan chan chan struct{}

	telemetryCollector telemetry.TelemetryCollector

	easylog    *log.ThrottledLogger
	statsd     statsd.ClientInterface
	timing     timing.Reporter
	mu         sync.Mutex
	compressor compression.Component
}

// NewTraceWriter returns a new TraceWriter. It is created for the given agent configuration and
// will accept incoming spans via the in channel.
func NewTraceWriter(
	cfg *config.AgentConfig,
	prioritySampler samplerTPSReader,
	errorsSampler samplerTPSReader,
	rareSampler samplerEnabledReader,
	telemetryCollector telemetry.TelemetryCollector,
	statsd statsd.ClientInterface,
	timing timing.Reporter,
	compressor compression.Component) *TraceWriter {
	tw := &TraceWriter{
		prioritySampler:    prioritySampler,
		errorsSampler:      errorsSampler,
		rareSampler:        rareSampler,
		hostname:           cfg.Hostname,
		env:                cfg.DefaultEnv,
		stats:              &info.TraceWriterInfo{},
		statsLastMinute:    &info.TraceWriterInfo{},
		stop:               make(chan struct{}),
		flushChan:          make(chan chan struct{}),
		syncMode:           cfg.SynchronousFlushing,
		tick:               5 * time.Second,
		agentVersion:       cfg.AgentVersion,
		easylog:            log.NewThrottled(5, 10*time.Second), // no more than 5 messages every 10 seconds
		telemetryCollector: telemetryCollector,
		statsd:             statsd,
		timing:             timing,
		compressor:         compressor,
	}
	climit := cfg.TraceWriter.ConnectionLimit
	if climit == 0 {
		climit = defaultConnectionLimit
	}
	if cfg.TraceWriter.QueueSize > 0 {
		log.Warnf("apm_config.trace_writer.queue_size is deprecated and will not be respected.")
	}

	if s := cfg.TraceWriter.FlushPeriodSeconds; s != 0 {
		tw.tick = time.Duration(s*1000) * time.Millisecond
	}
	tw.flushTicker = time.NewTicker(tw.tick)

	qsize := 1
	log.Infof("Trace writer initialized (climt=%d qsize=%d compression=%s)", climit, qsize, compressor.Encoding())
	tw.senders = newSenders(cfg, tw, pathTraces, climit, qsize, telemetryCollector, statsd)
	tw.wg.Add(1)
	go tw.timeFlush()
	tw.wg.Add(1)
	go tw.reporter()
	return tw
}

// UpdateAPIKey updates the API Key, if needed, on Trace Writer senders.
func (w *TraceWriter) UpdateAPIKey(oldKey, newKey string) {
	for _, s := range w.senders {
		if oldKey == s.cfg.apiKey {
			log.Debugf("API Key updated for traces endpoint=%s", s.cfg.url)
			s.cfg.apiKey = newKey
		}
	}
}

func (w *TraceWriter) reporter() {
	tck := time.NewTicker(w.tick)
	info.UpdateTraceWriterInfo(w.statsLastMinute)
	var lastReset time.Time
	defer tck.Stop()
	defer w.wg.Done()
	for {
		select {
		case now := <-tck.C:
			if now.Sub(lastReset) >= time.Minute {
				w.statsLastMinute.Reset()
				lastReset = now
			}
			w.report()
		case <-w.stop:
			return
		}
	}
}

func (w *TraceWriter) timeFlush() {
	defer w.wg.Done()
	for {
		select {
		case <-w.flushTicker.C:
			func() {
				w.flush()
			}()
		case <-w.stop:
			return
		}
	}
}

// Stop stops the TraceWriter and attempts to flush whatever is left in the senders buffers.
func (w *TraceWriter) Stop() {
	log.Debug("Exiting trace writer. Trying to flush whatever is left...")
	close(w.stop)
	// Wait for encoding/compression to complete on each payload,
	// and submission to senders
	w.wg.Wait()
	w.flush()
	stopSenders(w.senders)
	w.flushTicker.Stop()
}

// FlushSync blocks and sends pending payloads when syncMode is true
func (w *TraceWriter) FlushSync() error {
	if !w.syncMode {
		return errors.New("not flushing; sync mode not enabled")
	}
	defer w.report()

	w.flush()
	return nil
}

// appendChunks adds sampled chunks to the current payload, and in the case the payload
// is full, returns a finished payload which needs to be written out.
func (w *TraceWriter) appendChunks(pkg *SampledChunks) []*pb.TracerPayload {
	var toflush []*pb.TracerPayload
	w.mu.Lock()
	defer w.mu.Unlock()
	size := pkg.Size
	if size+w.bufferedSize > MaxPayloadSize {
		// reached maximum allowed buffered size
		// reset the buffer so we can add our payload and defer a flush.
		toflush = w.tracerPayloads
		w.resetBuffer()
	}
	if len(pkg.TracerPayload.Chunks) > 0 {
		log.Tracef("Writer: handling new tracer payload with %d spans: %v", pkg.SpanCount, pkg.TracerPayload)
		w.tracerPayloads = append(w.tracerPayloads, pkg.TracerPayload)
	}
	w.bufferedSize += size
	return toflush
}

// WriteChunks serializes the provided chunks, enqueueing them to be sent
func (w *TraceWriter) WriteChunks(pkg *SampledChunks) {
	w.stats.Spans.Add(pkg.SpanCount)
	w.stats.Traces.Add(int64(len(pkg.TracerPayload.Chunks)))
	w.stats.Events.Add(pkg.EventCount)

	toflush := w.appendChunks(pkg)
	if toflush != nil {
		w.flushPayloads(toflush)
	}
}

func (w *TraceWriter) resetBuffer() {
	w.bufferedSize = 0
	w.tracerPayloads = make([]*pb.TracerPayload, 0, len(w.tracerPayloads))
}

const headerLanguages = "X-Datadog-Reported-Languages"

// w must be locked for a flush.
func (w *TraceWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	defer w.resetBuffer()
	w.flushPayloads(w.tracerPayloads)
}

// w does not need to be locked during flushPayloads.
func (w *TraceWriter) flushPayloads(payloads []*pb.TracerPayload) {
	w.flushTicker.Reset(w.tick) // reset the flush timer whenever we flush
	if len(payloads) == 0 {
		// nothing to do
		return
	}

	defer w.timing.Since("datadog.trace_agent.trace_writer.encode_ms", time.Now())

	log.Debugf("Serializing %d tracer payloads.", len(payloads))
	p := pb.AgentPayload{
		AgentVersion:       w.agentVersion,
		HostName:           w.hostname,
		Env:                w.env,
		TargetTPS:          w.prioritySampler.GetTargetTPS(),
		ErrorTPS:           w.errorsSampler.GetTargetTPS(),
		RareSamplerEnabled: w.rareSampler.IsEnabled(),
		TracerPayloads:     payloads,
	}
	log.Debugf("Reported agent rates: target_tps=%v errors_tps=%v rare_sampling=%v", p.TargetTPS, p.ErrorTPS, p.RareSamplerEnabled)

	w.serialize(&p)
}

var outPool = sync.Pool{}

func getBS(size int) []byte {
	b := outPool.Get()
	if b == nil {
		return make([]byte, size)
	}
	bs := b.([]byte)
	if cap(bs) < size {
		return make([]byte, size)
	}
	return bs[:size]
}

func (w *TraceWriter) serialize(pl *pb.AgentPayload) {
	b := getBS(pl.SizeVT())
	defer outPool.Put(b)
	n, err := pl.MarshalToSizedBufferVT(b)
	b = b[:n]
	if err != nil {
		log.Errorf("Failed to serialize payload, data dropped: %v", err)
		return
	}

	w.stats.BytesUncompressed.Add(int64(len(b)))
	p := newPayload(map[string]string{
		"Content-Type":     "application/x-protobuf",
		"Content-Encoding": w.compressor.Encoding(),
		headerLanguages:    strings.Join(info.Languages(), "|"),
	})
	p.body.Grow(len(b) / 2)
	writer, err := w.compressor.NewWriter(p.body)
	if err != nil {
		// it will never happen, unless an invalid compression is chosen;
		// we know gzip.BestSpeed is valid.
		log.Errorf("Failed to initialize %s writer. No traces can be sent: %v", w.compressor.Encoding(), err)
		return
	}
	if _, err := writer.Write(b); err != nil {
		log.Errorf("Error %s trace payload: %v", w.compressor.Encoding(), err)
	}
	if err := writer.Close(); err != nil {
		log.Errorf("Error closing %s stream when writing trace payload: %v", w.compressor.Encoding(), err)
	}
	sendPayloads(w.senders, p, w.syncMode)
}

func (w *TraceWriter) report() {
	// update aggregated stats before reseting them.
	w.statsLastMinute.Acc(w.stats)

	_ = w.statsd.Count("datadog.trace_agent.trace_writer.payloads", w.stats.Payloads.Swap(0), nil, 1)
	_ = w.statsd.Count("datadog.trace_agent.trace_writer.bytes_uncompressed", w.stats.BytesUncompressed.Swap(0), nil, 1)
	_ = w.statsd.Count("datadog.trace_agent.trace_writer.retries", w.stats.Retries.Swap(0), nil, 1)
	_ = w.statsd.Count("datadog.trace_agent.trace_writer.bytes", w.stats.Bytes.Swap(0), nil, 1)
	_ = w.statsd.Count("datadog.trace_agent.trace_writer.errors", w.stats.Errors.Swap(0), nil, 1)
	_ = w.statsd.Count("datadog.trace_agent.trace_writer.traces", w.stats.Traces.Swap(0), nil, 1)
	_ = w.statsd.Count("datadog.trace_agent.trace_writer.events", w.stats.Events.Swap(0), nil, 1)
	_ = w.statsd.Count("datadog.trace_agent.trace_writer.spans", w.stats.Spans.Swap(0), nil, 1)
}

var _ eventRecorder = (*TraceWriter)(nil)

// recordEvent implements eventRecorder.
func (w *TraceWriter) recordEvent(t eventType, data *eventData) {
	if data != nil {
		_ = w.statsd.Histogram("datadog.trace_agent.trace_writer.connection_fill", data.connectionFill, nil, 1)
		_ = w.statsd.Histogram("datadog.trace_agent.trace_writer.queue_fill", data.queueFill, nil, 1)
	}
	switch t {
	case eventTypeRetry:
		log.Debugf("Retrying to flush trace payload; error: %s", data.err)
		w.stats.Retries.Inc()

	case eventTypeSent:
		log.Debugf("Flushed traces to the API; time: %s, bytes: %d", data.duration, data.bytes)
		w.timing.Since("datadog.trace_agent.trace_writer.flush_duration", time.Now().Add(-data.duration))
		w.stats.Bytes.Add(int64(data.bytes))
		w.stats.Payloads.Inc()
		if !w.telemetryCollector.SentFirstTrace() {
			go w.telemetryCollector.SendFirstTrace()
		}

	case eventTypeRejected:
		log.Warnf("Trace writer payload rejected by edge: %v", data.err)
		w.stats.Errors.Inc()

	case eventTypeDropped:
		w.easylog.Warn("Trace Payload dropped (%.2fKB).", float64(data.bytes)/1024)
		_ = w.statsd.Count("datadog.trace_agent.trace_writer.dropped", 1, nil, 1)
		_ = w.statsd.Count("datadog.trace_agent.trace_writer.dropped_bytes", int64(data.bytes), nil, 1)
	}
}

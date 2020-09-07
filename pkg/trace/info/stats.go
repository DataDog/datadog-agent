// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package info

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ReceiverStats is used to store all the stats per tags.
type ReceiverStats struct {
	sync.RWMutex
	Stats map[Tags]*TagStats
}

// NewReceiverStats returns a new ReceiverStats
func NewReceiverStats() *ReceiverStats {
	return &ReceiverStats{sync.RWMutex{}, map[Tags]*TagStats{}}
}

// GetTagStats returns the struct in which the stats will be stored depending of their tags.
func (rs *ReceiverStats) GetTagStats(tags Tags) *TagStats {
	rs.Lock()
	tagStats, ok := rs.Stats[tags]
	if !ok {
		tagStats = newTagStats(tags)
		rs.Stats[tags] = tagStats
	}
	rs.Unlock()

	return tagStats
}

// Acc accumulates the stats from another ReceiverStats struct.
func (rs *ReceiverStats) Acc(recent *ReceiverStats) {
	recent.Lock()
	for _, tagStats := range recent.Stats {
		ts := rs.GetTagStats(tagStats.Tags)
		ts.update(&tagStats.Stats)
	}
	recent.Unlock()
}

// Publish updates stats about per-tag stats
func (rs *ReceiverStats) Publish() {
	rs.RLock()
	for _, tagStats := range rs.Stats {
		tagStats.publish()
	}
	rs.RUnlock()
}

// Languages returns the set of languages reporting traces to the Agent.
func (rs *ReceiverStats) Languages() []string {
	langSet := make(map[string]bool)
	langs := []string{}

	rs.RLock()
	for tags := range rs.Stats {
		if _, ok := langSet[tags.Lang]; !ok {
			langs = append(langs, tags.Lang)
			langSet[tags.Lang] = true
		}
	}
	rs.RUnlock()

	sort.Strings(langs)

	return langs
}

// Reset resets the ReceiverStats internal data
func (rs *ReceiverStats) Reset() {
	rs.Lock()
	for key, tagStats := range rs.Stats {
		// If a tagStats was empty, let's drop it.
		// That's a way to avoid over-time leaks.
		if tagStats.isEmpty() {
			delete(rs.Stats, key)
		}
		tagStats.reset()
	}
	rs.Unlock()
}

// LogStats logs one-line summaries of ReceiverStats. Problematic stats are logged as warnings.
func (rs *ReceiverStats) LogStats() {
	rs.RLock()
	defer rs.RUnlock()

	if len(rs.Stats) == 0 {
		log.Info("No data received")
		return
	}

	for _, ts := range rs.Stats {
		if !ts.isEmpty() {
			tags := ts.Tags.toArray()
			log.Infof("%v -> %s", tags, ts.infoString())
			warnString := ts.WarnString()
			if len(warnString) > 0 {
				log.Warnf("%v -> %s. Enable debug logging for more details.", tags, warnString)
			}
		}
	}
}

// TagStats is the struct used to associate the stats with their set of tags.
type TagStats struct {
	Tags
	Stats
}

func newTagStats(tags Tags) *TagStats {
	return &TagStats{tags, Stats{TracesDropped: &TracesDropped{}, SpansMalformed: &SpansMalformed{}}}
}

func (ts *TagStats) publish() {
	// Atomically load the stats from ts
	tracesReceived := atomic.LoadInt64(&ts.TracesReceived)
	tracesFiltered := atomic.LoadInt64(&ts.TracesFiltered)
	tracesPriorityNone := atomic.LoadInt64(&ts.TracesPriorityNone)
	tracesPriorityNeg := atomic.LoadInt64(&ts.TracesPriorityNeg)
	tracesPriority0 := atomic.LoadInt64(&ts.TracesPriority0)
	tracesPriority1 := atomic.LoadInt64(&ts.TracesPriority1)
	tracesPriority2 := atomic.LoadInt64(&ts.TracesPriority2)
	tracesBytes := atomic.LoadInt64(&ts.TracesBytes)
	spansReceived := atomic.LoadInt64(&ts.SpansReceived)
	spansDropped := atomic.LoadInt64(&ts.SpansDropped)
	spansFiltered := atomic.LoadInt64(&ts.SpansFiltered)
	eventsExtracted := atomic.LoadInt64(&ts.EventsExtracted)
	eventsSampled := atomic.LoadInt64(&ts.EventsSampled)
	requestsMade := atomic.LoadInt64(&ts.PayloadAccepted)
	requestsRejected := atomic.LoadInt64(&ts.PayloadRefused)

	// Publish the stats
	tags := ts.Tags.toArray()

	metrics.Count("datadog.trace_agent.receiver.trace", tracesReceived, tags, 1)
	metrics.Count("datadog.trace_agent.receiver.traces_received", tracesReceived, tags, 1)
	metrics.Count("datadog.trace_agent.receiver.traces_filtered", tracesFiltered, tags, 1)
	metrics.Count("datadog.trace_agent.receiver.traces_priority", tracesPriorityNone, append(tags, "priority:none"), 1)
	metrics.Count("datadog.trace_agent.receiver.traces_priority", tracesPriorityNeg, append(tags, "priority:neg"), 1)
	metrics.Count("datadog.trace_agent.receiver.traces_priority", tracesPriority0, append(tags, "priority:0"), 1)
	metrics.Count("datadog.trace_agent.receiver.traces_priority", tracesPriority1, append(tags, "priority:1"), 1)
	metrics.Count("datadog.trace_agent.receiver.traces_priority", tracesPriority2, append(tags, "priority:2"), 1)
	metrics.Count("datadog.trace_agent.receiver.traces_bytes", tracesBytes, tags, 1)
	metrics.Count("datadog.trace_agent.receiver.spans_received", spansReceived, tags, 1)
	metrics.Count("datadog.trace_agent.receiver.spans_dropped", spansDropped, tags, 1)
	metrics.Count("datadog.trace_agent.receiver.spans_filtered", spansFiltered, tags, 1)
	metrics.Count("datadog.trace_agent.receiver.events_extracted", eventsExtracted, tags, 1)
	metrics.Count("datadog.trace_agent.receiver.events_sampled", eventsSampled, tags, 1)
	metrics.Count("datadog.trace_agent.receiver.payload_accepted", requestsMade, tags, 1)
	metrics.Count("datadog.trace_agent.receiver.payload_refused", requestsRejected, tags, 1)

	for reason, count := range ts.TracesDropped.tagValues() {
		metrics.Count("datadog.trace_agent.normalizer.traces_dropped", count, append(tags, "reason:"+reason), 1)
	}
	for reason, count := range ts.SpansMalformed.tagValues() {
		metrics.Count("datadog.trace_agent.normalizer.spans_malformed", count, append(tags, "reason:"+reason), 1)
	}
}

// mapToString serializes the entries in this map into format "key1: value1, key2: value2, ...", sorted by
// key to ensure consistent output order. Only non-zero values are included.
func mapToString(m map[string]int64) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var results []string
	for _, key := range keys {
		value := m[key]
		if value > 0 {
			results = append(results, fmt.Sprintf("%s:%d", key, value))
		}
	}
	return strings.Join(results, ", ")
}

// TracesDropped contains counts for reasons traces have been dropped
type TracesDropped struct {
	// DecodingError is when the agent fails to decode a trace payload
	DecodingError int64
	// PayloadTooLarge specifies the number of traces dropped due to the payload
	// being too large to be accepted.
	PayloadTooLarge int64
	// EmptyTrace is when the trace contains no spans
	EmptyTrace int64
	// TraceIDZero is when any spans in a trace have TraceId=0
	TraceIDZero int64
	// SpanIDZero is when any span has SpanId=0
	SpanIDZero int64
	// ForeignSpan is when a span in a trace has a TraceId that is different than the first span in the trace
	ForeignSpan int64
	// Timeout is when a request times out.
	Timeout int64
	// EOF is when an unexpected EOF is encountered, this can happen because the client has aborted
	// or because a bad payload (i.e. shorter than claimed in Content-Length) was sent.
	EOF int64
}

// tagValues converts TracesDropped into a map representation with keys matching standardized names for all reasons
func (s *TracesDropped) tagValues() map[string]int64 {
	return map[string]int64{
		"payload_too_large": atomic.LoadInt64(&s.PayloadTooLarge),
		"decoding_error":    atomic.LoadInt64(&s.DecodingError),
		"empty_trace":       atomic.LoadInt64(&s.EmptyTrace),
		"trace_id_zero":     atomic.LoadInt64(&s.TraceIDZero),
		"span_id_zero":      atomic.LoadInt64(&s.SpanIDZero),
		"foreign_span":      atomic.LoadInt64(&s.ForeignSpan),
		"timeout":           atomic.LoadInt64(&s.Timeout),
		"unexpected_eof":    atomic.LoadInt64(&s.EOF),
	}
}

func (s *TracesDropped) String() string {
	return mapToString(s.tagValues())
}

// SpansMalformed contains counts for reasons malformed spans have been accepted after applying automatic fixes
type SpansMalformed struct {
	// DuplicateSpanID is when one or more spans in a trace have the same SpanId
	DuplicateSpanID int64
	// ServiceEmpty is when a span has an empty Service field
	ServiceEmpty int64
	// ServiceTruncate is when a span's Service is truncated for exceeding the max length
	ServiceTruncate int64
	// ServiceInvalid is when a span's Service doesn't conform to Datadog tag naming standards
	ServiceInvalid int64
	// SpanNameEmpty is when a span's Name is empty
	SpanNameEmpty int64
	// SpanNameTruncate is when a span's Name is truncated for exceeding the max length
	SpanNameTruncate int64
	// SpanNameInvalid is when a span's Name doesn't conform to Datadog tag naming standards
	SpanNameInvalid int64
	// ResourceEmpty is when a span's Resource is empty
	ResourceEmpty int64
	// TypeTruncate is when a span's Type is truncated for exceeding the max length
	TypeTruncate int64
	// InvalidStartDate is when a span's Start date is invalid
	InvalidStartDate int64
	// InvalidDuration is when a span's Duration is invalid
	InvalidDuration int64
	// InvalidHTTPStatusCode is when a span's metadata contains an invalid http status code
	InvalidHTTPStatusCode int64
}

// tagValues converts SpansMalformed into a map representation with keys matching standardized names for all reasons
func (s *SpansMalformed) tagValues() map[string]int64 {
	return map[string]int64{
		"duplicate_span_id":        atomic.LoadInt64(&s.DuplicateSpanID),
		"service_empty":            atomic.LoadInt64(&s.ServiceEmpty),
		"service_truncate":         atomic.LoadInt64(&s.ServiceTruncate),
		"service_invalid":          atomic.LoadInt64(&s.ServiceInvalid),
		"span_name_empty":          atomic.LoadInt64(&s.SpanNameEmpty),
		"span_name_truncate":       atomic.LoadInt64(&s.SpanNameTruncate),
		"span_name_invalid":        atomic.LoadInt64(&s.SpanNameInvalid),
		"resource_empty":           atomic.LoadInt64(&s.ResourceEmpty),
		"type_truncate":            atomic.LoadInt64(&s.TypeTruncate),
		"invalid_start_date":       atomic.LoadInt64(&s.InvalidStartDate),
		"invalid_duration":         atomic.LoadInt64(&s.InvalidDuration),
		"invalid_http_status_code": atomic.LoadInt64(&s.InvalidHTTPStatusCode),
	}
}

func (s *SpansMalformed) String() string {
	return mapToString(s.tagValues())
}

// Stats holds the metrics that will be reported every 10s by the agent.
// Its fields require to be accessed in an atomic way.
type Stats struct {
	// TracesReceived is the total number of traces received, including the dropped ones.
	TracesReceived int64
	// TracesDropped contains stats about the count of dropped traces by reason
	TracesDropped *TracesDropped
	// SpansMalformed contains stats about the count of malformed traces by reason
	SpansMalformed *SpansMalformed
	// TracesFiltered is the number of traces filtered.
	TracesFiltered int64
	// TracesPriorityNone is the number of traces with no sampling priority.
	TracesPriorityNone int64
	// TracesPriorityNeg is the number of traces with a negative sampling priority.
	TracesPriorityNeg int64
	// TracesPriority0 is the number of traces with sampling priority set to zero.
	TracesPriority0 int64
	// TracesPriority1 is the number of traces with sampling priority automatically set to 1.
	TracesPriority1 int64
	// TracesPriority2 is the number of traces with sampling priority manually set to 2 or more.
	TracesPriority2 int64
	// TracesBytes is the amount of data received on the traces endpoint (raw data, encoded, compressed).
	TracesBytes int64
	// SpansReceived is the total number of spans received, including the dropped ones.
	SpansReceived int64
	// SpansDropped is the number of spans dropped.
	SpansDropped int64
	// SpansFiltered is the number of spans filtered.
	SpansFiltered int64
	// EventsExtracted is the total number of APM events extracted from traces.
	EventsExtracted int64
	// EventsSampled is the total number of APM events sampled.
	EventsSampled int64
	// PayloadAccepted counts the number of payloads that have been accepted by the HTTP handler.
	PayloadAccepted int64
	// PayloadRefused counts the number of payloads that have been rejected by the rate limiter.
	PayloadRefused int64
}

func (s *Stats) update(recent *Stats) {
	atomic.AddInt64(&s.TracesReceived, atomic.LoadInt64(&recent.TracesReceived))

	atomic.AddInt64(&s.TracesDropped.DecodingError, atomic.LoadInt64(&recent.TracesDropped.DecodingError))
	atomic.AddInt64(&s.TracesDropped.EmptyTrace, atomic.LoadInt64(&recent.TracesDropped.EmptyTrace))
	atomic.AddInt64(&s.TracesDropped.TraceIDZero, atomic.LoadInt64(&recent.TracesDropped.TraceIDZero))
	atomic.AddInt64(&s.TracesDropped.SpanIDZero, atomic.LoadInt64(&recent.TracesDropped.SpanIDZero))
	atomic.AddInt64(&s.TracesDropped.ForeignSpan, atomic.LoadInt64(&recent.TracesDropped.ForeignSpan))
	atomic.AddInt64(&s.SpansMalformed.DuplicateSpanID, atomic.LoadInt64(&recent.SpansMalformed.DuplicateSpanID))
	atomic.AddInt64(&s.SpansMalformed.ServiceEmpty, atomic.LoadInt64(&recent.SpansMalformed.ServiceEmpty))
	atomic.AddInt64(&s.SpansMalformed.ServiceTruncate, atomic.LoadInt64(&recent.SpansMalformed.ServiceTruncate))
	atomic.AddInt64(&s.SpansMalformed.ServiceInvalid, atomic.LoadInt64(&recent.SpansMalformed.ServiceInvalid))
	atomic.AddInt64(&s.SpansMalformed.SpanNameEmpty, atomic.LoadInt64(&recent.SpansMalformed.SpanNameEmpty))
	atomic.AddInt64(&s.SpansMalformed.SpanNameTruncate, atomic.LoadInt64(&recent.SpansMalformed.SpanNameTruncate))
	atomic.AddInt64(&s.SpansMalformed.SpanNameInvalid, atomic.LoadInt64(&recent.SpansMalformed.SpanNameInvalid))
	atomic.AddInt64(&s.SpansMalformed.ResourceEmpty, atomic.LoadInt64(&recent.SpansMalformed.ResourceEmpty))
	atomic.AddInt64(&s.SpansMalformed.TypeTruncate, atomic.LoadInt64(&recent.SpansMalformed.TypeTruncate))
	atomic.AddInt64(&s.SpansMalformed.InvalidStartDate, atomic.LoadInt64(&recent.SpansMalformed.InvalidStartDate))
	atomic.AddInt64(&s.SpansMalformed.InvalidDuration, atomic.LoadInt64(&recent.SpansMalformed.InvalidDuration))
	atomic.AddInt64(&s.SpansMalformed.InvalidHTTPStatusCode, atomic.LoadInt64(&recent.SpansMalformed.InvalidHTTPStatusCode))

	atomic.AddInt64(&s.TracesFiltered, atomic.LoadInt64(&recent.TracesFiltered))
	atomic.AddInt64(&s.TracesPriorityNone, atomic.LoadInt64(&recent.TracesPriorityNone))
	atomic.AddInt64(&s.TracesPriorityNeg, atomic.LoadInt64(&recent.TracesPriorityNeg))
	atomic.AddInt64(&s.TracesPriority0, atomic.LoadInt64(&recent.TracesPriority0))
	atomic.AddInt64(&s.TracesPriority1, atomic.LoadInt64(&recent.TracesPriority1))
	atomic.AddInt64(&s.TracesPriority2, atomic.LoadInt64(&recent.TracesPriority2))
	atomic.AddInt64(&s.TracesBytes, atomic.LoadInt64(&recent.TracesBytes))
	atomic.AddInt64(&s.SpansReceived, atomic.LoadInt64(&recent.SpansReceived))
	atomic.AddInt64(&s.SpansDropped, atomic.LoadInt64(&recent.SpansDropped))
	atomic.AddInt64(&s.SpansFiltered, atomic.LoadInt64(&recent.SpansFiltered))
	atomic.AddInt64(&s.EventsExtracted, atomic.LoadInt64(&recent.EventsExtracted))
	atomic.AddInt64(&s.EventsSampled, atomic.LoadInt64(&recent.EventsSampled))
	atomic.AddInt64(&s.PayloadAccepted, atomic.LoadInt64(&recent.PayloadAccepted))
	atomic.AddInt64(&s.PayloadRefused, atomic.LoadInt64(&recent.PayloadRefused))
}

func (s *Stats) reset() {
	atomic.StoreInt64(&s.TracesReceived, 0)
	atomic.StoreInt64(&s.TracesDropped.PayloadTooLarge, 0)
	atomic.StoreInt64(&s.TracesDropped.DecodingError, 0)
	atomic.StoreInt64(&s.TracesDropped.EmptyTrace, 0)
	atomic.StoreInt64(&s.TracesDropped.TraceIDZero, 0)
	atomic.StoreInt64(&s.TracesDropped.SpanIDZero, 0)
	atomic.StoreInt64(&s.TracesDropped.ForeignSpan, 0)
	atomic.StoreInt64(&s.TracesDropped.Timeout, 0)
	atomic.StoreInt64(&s.TracesDropped.EOF, 0)
	atomic.StoreInt64(&s.SpansMalformed.DuplicateSpanID, 0)
	atomic.StoreInt64(&s.SpansMalformed.ServiceEmpty, 0)
	atomic.StoreInt64(&s.SpansMalformed.ServiceTruncate, 0)
	atomic.StoreInt64(&s.SpansMalformed.ServiceInvalid, 0)
	atomic.StoreInt64(&s.SpansMalformed.SpanNameEmpty, 0)
	atomic.StoreInt64(&s.SpansMalformed.SpanNameTruncate, 0)
	atomic.StoreInt64(&s.SpansMalformed.SpanNameInvalid, 0)
	atomic.StoreInt64(&s.SpansMalformed.ResourceEmpty, 0)
	atomic.StoreInt64(&s.SpansMalformed.TypeTruncate, 0)
	atomic.StoreInt64(&s.SpansMalformed.InvalidStartDate, 0)
	atomic.StoreInt64(&s.SpansMalformed.InvalidDuration, 0)
	atomic.StoreInt64(&s.SpansMalformed.InvalidHTTPStatusCode, 0)
	atomic.StoreInt64(&s.TracesFiltered, 0)
	atomic.StoreInt64(&s.TracesPriorityNone, 0)
	atomic.StoreInt64(&s.TracesPriorityNeg, 0)
	atomic.StoreInt64(&s.TracesPriority0, 0)
	atomic.StoreInt64(&s.TracesPriority1, 0)
	atomic.StoreInt64(&s.TracesPriority2, 0)
	atomic.StoreInt64(&s.TracesBytes, 0)
	atomic.StoreInt64(&s.SpansReceived, 0)
	atomic.StoreInt64(&s.SpansDropped, 0)
	atomic.StoreInt64(&s.SpansFiltered, 0)
	atomic.StoreInt64(&s.EventsExtracted, 0)
	atomic.StoreInt64(&s.EventsSampled, 0)
	atomic.StoreInt64(&s.PayloadAccepted, 0)
	atomic.StoreInt64(&s.PayloadRefused, 0)
}

func (s *Stats) isEmpty() bool {
	tracesBytes := atomic.LoadInt64(&s.TracesBytes)

	return tracesBytes == 0
}

// infoString returns a string representation of the Stats struct containing standard operational stats (not problems)
func (s *Stats) infoString() string {
	// Atomically load the stats
	tracesReceived := atomic.LoadInt64(&s.TracesReceived)
	tracesFiltered := atomic.LoadInt64(&s.TracesFiltered)
	// Omitting priority information, use expvar or metrics for debugging purpose
	tracesBytes := atomic.LoadInt64(&s.TracesBytes)
	eventsExtracted := atomic.LoadInt64(&s.EventsExtracted)
	eventsSampled := atomic.LoadInt64(&s.EventsSampled)

	return fmt.Sprintf("traces received: %d, traces filtered: %d, "+
		"traces amount: %d bytes, events extracted: %d, events sampled: %d",
		tracesReceived, tracesFiltered, tracesBytes, eventsExtracted, eventsSampled)
}

// WarnString returns a string representation of the Stats struct containing only issues which we should be warning on
// if there are no issues then an empty string is returned
func (ts *TagStats) WarnString() string {
	var (
		w []string
		d string
	)
	if ts.TracesDropped != nil {
		d = ts.TracesDropped.String()
	}
	if len(d) > 0 {
		w = append(w, fmt.Sprintf("traces_dropped(%s)", d))
	}
	var m string
	if ts.SpansMalformed != nil {
		m = ts.SpansMalformed.String()
	}
	if len(m) > 0 {
		w = append(w, fmt.Sprintf("spans_malformed(%s)", m))
	}
	return strings.Join(w, ", ")
}

// Tags holds the tags we parse when we handle the header of the payload.
type Tags struct {
	Lang, LangVersion, LangVendor, Interpreter, TracerVersion string
	EndpointVersion                                           string
}

// toArray will transform the Tags struct into a slice of string.
// We only publish the non-empty tags.
func (t *Tags) toArray() []string {
	tags := make([]string, 0, 5)

	if t.Lang != "" {
		tags = append(tags, "lang:"+t.Lang)
	}
	if t.LangVersion != "" {
		tags = append(tags, "lang_version:"+t.LangVersion)
	}
	if t.LangVendor != "" {
		tags = append(tags, "lang_vendor:"+t.LangVendor)
	}
	if t.Interpreter != "" {
		tags = append(tags, "interpreter:"+t.Interpreter)
	}
	if t.TracerVersion != "" {
		tags = append(tags, "tracer_version:"+t.TracerVersion)
	}
	if t.EndpointVersion != "" {
		tags = append(tags, "endpoint_version:"+t.EndpointVersion)
	}

	return tags
}

package info

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
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

// Strings gives a multi strings representation of the ReceiverStats struct.
func (rs *ReceiverStats) Strings() []string {
	rs.RLock()
	defer rs.RUnlock()

	if len(rs.Stats) == 0 {
		return []string{"no data received"}
	}

	strings := make([]string, 0, len(rs.Stats))

	for _, ts := range rs.Stats {
		if !ts.isEmpty() {
			strings = append(strings, fmt.Sprintf("%v -> %s", ts.Tags.toArray(), ts.String()))
		}
	}
	return strings
}

// TagStats is the struct used to associate the stats with their set of tags.
type TagStats struct {
	Tags
	Stats
}

func newTagStats(tags Tags) *TagStats {
	return &TagStats{tags, Stats{}}
}

type NormalizationIssue struct {
	Reason string
	Count int64
}

func (ts *TagStats) DroppedTraceNormalizationIssues() []NormalizationIssue {
	return []NormalizationIssue{
		{"decoding_error", atomic.LoadInt64(&ts.TracesDroppedDecodingError)},
		{"empty_trace", atomic.LoadInt64(&ts.TracesDroppedEmptyTrace)},
		{"trace_id_zero", atomic.LoadInt64(&ts.TracesDroppedTraceIdZero)},
		{"span_id_zero", atomic.LoadInt64(&ts.TracesDroppedSpanIdZero)},
		{"foreign_span", atomic.LoadInt64(&ts.TracesDroppedForeignSpan)},
	}
}

func (ts *TagStats) MalformedTraceNormalizationIssues() []NormalizationIssue {
	return []NormalizationIssue{
		{"duplicate_span_id", atomic.LoadInt64(&ts.TracesMalformedDuplicateSpanId)},
		{"service_empty", atomic.LoadInt64(&ts.TracesMalformedServiceEmpty)},
		{"service_truncate", atomic.LoadInt64(&ts.TracesMalformedServiceTruncate)},
		{"service_invalid", atomic.LoadInt64(&ts.TracesMalformedServiceInvalid)},
		{"span_name_empty", atomic.LoadInt64(&ts.TracesMalformedSpanNameEmpty)},
		{"span_name_truncate", atomic.LoadInt64(&ts.TracesMalformedSpanNameTruncate)},
		{"span_name_invalid", atomic.LoadInt64(&ts.TracesMalformedSpanNameInvalid)},
		{"resource_empty", atomic.LoadInt64(&ts.TracesMalformedResourceEmpty)},
		{"type_truncate", atomic.LoadInt64(&ts.TracesMalformedTypeTruncate)},
		{"invalid_start_date", atomic.LoadInt64(&ts.TracesMalformedInvalidStartDate)},
		{"invalid_duration", atomic.LoadInt64(&ts.TracesMalformedInvalidDuration)},
		{"invalid_http_status_code", atomic.LoadInt64(&ts.TracesMalformedInvalidHttpStatusCode)},
	}
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
	servicesReceived := atomic.LoadInt64(&ts.ServicesReceived)
	servicesBytes := atomic.LoadInt64(&ts.ServicesBytes)
	eventsExtracted := atomic.LoadInt64(&ts.EventsExtracted)
	eventsSampled := atomic.LoadInt64(&ts.EventsSampled)
	requestsMade := atomic.LoadInt64(&ts.PayloadAccepted)

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
	metrics.Count("datadog.trace_agent.receiver.services_received", servicesReceived, tags, 1)
	metrics.Count("datadog.trace_agent.receiver.services_bytes", servicesBytes, tags, 1)
	metrics.Count("datadog.trace_agent.receiver.events_extracted", eventsExtracted, tags, 1)
	metrics.Count("datadog.trace_agent.receiver.events_sampled", eventsSampled, tags, 1)
	metrics.Count("datadog.trace_agent.receiver.payload_accepted", requestsMade, tags, 1)


	var droppedReasons []string
	for _, c := range ts.DroppedTraceNormalizationIssues() {
		if c.Count > 0 {
			metrics.Count("datadog.trace_agent.normalizer.traces_dropped", c.Count, append(tags, "reason:" + c.Reason), 1)
			droppedReasons = append(droppedReasons, c.Reason + ":" + strconv.FormatInt(c.Count, 10))
		}
	}

	var malformedReasons []string
	for _, c := range ts.MalformedTraceNormalizationIssues() {
		if c.Count > 0 {
			metrics.Count("datadog.trace_agent.normalizer.traces_malformed", c.Count, append(tags, "reason:" + c.Reason), 1)
			malformedReasons = append(malformedReasons, c.Reason + ":" + strconv.FormatInt(c.Count, 10))
		}
	}

	var normalizerMessages []string
	if len(droppedReasons) > 0 {
		normalizerMessages = append(normalizerMessages, fmt.Sprintf("dropped_traces(%s)", strings.Join(droppedReasons, ", ")))
	}
	if len(malformedReasons) > 0 {
		normalizerMessages = append(normalizerMessages, fmt.Sprintf("malformed_traces(%s)", strings.Join(malformedReasons, ", ")))
	}

	if len(normalizerMessages) > 0 {
		log.Warn("received invalid traces (enable debug logging for more details): %s", strings.Join(normalizerMessages, " "))
	}
}

// Stats holds the metrics that will be reported every 10s by the agent.
// Its fields require to be accessed in an atomic way.
type Stats struct {
	// TracesReceived is the total number of traces received, including the dropped ones.
	TracesReceived int64
	// TracesDroppedDecodingError is the number of traces dropped due to decoding error
	TracesDroppedDecodingError int64
	// TracesDroppedEmptyTrace is the number of traces dropped due to empty trace
	TracesDroppedEmptyTrace int64
	// TracesDropped is the number of traces dropped due to trace-ID=zero
	TracesDroppedTraceIdZero int64
	// TracesDropped is the number of traces dropped.
	TracesDroppedSpanIdZero int64
	// TracesDropped is the number of traces dropped.
	TracesDroppedForeignSpan int64
	// TracesMalformedDuplicateSpanId is the number of traces dropped.
	TracesMalformedDuplicateSpanId int64
	TracesMalformedServiceEmpty int64
	TracesMalformedServiceTruncate int64
	TracesMalformedServiceInvalid int64
	TracesMalformedSpanNameEmpty int64
	TracesMalformedSpanNameTruncate int64
	TracesMalformedSpanNameInvalid int64
	TracesMalformedResourceEmpty int64
	TracesMalformedTypeTruncate int64
	TracesMalformedInvalidStartDate int64
	TracesMalformedInvalidDuration int64
	TracesMalformedInvalidHttpStatusCode int64

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
	// ServicesReceived is the number of services received.
	ServicesReceived int64
	// ServicesBytes is the amount of data received on the services endpoint (raw data, encoded, compressed).
	ServicesBytes int64
	// EventsExtracted is the total number of APM events extracted from traces.
	EventsExtracted int64
	// EventsSampled is the total number of APM events sampled.
	EventsSampled int64
	// PayloadAccepted counts the number of payloads that have been accepted by the HTTP handler.
	PayloadAccepted int64
}

func (s *Stats) update(recent *Stats) {
	atomic.AddInt64(&s.TracesReceived, atomic.LoadInt64(&recent.TracesReceived))
	atomic.AddInt64(&s.TracesDroppedDecodingError, atomic.LoadInt64(&recent.TracesDroppedDecodingError))
	atomic.AddInt64(&s.TracesDroppedEmptyTrace, atomic.LoadInt64(&recent.TracesDroppedEmptyTrace))
	atomic.AddInt64(&s.TracesDroppedTraceIdZero, atomic.LoadInt64(&recent.TracesDroppedTraceIdZero))
	atomic.AddInt64(&s.TracesDroppedSpanIdZero, atomic.LoadInt64(&recent.TracesDroppedSpanIdZero))
	atomic.AddInt64(&s.TracesDroppedForeignSpan, atomic.LoadInt64(&recent.TracesDroppedForeignSpan))
	atomic.AddInt64(&s.TracesMalformedDuplicateSpanId, atomic.LoadInt64(&recent.TracesMalformedDuplicateSpanId))
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
	atomic.AddInt64(&s.ServicesReceived, atomic.LoadInt64(&recent.ServicesReceived))
	atomic.AddInt64(&s.ServicesBytes, atomic.LoadInt64(&recent.ServicesBytes))
	atomic.AddInt64(&s.EventsExtracted, atomic.LoadInt64(&recent.EventsExtracted))
	atomic.AddInt64(&s.EventsSampled, atomic.LoadInt64(&recent.EventsSampled))
	atomic.AddInt64(&s.PayloadAccepted, atomic.LoadInt64(&recent.PayloadAccepted))
}

func (s *Stats) reset() {
	atomic.StoreInt64(&s.TracesReceived, 0)
	atomic.StoreInt64(&s.TracesDroppedDecodingError, 0)
	atomic.StoreInt64(&s.TracesDroppedEmptyTrace, 0)
	atomic.StoreInt64(&s.TracesDroppedTraceIdZero, 0)
	atomic.StoreInt64(&s.TracesDroppedSpanIdZero, 0)
	atomic.StoreInt64(&s.TracesDroppedForeignSpan, 0)
	atomic.StoreInt64(&s.TracesMalformedDuplicateSpanId, 0)
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
	atomic.StoreInt64(&s.ServicesReceived, 0)
	atomic.StoreInt64(&s.ServicesBytes, 0)
	atomic.StoreInt64(&s.EventsExtracted, 0)
	atomic.StoreInt64(&s.EventsSampled, 0)
	atomic.StoreInt64(&s.PayloadAccepted, 0)
}

func (s *Stats) isEmpty() bool {
	tracesBytes := atomic.LoadInt64(&s.TracesBytes)

	return tracesBytes == 0
}

// String returns a string representation of the Stats struct
func (s *Stats) String() string {
	// TODO: just marshal this whole thing to JSON
	// Atomically load the stats
	tracesReceived := atomic.LoadInt64(&s.TracesReceived)
	//tracesDropped := atomic.LoadInt64(&s.TracesDropped)
	tracesFiltered := atomic.LoadInt64(&s.TracesFiltered)
	// Omitting priority information, use expvar or metrics for debugging purpose
	tracesBytes := atomic.LoadInt64(&s.TracesBytes)
	servicesReceived := atomic.LoadInt64(&s.ServicesReceived)
	servicesBytes := atomic.LoadInt64(&s.ServicesBytes)
	eventsExtracted := atomic.LoadInt64(&s.EventsExtracted)
	eventsSampled := atomic.LoadInt64(&s.EventsSampled)

	return fmt.Sprintf("traces received: %d, traces dropped: %d, traces filtered: %d, "+
		"traces amount: %d bytes, services received: %d, services amount: %d bytes, "+
		"events extracted: %d, events sampled: %d",
		tracesReceived, tracesFiltered,
		tracesBytes, servicesReceived, servicesBytes,
		eventsExtracted, eventsSampled)
}

// Tags holds the tags we parse when we handle the header of the payload.
type Tags struct {
	Lang, LangVersion, Interpreter, TracerVersion string
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
	if t.Interpreter != "" {
		tags = append(tags, "interpreter:"+t.Interpreter)
	}
	if t.TracerVersion != "" {
		tags = append(tags, "tracer_version:"+t.TracerVersion)
	}

	return tags
}

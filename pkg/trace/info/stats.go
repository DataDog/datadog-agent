package info

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"sort"
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
			log.Infof("%v -> %s", tags, ts.InfoString())
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
	return &TagStats{tags, Stats{}}
}

// AllNormalizationIssues returns a map of counts of all normalization issue reasons due to dropped or malformed traces
func (ts *TagStats) AllNormalizationIssues() map[string]int64 {
	m := ts.TracesDropped.tagValues()
	for r, c := range ts.TracesMalformed.tagValues() {
		m[r] = c
	}
	return m
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

	for reason, count := range ts.TracesDropped.tagValues() {
		metrics.Count("datadog.trace_agent.normalizer.traces_dropped", count, append(tags, "reason:"+reason), 1)
	}

	for reason, count := range ts.TracesMalformed.tagValues() {
		metrics.Count("datadog.trace_agent.normalizer.traces_malformed", count, append(tags, "reason:"+reason), 1)
	}
}

// inlineNonZeroTagValues serializes the entries in this map into format "key1: value1, key2: value2, ...", sorted by
// key to ensure consistent output order. Only non-zero values are included.
func inlineNonZeroTagValues(statsMap map[string]int64) string {
	keys := make([]string, len(statsMap))
	i := 0
	for k := range statsMap {
		keys[i] = k
		i++
	}
	sort.Strings(keys)

	var results []string
	for _, key := range keys {
		value := statsMap[key]
		if value > 0 {
			results = append(results, fmt.Sprintf("%s:%d", key, value))
		}
	}

	return strings.Join(results, ", ")
}

// TracesDropped contains counts for reasons traces have been dropped
type TracesDropped struct {
	DecodingError int64
	EmptyTrace    int64
	TraceIDZero   int64
	SpanIDZero    int64
	ForeignSpan   int64
}

func (s *TracesDropped) tagValues() (result map[string]int64) {
	return map[string]int64{
		"decoding_error": atomic.LoadInt64(&s.DecodingError),
		"empty_trace":    atomic.LoadInt64(&s.EmptyTrace),
		"trace_id_zero":  atomic.LoadInt64(&s.TraceIDZero),
		"span_id_zero":   atomic.LoadInt64(&s.SpanIDZero),
		"foreign_span":   atomic.LoadInt64(&s.ForeignSpan),
	}
}

func (s *TracesDropped) String() string {
	return inlineNonZeroTagValues(s.tagValues())
}

// TracesMalformed contains counts for reasons malformed traces have been accepted after applying automatic fixes
type TracesMalformed struct {
	DuplicateSpanID       int64
	ServiceEmpty          int64
	ServiceTruncate       int64
	ServiceInvalid        int64
	SpanNameEmpty         int64
	SpanNameTruncate      int64
	SpanNameInvalid       int64
	ResourceEmpty         int64
	TypeTruncate          int64
	InvalidStartDate      int64
	InvalidDuration       int64
	InvalidHTTPStatusCode int64
}

// tagValues converts TracesMalformed into a map
func (s *TracesMalformed) tagValues() (result map[string]int64) {
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

func (s *TracesMalformed) String() string {
	return inlineNonZeroTagValues(s.tagValues())
}

// Stats holds the metrics that will be reported every 10s by the agent.
// Its fields require to be accessed in an atomic way.
type Stats struct {
	// TracesReceived is the total number of traces received, including the dropped ones.
	TracesReceived int64
	// TracesDropped contains stats about the count of dropped traces by reason
	TracesDropped TracesDropped
	// TracesMalformed contains stats about the count of malformed traces by reason
	TracesMalformed TracesMalformed
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

	// TracesDropped
	atomic.AddInt64(&s.TracesDropped.DecodingError, atomic.LoadInt64(&recent.TracesDropped.DecodingError))
	atomic.AddInt64(&s.TracesDropped.EmptyTrace, atomic.LoadInt64(&recent.TracesDropped.EmptyTrace))
	atomic.AddInt64(&s.TracesDropped.TraceIDZero, atomic.LoadInt64(&recent.TracesDropped.TraceIDZero))
	atomic.AddInt64(&s.TracesDropped.SpanIDZero, atomic.LoadInt64(&recent.TracesDropped.SpanIDZero))
	atomic.AddInt64(&s.TracesDropped.ForeignSpan, atomic.LoadInt64(&recent.TracesDropped.ForeignSpan))

	// Traces Malformed
	atomic.AddInt64(&s.TracesMalformed.DuplicateSpanID, atomic.LoadInt64(&recent.TracesMalformed.DuplicateSpanID))
	atomic.AddInt64(&s.TracesMalformed.ServiceEmpty, atomic.LoadInt64(&recent.TracesMalformed.ServiceEmpty))
	atomic.AddInt64(&s.TracesMalformed.ServiceTruncate, atomic.LoadInt64(&recent.TracesMalformed.ServiceTruncate))
	atomic.AddInt64(&s.TracesMalformed.ServiceInvalid, atomic.LoadInt64(&recent.TracesMalformed.ServiceInvalid))
	atomic.AddInt64(&s.TracesMalformed.SpanNameEmpty, atomic.LoadInt64(&recent.TracesMalformed.SpanNameEmpty))
	atomic.AddInt64(&s.TracesMalformed.SpanNameTruncate, atomic.LoadInt64(&recent.TracesMalformed.SpanNameTruncate))
	atomic.AddInt64(&s.TracesMalformed.SpanNameInvalid, atomic.LoadInt64(&recent.TracesMalformed.SpanNameInvalid))
	atomic.AddInt64(&s.TracesMalformed.ResourceEmpty, atomic.LoadInt64(&recent.TracesMalformed.ResourceEmpty))
	atomic.AddInt64(&s.TracesMalformed.TypeTruncate, atomic.LoadInt64(&recent.TracesMalformed.TypeTruncate))
	atomic.AddInt64(&s.TracesMalformed.InvalidStartDate, atomic.LoadInt64(&recent.TracesMalformed.InvalidStartDate))
	atomic.AddInt64(&s.TracesMalformed.InvalidDuration, atomic.LoadInt64(&recent.TracesMalformed.InvalidDuration))
	atomic.AddInt64(&s.TracesMalformed.InvalidHTTPStatusCode, atomic.LoadInt64(&recent.TracesMalformed.InvalidHTTPStatusCode))

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
	atomic.AddInt64(&s.TracesDropped.DecodingError, 0)
	atomic.AddInt64(&s.TracesDropped.EmptyTrace, 0)
	atomic.AddInt64(&s.TracesDropped.TraceIDZero, 0)
	atomic.AddInt64(&s.TracesDropped.SpanIDZero, 0)
	atomic.AddInt64(&s.TracesDropped.ForeignSpan, 0)
	atomic.AddInt64(&s.TracesMalformed.DuplicateSpanID, 0)
	atomic.AddInt64(&s.TracesMalformed.ServiceEmpty, 0)
	atomic.AddInt64(&s.TracesMalformed.ServiceTruncate, 0)
	atomic.AddInt64(&s.TracesMalformed.ServiceInvalid, 0)
	atomic.AddInt64(&s.TracesMalformed.SpanNameEmpty, 0)
	atomic.AddInt64(&s.TracesMalformed.SpanNameTruncate, 0)
	atomic.AddInt64(&s.TracesMalformed.SpanNameInvalid, 0)
	atomic.AddInt64(&s.TracesMalformed.ResourceEmpty, 0)
	atomic.AddInt64(&s.TracesMalformed.TypeTruncate, 0)
	atomic.AddInt64(&s.TracesMalformed.InvalidStartDate, 0)
	atomic.AddInt64(&s.TracesMalformed.InvalidDuration, 0)
	atomic.AddInt64(&s.TracesMalformed.InvalidHTTPStatusCode, 0)
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

// InfoString returns a string representation of the Stats struct containing standard operational stats (not problems)
func (s *Stats) InfoString() string {
	// Atomically load the stats
	tracesReceived := atomic.LoadInt64(&s.TracesReceived)
	tracesFiltered := atomic.LoadInt64(&s.TracesFiltered)
	// Omitting priority information, use expvar or metrics for debugging purpose
	tracesBytes := atomic.LoadInt64(&s.TracesBytes)
	servicesReceived := atomic.LoadInt64(&s.ServicesReceived)
	servicesBytes := atomic.LoadInt64(&s.ServicesBytes)
	eventsExtracted := atomic.LoadInt64(&s.EventsExtracted)
	eventsSampled := atomic.LoadInt64(&s.EventsSampled)

	return fmt.Sprintf("traces received: %d, traces filtered: %d, "+
		"traces amount: %d bytes, services received: %d, services amount: %d bytes, "+
		"events extracted: %d, events sampled: %d",
		tracesReceived, tracesFiltered,
		tracesBytes, servicesReceived, servicesBytes,
		eventsExtracted, eventsSampled)
}

// WarnString returns a string representation of the Stats struct containing only issues which we should be warning on
// if there are no issues then an empty string is returned
func (ts *TagStats) WarnString() string {
	var warnings []string
	droppedReasons := ts.TracesDropped.String()
	if len(droppedReasons) > 0 {
		warnings = append(warnings, fmt.Sprintf("dropped_traces(%s)", droppedReasons))
	}
	malformedReasons := ts.TracesMalformed.String()
	if len(malformedReasons) > 0 {
		warnings = append(warnings, fmt.Sprintf("malformed_traces(%s)", malformedReasons))
	}
	return strings.Join(warnings, ", ")
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

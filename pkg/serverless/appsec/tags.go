package appsec

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
)

type Span pb.Span

func (s *Span) SetTag(k, v string) {
	if s.Meta == nil {
		s.Meta = make(map[string]string)
	}
	s.Meta[k] = v
}

func (s *Span) SetMetric(k string, v float64) {
	if s.Metrics == nil {
		s.Metrics = make(map[string]float64)
	}
	s.Metrics[k] = v
}

// SetAppSecTags sets the AppSec-specific span tags that are expected to be in
// the web service entry span (span of type `web`) when AppSec is enabled.
func SetAppSecTags(span *Span) {
	span.SetMetric("_dd.appsec.enabled", 1)
	span.SetTag("_dd.runtime_family", "go")
}

// SetSecurityEventTags sets the AppSec-specific span tags when a security event occurred into the service entry span.
func SetSecurityEventTags(span *Span, events []json.RawMessage, headers, respHeaders map[string][]string) {
	setEventSpanTags(span, events)
	for h, v := range NormalizeHTTPHeaders(headers) {
		span.SetTag("http.request.headers."+h, v)
	}
	for h, v := range NormalizeHTTPHeaders(respHeaders) {
		span.SetTag("http.response.headers."+h, v)
	}
}

// List of HTTP headers we collect and send.
var collectedHTTPHeaders = [...]string{
	"host",
	"x-forwarded-for",
	"x-client-ip",
	"x-real-ip",
	"x-forwarded",
	"x-cluster-client-ip",
	"forwarded-for",
	"forwarded",
	"via",
	"true-client-ip",
	"content-length",
	"content-type",
	"content-encoding",
	"content-language",
	"forwarded",
	"user-agent",
	"accept",
	"accept-encoding",
	"accept-language",
}

func init() {
	// Required by sort.SearchStrings
	sort.Strings(collectedHTTPHeaders[:])
}

// NormalizeHTTPHeaders returns the HTTP headers following Datadog's
// normalization format.
func NormalizeHTTPHeaders(headers map[string][]string) (normalized map[string]string) {
	if len(headers) == 0 {
		return nil
	}
	normalized = make(map[string]string)
	for k, v := range headers {
		k = strings.ToLower(k)
		if i := sort.SearchStrings(collectedHTTPHeaders[:], k); i < len(collectedHTTPHeaders) && collectedHTTPHeaders[i] == k {
			normalized[k] = strings.Join(v, ",")
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

// SetEventSpanTags sets the security event span tags into the service entry span.
func setEventSpanTags(span *Span, events []json.RawMessage) error {
	// Set the appsec event span tag
	val, err := makeEventTagValue(events)
	if err != nil {
		return err
	}
	span.SetTag("_dd.appsec.json", string(val))
	// Keep this span due to the security event
	//
	// This is a workaround to tell the tracer that the trace was kept by AppSec.
	// Passing any other value than SamplerAppSec has no effect.
	// Customers should use `span.SetTag(ext.ManualKeep, true)` pattern
	// to keep the trace, manually.
	span.SetMetric(ext.ManualKeep, 5)
	span.SetTag("_dd.origin", "appsec")
	// Set the appsec.event tag needed by the appsec backend
	span.SetTag("appsec.event", "true")
	return nil
}

// Create the value of the security event tag.
func makeEventTagValue(events []json.RawMessage) (json.RawMessage, error) {
	var v interface{}
	if l := len(events); l == 1 {
		// eventTag is the structure to use in the `_dd.appsec.json` span tag.
		// In this case of 1 event, it already is an array as expected.
		type eventTag struct {
			Triggers json.RawMessage `json:"triggers"`
		}
		v = eventTag{Triggers: events[0]}
	} else {
		// eventTag is the structure to use in the `_dd.appsec.json` span tag.
		// With more than one event, we need to concatenate the arrays together
		// (ie. convert [][]json.RawMessage into []json.RawMessage).
		type eventTag struct {
			Triggers []json.RawMessage `json:"triggers"`
		}
		concatenated := make([]json.RawMessage, 0, l) // at least len(events)
		for _, event := range events {
			// Unmarshal the top level array
			var tmp []json.RawMessage
			if err := json.Unmarshal(event, &tmp); err != nil {
				return nil, fmt.Errorf("unexpected error while unserializing the appsec event `%s`: %v", string(event), err)
			}
			concatenated = append(concatenated, tmp...)
		}
		v = eventTag{Triggers: concatenated}
	}

	tag, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("unexpected error while serializing the appsec event span tag: %v", err)
	}
	return tag, nil
}

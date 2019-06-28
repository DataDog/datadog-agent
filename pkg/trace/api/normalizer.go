package api

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// MaxServiceLen the maximum length a service can have
	MaxServiceLen = 100
	// MaxNameLen the maximum length a name can have
	MaxNameLen = 100
	// MaxTypeLen the maximum length a span type can have
	MaxTypeLen = 100
	// DefaultServiceName is the default name we assign a service if it's missing and we have no reasonable fallback
	DefaultServiceName = "service"
	// DefaultSpanName is the default name we assign a span if it's missing and we have no reasonable fallback
	DefaultSpanName = "service.trace"
)

var (
	// Year2000NanosecTS is an arbitrary cutoff to spot weird-looking values
	Year2000NanosecTS = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC).UnixNano()
)

// normalize makes sure a Span is properly initialized and encloses the minimum required info, returning error if it
// is invalid beyond repair
func normalize(ts *info.TagStats, s *pb.Span) error {
	fallbackServiceName := DefaultServiceName
	if ts.Lang != "" {
		fallbackServiceName = ts.Lang
	}

	if s.TraceID == 0 {
		atomic.AddInt64(&ts.TracesDropped.TraceIDZero, 1)
		return fmt.Errorf("TraceID is zero (reason:trace_id_zero): %s", s)
	}
	if s.SpanID == 0 {
		atomic.AddInt64(&ts.TracesDropped.SpanIDZero, 1)
		return fmt.Errorf("SpanID is zero (reason:span_id_zero): %s", s)
	}

	// Service
	if s.Service == "" {
		atomic.AddInt64(&ts.SpansMalformed.ServiceEmpty, 1)
		log.Debugf("Fixing malformed trace. Service is empty (reason:service_empty), setting span.service=%s: %s", fallbackServiceName, s)
		s.Service = fallbackServiceName
	}
	if len(s.Service) > MaxServiceLen {
		atomic.AddInt64(&ts.SpansMalformed.ServiceTruncate, 1)
		log.Debugf("Fixing malformed trace. Service is too long (reason:service_truncate), truncating span.service to length=%d: %s", MaxServiceLen, s)
		s.Service = s.Service[:MaxServiceLen]
	}
	// service should comply with Datadog tag normalization as it's eventually a tag
	svc := normalizeTag(s.Service)
	if svc == "" {
		atomic.AddInt64(&ts.SpansMalformed.ServiceInvalid, 1)
		log.Debugf("Fixing malformed trace. Service is invalid (reason:service_invalid), replacing invalid span.service=%s with fallback span.service=%s: %s", s.Service, fallbackServiceName, s)
		svc = fallbackServiceName
	}
	s.Service = svc

	// Name
	if s.Name == "" {
		atomic.AddInt64(&ts.SpansMalformed.SpanNameEmpty, 1)
		log.Debugf("Fixing malformed trace. Name is empty (reason:span_name_empty), setting span.name=%s: %s", DefaultSpanName, s)
		s.Name = DefaultSpanName
	}
	if len(s.Name) > MaxNameLen {
		atomic.AddInt64(&ts.SpansMalformed.SpanNameTruncate, 1)
		log.Debugf("Fixing malformed trace. Name is too long (reason:span_name_truncate), truncating span.name to length=%d: %s", MaxServiceLen, s)
		s.Name = s.Name[:MaxNameLen]
	}
	// name shall comply with Datadog metric name normalization
	name, ok := normMetricNameParse(s.Name)
	if !ok {
		atomic.AddInt64(&ts.SpansMalformed.SpanNameInvalid, 1)
		log.Debugf("Fixing malformed trace. Name is invalid (reason:span_name_invalid), setting span.name=%s: %s", DefaultSpanName, s)
		name = DefaultSpanName
	}
	s.Name = name

	if s.Resource == "" {
		atomic.AddInt64(&ts.SpansMalformed.ResourceEmpty, 1)
		log.Debugf("Fixing malformed trace. Resource is empty (reason:resource_empty), setting span.resource=%s: %s", s.Name, s)
		s.Resource = s.Name
	}
	s.Resource = toUTF8(s.Resource)

	// ParentID, TraceID and SpanID set in the client could be the same
	// Supporting the ParentID == TraceID == SpanID for the root span, is compliant
	// with the Zipkin implementation. Furthermore, as described in the PR
	// https://github.com/openzipkin/zipkin/pull/851 the constraint that the
	// root span's ``trace id = span id`` has been removed
	if s.ParentID == s.TraceID && s.ParentID == s.SpanID {
		s.ParentID = 0
		log.Debugf("span.normalize: `ParentID`, `TraceID` and `SpanID` are the same; `ParentID` set to 0: %d", s.TraceID)
	}

	// Start & Duration as nanoseconds timestamps
	// if s.Start is very little, less than year 2000 probably a unit issue so discard
	// (or it is "le bug de l'an 2000")
	if s.Start < Year2000NanosecTS {
		atomic.AddInt64(&ts.SpansMalformed.InvalidStartDate, 1)
		log.Debugf("Fixing malformed trace. Start date is invalid (reason:invalid_start_date), setting span.start=time.now(): %s", s)
		s.Start = time.Now().UnixNano()
	}

	if s.Duration < 0 {
		atomic.AddInt64(&ts.SpansMalformed.InvalidDuration, 1)
		log.Debugf("Fixing malformed trace. Duration is invalid (reason:invalid_duration), setting span.duration=0: %s", s)
		s.Duration = 0
	}

	// ParentID set on the client side, no way of checking

	// Type
	s.Type = toUTF8(s.Type)
	if len(s.Type) > MaxTypeLen {
		atomic.AddInt64(&ts.SpansMalformed.TypeTruncate, 1)
		log.Debugf("Fixing malformed trace. Type is too long (reason:type_truncate), truncating span.type to length=%d: %s", MaxTypeLen, s)
		s.Type = s.Type[:MaxTypeLen]
	}

	for k, v := range s.Meta {
		utf8K := toUTF8(k)

		if k != utf8K {
			delete(s.Meta, k)
			k = utf8K
		}

		s.Meta[k] = toUTF8(v)
	}

	// Environment
	if env, ok := s.Meta["env"]; ok {
		s.Meta["env"] = normalizeTag(env)
	}

	// Status Code
	if sc, ok := s.Meta["http.status_code"]; ok {
		if !isValidStatusCode(sc) {
			atomic.AddInt64(&ts.SpansMalformed.InvalidHTTPStatusCode, 1)
			log.Debugf("Fixing malformed trace. HTTP status code is invalid (reason:invalid_http_status_code), dropping invalid http.status_code=%s: %s", sc, s)
			delete(s.Meta, "http.status_code")
		}
	}

	return nil
}

// normalizeTrace takes a trace and
// * rejects the trace if there is a trace ID discrepancy between 2 spans
// * rejects the trace if two spans have the same span_id
// * rejects empty traces
// * rejects traces where at least one span cannot be normalized
// * return the normalized trace and an error:
//   - nil if the trace can be accepted
//   - a reason tag explaining the reason the traces failed normalization
func normalizeTrace(ts *info.TagStats, t pb.Trace) error {
	if len(t) == 0 {
		atomic.AddInt64(&ts.TracesDropped.EmptyTrace, 1)
		return errors.New("trace is empty (reason:empty_trace)")
	}

	spanIDs := make(map[uint64]struct{})
	firstSpan := t[0]

	for _, span := range t {
		if span.TraceID != firstSpan.TraceID {
			atomic.AddInt64(&ts.TracesDropped.ForeignSpan, 1)
			return fmt.Errorf("trace has foreign span (reason:foreign_span): %s", span)
		}

		if err := normalize(ts, span); err != nil {
			return err
		}

		if _, ok := spanIDs[span.SpanID]; ok {
			atomic.AddInt64(&ts.SpansMalformed.DuplicateSpanID, 1)
			log.Debugf("Found malformed trace with duplicate span ID (reason:duplicate_span_id): %s", span)
		}

		spanIDs[span.SpanID] = struct{}{}
	}

	return nil
}

func isValidStatusCode(sc string) bool {
	if code, err := strconv.ParseUint(sc, 10, 64); err == nil {
		return 100 <= code && code < 600
	}
	return false
}

// This code is borrowed from dd-go metric normalization

// fast isAlpha for ascii
func isAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// fast isAlphaNumeric for ascii
func isAlphaNum(b byte) bool {
	return isAlpha(b) || (b >= '0' && b <= '9')
}

// normMetricNameParse normalizes metric names with a parser instead of using
// garbage-creating string replacement routines.
func normMetricNameParse(name string) (string, bool) {
	if name == "" || len(name) > MaxNameLen {
		return name, false
	}

	var i, ptr int
	res := make([]byte, 0, len(name))

	// skip non-alphabetic characters
	for ; i < len(name) && !isAlpha(name[i]); i++ {
	}

	// if there were no alphabetic characters it wasn't valid
	if i == len(name) {
		return "", false
	}

	for ; i < len(name); i++ {
		switch {
		case isAlphaNum(name[i]):
			res = append(res, name[i])
			ptr++
		case name[i] == '.':
			// we skipped all non-alpha chars up front so we have seen at least one
			switch res[ptr-1] {
			// overwrite underscores that happen before periods
			case '_':
				res[ptr-1] = '.'
			default:
				res = append(res, '.')
				ptr++
			}
		default:
			// we skipped all non-alpha chars up front so we have seen at least one
			switch res[ptr-1] {
			// no double underscores, no underscores after periods
			case '.', '_':
			default:
				res = append(res, '_')
				ptr++
			}
		}
	}

	if res[ptr-1] == '_' {
		res = res[:ptr-1]
	}

	return string(res), true
}

// toUTF8 forces the string to utf-8 by replacing illegal character sequences with the utf-8 replacement character.
func toUTF8(s string) string {
	if utf8.ValidString(s) {
		// if string is already valid utf8, return it as-is. Checking validity is cheaper than blindly rewriting.
		return s
	}

	in := strings.NewReader(s)
	var out bytes.Buffer
	out.Grow(len(s))

	for {
		r, _, err := in.ReadRune()
		if err != nil {
			// note: by contract, if `in` contains non-valid utf-8, no error is returned. Rather the utf-8 replacement
			// character is returned. Therefore, the only error should usually be io.EOF indicating end of string.
			// If any other error is returned by chance, we quit as well, outputting whatever part of the string we
			// had already constructed.
			return out.String()
		}

		out.WriteRune(r)
	}
}

const maxTagLength = 200

// normalizeTag applies some normalization to ensure the tags match the backend requirements.
func normalizeTag(v string) string {
	// the algorithm works by creating a set of cuts marking start and end offsets in v
	// that have to be replaced with underscore (_)
	if len(v) == 0 {
		return ""
	}
	var (
		trim  int      // start character (if trimming)
		cuts  [][2]int // sections to discard: (start, end) pairs
		chars int      // number of characters processed
	)
	var (
		i    int  // current byte
		r    rune // current rune
		jump int  // tracks how many bytes the for range advances on its next iteration
	)
	tag := []byte(v)
	for i, r = range v {
		jump = utf8.RuneLen(r) // next i will be i+jump
		if r == utf8.RuneError {
			// On invalid UTF-8, the for range advances only 1 byte (see: https://golang.org/ref/spec#For_range (point 2)).
			// However, utf8.RuneError is equivalent to unicode.ReplacementChar so we should rely on utf8.DecodeRune to tell
			// us whether this is an actual error or just a unicode.ReplacementChar that was present in the string.
			_, width := utf8.DecodeRune(tag[i:])
			jump = width
		}
		// fast path; all letters (and colons) are ok
		switch {
		case r >= 'a' && r <= 'z' || r == ':':
			chars++
			goto end
		case r >= 'A' && r <= 'Z':
			// lower-case
			tag[i] += 'a' - 'A'
			chars++
			goto end
		}
		if unicode.IsUpper(r) {
			// lowercase this character
			if low := unicode.ToLower(r); utf8.RuneLen(r) == utf8.RuneLen(low) {
				// but only if the width of the lowercased character is the same;
				// there are some rare edge-cases where this is not the case, such
				// as \u017F (ſ)
				utf8.EncodeRune(tag[i:], low)
				r = low
			}
		}
		switch {
		case unicode.IsLetter(r):
			chars++
		case chars == 0:
			// this character can not start the string, trim
			trim = i + jump
			goto end
		case unicode.IsDigit(r) || r == '.' || r == '/' || r == '-':
			chars++
		default:
			// illegal character
			if n := len(cuts); n > 0 && cuts[n-1][1] >= i {
				// merge intersecting cuts
				cuts[n-1][1] += jump
			} else {
				// start a new cut
				cuts = append(cuts, [2]int{i, i + jump})
			}
		}
	end:
		if i+jump >= 2*maxTagLength {
			// bail early if the tag contains a lot of non-letter/digit characters.
			// If a tag is test🍣🍣[...]🍣, then it's unlikely to be a properly formatted tag
			break
		}
		if chars >= maxTagLength {
			// we've reached the maximum
			break
		}
	}

	tag = tag[trim : i+jump] // trim start and end
	if len(cuts) == 0 {
		// tag was ok, return it as it is
		return string(tag)
	}
	delta := trim // cut offsets delta
	for _, cut := range cuts {
		// start and end of cut, including delta from previous cuts:
		start, end := cut[0]-delta, cut[1]-delta

		if end >= len(tag) {
			// this cut includes the end of the string; discard it
			// completely and finish the loop.
			tag = tag[:start]
			break
		}
		// replace the beginning of the cut with '_'
		tag[start] = '_'
		if end-start == 1 {
			// nothing to discard
			continue
		}
		// discard remaining characters in the cut
		copy(tag[start+1:], tag[end:])

		// shorten the slice
		tag = tag[:len(tag)-(end-start)+1]

		// count the new delta for future cuts
		delta += cut[1] - cut[0] - 1
	}
	return string(tag)
}

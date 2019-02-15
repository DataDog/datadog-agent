package api

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	log "github.com/cihub/seelog"
)

const (
	// MaxServiceLen the maximum length a service can have
	MaxServiceLen = 100
	// MaxNameLen the maximum length a name can have
	MaxNameLen = 100
	// MaxTypeLen the maximum length a span type can have
	MaxTypeLen = 100
	// MaxEndDateOffset the maximum amount of time in the future we
	// tolerate for span end dates
	MaxEndDateOffset = 10 * time.Minute
)

var (
	// Year2000NanosecTS is an arbitrary cutoff to spot weird-looking values
	Year2000NanosecTS = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC).UnixNano()
)

// normalize makes sure a Span is properly initialized and encloses the minimum required info
func normalize(s *pb.Span) error {
	// Service
	if s.Service == "" {
		return errors.New("empty `Service`")
	}
	if len(s.Service) > MaxServiceLen {
		return fmt.Errorf("`Service` too long (%d chars max): %s", MaxServiceLen, s.Service)
	}
	// service should comply with Datadog tag normalization as it's eventually a tag
	svc := normalizeTag(s.Service)
	if svc == "" {
		return fmt.Errorf("invalid `Service`: %q", s.Service)
	}
	s.Service = svc

	// Name
	if s.Name == "" {
		return errors.New("empty `Name`")
	}
	if len(s.Name) > MaxNameLen {
		return fmt.Errorf("`Name` too long (%d chars max): %s", MaxNameLen, s.Name)
	}
	// name shall comply with Datadog metric name normalization
	name, ok := normMetricNameParse(s.Name)
	if !ok {
		return fmt.Errorf("invalid `Name`: %s", s.Name)
	}
	s.Name = name

	// Resource
	resource := toUTF8(s.Resource)
	if s.Resource == "" {
		return fmt.Errorf("`Resource` is invalid UTF-8: %q", resource)
	}
	s.Resource = resource

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
		return fmt.Errorf("invalid `Start` (must be nanosecond epoch): %d", s.Start)
	}

	// If the end date is too far away in the future, it's probably a mistake.
	if s.Start+s.Duration > time.Now().Add(MaxEndDateOffset).UnixNano() {
		return fmt.Errorf("invalid `Start`+`Duration`: too far in the future")
	}

	if s.Duration <= 0 {
		return fmt.Errorf("invalid `Duration`: %d", s.Duration)
	}

	// ParentID set on the client side, no way of checking

	// Type
	typ := toUTF8(s.Type)
	if typ == "" {
		return fmt.Errorf("`Type` is invalid UTF-8: %q", typ)
	}
	s.Type = typ
	if len(s.Type) > MaxTypeLen {
		return fmt.Errorf("`Type` too long (%d chars max): %s", MaxTypeLen, s.Type)
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
			delete(s.Meta, "http.status_code")
			log.Debugf("Drop invalid meta `http.status_code`: %s", sc)
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
//   - an error string if the trace needs to be dropped
func normalizeTrace(t pb.Trace) error {
	if len(t) == 0 {
		return errors.New("empty trace")
	}

	spanIDs := make(map[uint64]struct{})
	traceID := t[0].TraceID

	for _, span := range t {
		if span.TraceID == 0 {
			return errors.New("empty `TraceID`")
		}
		if span.SpanID == 0 {
			return errors.New("empty `SpanID`")
		}
		if _, ok := spanIDs[span.SpanID]; ok {
			return fmt.Errorf("duplicate `SpanID` %v (span %v)", span.SpanID, span)
		}
		if span.TraceID != traceID {
			return fmt.Errorf("foreign span in trace (Name:TraceID) %s:%x != %s:%x", t[0].Name, t[0].TraceID, span.Name, span.TraceID)
		}
		if err := normalize(span); err != nil {
			return fmt.Errorf("invalid span (SpanID:%d): %v", span.SpanID, err)
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

// normalizeTag applies some normalization to ensure the tags match the
// backend requirements
func normalizeTag(tag string) string {
	if len(tag) == 0 {
		return ""
	}
	var (
		trim   int      // start character (if trimming)
		wiping bool     // true when the previous character has been discarded
		wipe   [][2]int // sections to discard: (start, end) pairs
		chars  int      // number of characters processed
	)
	var (
		i int  // current byte
		c rune // current rune
	)
	norm := []byte(tag)
	for i, c = range tag {
		if chars >= maxTagLength {
			// we've reached the maximum
			break
		}
		// fast path; all letters (and colons) are ok
		switch {
		case c >= 'a' && c <= 'z' || c == ':':
			chars++
			wiping = false
			continue
		case c >= 'A' && c <= 'Z':
			// lower-case
			norm[i] += 'a' - 'A'
			chars++
			wiping = false
			continue
		}

		if utf8.ValidRune(c) && unicode.IsUpper(c) {
			// lowercase this character
			if low := unicode.ToLower(c); utf8.RuneLen(c) == utf8.RuneLen(low) {
				// but only if the width of the lowercased character is the same;
				// there are some rare edge-cases where this is not the case, such
				// as \u017F (ſ)
				utf8.EncodeRune(norm[i:], low)
				c = low
			}
		}
		switch {
		case unicode.IsLetter(c):
			chars++
			wiping = false
		case chars == 0:
			// this character can not start the string, trim
			trim = i + utf8.RuneLen(c)
			continue
		case unicode.IsDigit(c) || c == '.' || c == '/' || c == '-':
			chars++
			wiping = false
		default:
			// illegal character
			if !wiping {
				// start a new cut
				wipe = append(wipe, [2]int{i, i + utf8.RuneLen(c)})
				wiping = true
			} else {
				// lengthen current cut
				wipe[len(wipe)-1][1] += utf8.RuneLen(c)
			}
		}
	}

	norm = norm[trim : i+utf8.RuneLen(c)] // trim start and end
	if len(wipe) == 0 {
		// tag was ok, return it as it is
		return string(norm)
	}
	delta := trim // cut offsets delta
	for _, cut := range wipe {
		// start and end of cut, including delta from previous cuts:
		start, end := cut[0]-delta, cut[1]-delta

		if end >= len(norm) {
			// this cut includes the end of the string; discard it
			// completely and finish the loop.
			norm = norm[:start]
			break
		}
		// replace the beginning of the cut with '_'
		norm[start] = '_'
		if end-start == 1 {
			// nothing to discard
			continue
		}
		// discard remaining characters in the cut
		copy(norm[start+1:], norm[end:])

		// shorten the slice
		norm = norm[:len(norm)-(end-start)+1]

		// count the new delta for future cuts
		delta += cut[1] - cut[0] - 1
	}
	return string(norm)
}

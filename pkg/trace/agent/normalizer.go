// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	normalizeutil "github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
)

const (
	// MaxTypeLen the maximum length a span type can have
	MaxTypeLen = 100
	// tagOrigin specifies the origin of the trace.
	// DEPRECATED: Origin is now specified as a TraceChunk field.
	tagOrigin = "_dd.origin"
	// tagSamplingPriority specifies the sampling priority of the trace.
	// DEPRECATED: Priority is now specified as a TraceChunk field.
	tagSamplingPriority = "_sampling_priority_v1"
	// peerServiceKey is the key for the peer.service meta field.
	peerServiceKey = "peer.service"
	// baseServiceKey is the key for the _dd.base_service meta field.
	baseServiceKey = "_dd.base_service"
)

var (
	// Year2000NanosecTS is an arbitrary cutoff to spot weird-looking values
	Year2000NanosecTS = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC).UnixNano()
)

// normalizeService handles service normalization for both pb.Span and idx.InternalSpan
func (a *Agent) normalizeService(ts *info.TagStats, service string, lang string) (string, error) {
	svc, err := normalizeutil.NormalizeService(service, lang)
	switch err {
	case normalizeutil.ErrEmpty:
		ts.SpansMalformed.ServiceEmpty.Inc()
		log.Debugf("Fixing malformed trace. Service is empty (reason:service_empty), setting span.service=%s: %s", service, service)
	case normalizeutil.ErrTooLong:
		ts.SpansMalformed.ServiceTruncate.Inc()
		log.Debugf("Fixing malformed trace. Service is too long (reason:service_truncate), truncating span.service to length=%d: %s", normalizeutil.MaxServiceLen, service)
	case normalizeutil.ErrInvalid:
		ts.SpansMalformed.ServiceInvalid.Inc()
		log.Debugf("Fixing malformed trace. Service is invalid (reason:service_invalid), replacing invalid span.service=%s with fallback span.service=%s: %s", service, svc, service)
	}
	return svc, err
}

// normalizePeerService handles peer service normalization for both pb.Span and idx.InternalSpan
func (a *Agent) normalizePeerService(ts *info.TagStats, pSvc string) string {
	ps, err := normalizeutil.NormalizePeerService(pSvc)
	switch err {
	case normalizeutil.ErrTooLong:
		ts.SpansMalformed.PeerServiceTruncate.Inc()
		log.Debugf("Fixing malformed trace. peer.service is too long (reason:peer_service_truncate), truncating peer.service to length=%d: %s", normalizeutil.MaxServiceLen, ps)
	case normalizeutil.ErrInvalid:
		ts.SpansMalformed.PeerServiceInvalid.Inc()
		log.Debugf("Fixing malformed trace. peer.service is invalid (reason:peer_service_invalid), replacing invalid peer.service=%s with empty string", pSvc)
	default:
		if err != nil {
			log.Debugf("Unexpected error in peer.service normalization from original value (%s) to new value (%s): %s", pSvc, ps, err)
		}
	}
	return ps
}

// normalizeBaseService handles base service normalization for both pb.Span and idx.InternalSpan
func (a *Agent) normalizeBaseService(ts *info.TagStats, bSvc string) string {
	bs, err := normalizeutil.NormalizePeerService(bSvc)
	switch err {
	case normalizeutil.ErrTooLong:
		ts.SpansMalformed.BaseServiceTruncate.Inc()
		log.Debugf("Fixing malformed trace. _dd.base_service is too long (reason:base_service_truncate), truncating _dd.base_service to length=%d: %s", normalizeutil.MaxServiceLen, bs)
	case normalizeutil.ErrInvalid:
		ts.SpansMalformed.BaseServiceInvalid.Inc()
		log.Debugf("Fixing malformed trace. _dd.base_service is invalid (reason:base_service_invalid), replacing invalid _dd.base_service=%s with empty string", bSvc)
	default:
		if err != nil {
			log.Debugf("Unexpected error in _dd.base_service normalization from original value (%s) to new value (%s): %s", bSvc, bs, err)
		}
	}
	return bs
}

// normalizeName handles name normalization for both pb.Span and idx.InternalSpan
func (a *Agent) normalizeName(ts *info.TagStats, name string) (string, error) {
	newName, err := normalizeutil.NormalizeName(name)
	switch err {
	case normalizeutil.ErrEmpty:
		ts.SpansMalformed.SpanNameEmpty.Inc()
		log.Debugf("Fixing malformed trace. Name is empty (reason:span_name_empty), setting span.name=%s: %s", name, name)
	case normalizeutil.ErrTooLong:
		ts.SpansMalformed.SpanNameTruncate.Inc()
		log.Debugf("Fixing malformed trace. Name is too long (reason:span_name_truncate), truncating span.name to length=%d: %s", normalizeutil.MaxServiceLen, name)
	case normalizeutil.ErrInvalid:
		ts.SpansMalformed.SpanNameInvalid.Inc()
		log.Debugf("Fixing malformed trace. Name is invalid (reason:span_name_invalid), setting span.name=%s: %s", name, name)
	}
	return newName, err
}

// validateAndFixDuration handles duration validation for both pb.Span and idx.InternalSpan
func (a *Agent) validateAndFixDuration(ts *info.TagStats, start int64, duration int64) int64 {
	if duration < 0 {
		ts.SpansMalformed.InvalidDuration.Inc()
		log.Debugf("Fixing malformed trace. Duration is invalid (reason:invalid_duration), setting span.duration=0")
		return 0
	}
	if duration > math.MaxInt64-start {
		ts.SpansMalformed.InvalidDuration.Inc()
		log.Debugf("Fixing malformed trace. Duration is too large and causes overflow (reason:invalid_duration), setting span.duration=0")
		return 0
	}
	return duration
}

// validateAndFixStartTime handles start time validation for both pb.Span and idx.InternalSpan
func (a *Agent) validateAndFixStartTime(ts *info.TagStats, start int64, duration int64) int64 {
	if start < Year2000NanosecTS {
		ts.SpansMalformed.InvalidStartDate.Inc()
		log.Debugf("Fixing malformed trace. Start date is invalid (reason:invalid_start_date), setting span.start=time.now()")
		now := time.Now().UnixNano()
		newStart := now - duration
		if newStart < 0 {
			return now
		}
		return newStart
	}
	return start
}

// validateAndFixType handles type validation for both pb.Span and idx.InternalSpan
func (a *Agent) validateAndFixType(ts *info.TagStats, spanType string) string {
	if len(spanType) > MaxTypeLen {
		ts.SpansMalformed.TypeTruncate.Inc()
		log.Debugf("Fixing malformed trace. Type is too long (reason:type_truncate), truncating span.type to length=%d: %s", MaxTypeLen, spanType)
		return normalizeutil.TruncateUTF8(spanType, MaxTypeLen)
	}
	return spanType
}

// validateAndFixHTTPStatusCode handles HTTP status code validation for both pb.Span and idx.InternalSpan
func (a *Agent) validateAndFixHTTPStatusCode(ts *info.TagStats, sc string) (string, bool) {
	if !isValidStatusCode(sc) {
		ts.SpansMalformed.InvalidHTTPStatusCode.Inc()
		log.Debugf("Fixing malformed trace. HTTP status code is invalid (reason:invalid_http_status_code), dropping invalid http.status_code=%s", sc)
		return "", false
	}
	return sc, true
}

// normalizeSpanLinks handles span links normalization for both pb.Span and idx.InternalSpan
func (a *Agent) normalizeSpanLinks(links []*pb.SpanLink) {
	for _, link := range links {
		if val, ok := link.Attributes["link.name"]; ok {
			newName, err := normalizeutil.NormalizeName(val)
			if err != nil {
				log.Debugf("Fixing malformed trace. 'link.name' attribute in span link is invalid (reason=%q), setting link.Attributes[\"link.name\"]=%s", err, newName)
			}
			link.Attributes["link.name"] = newName
		}
	}
}

// validateAndFixDurationV1 handles duration validation for idx.InternalSpan
func (a *Agent) validateAndFixDurationV1(ts *info.TagStats, start uint64, duration uint64) uint64 {
	if duration > math.MaxInt64-uint64(start) {
		ts.SpansMalformed.InvalidDuration.Inc()
		log.Debugf("Fixing malformed trace. Duration is too large and causes overflow (reason:invalid_duration), setting span.duration=0")
		return 0
	}
	return duration
}

// validateAndFixStartTimeV1 handles start time validation for idx.InternalSpan
func (a *Agent) validateAndFixStartTimeV1(ts *info.TagStats, start uint64, duration uint64) uint64 {
	if start < uint64(Year2000NanosecTS) {
		ts.SpansMalformed.InvalidStartDate.Inc()
		log.Debugf("Fixing malformed trace. Start date is invalid (reason:invalid_start_date), setting span.start=time.now()")
		now := uint64(time.Now().UnixNano())
		newStart := now - duration
		if newStart > now { // Check for underflow
			return now
		}
		return newStart
	}
	return start
}

// normalizeSpanLinksV1 handles span links normalization for idx.InternalSpan
func (a *Agent) normalizeSpanLinksV1(links []*idx.InternalSpanLink) {
	for _, link := range links {
		if val, ok := link.GetAttributeAsString("link.name"); ok {
			newName, err := normalizeutil.NormalizeName(val)
			if err != nil {
				log.Debugf("Fixing malformed trace. 'link.name' attribute in span link is invalid (reason=%q), setting link.Attributes[\"link.name\"]=%s", err, newName)
			}
			link.SetStringAttribute("link.name", newName)
		}
	}
}

// normalize makes sure a Span is properly initialized and encloses the minimum required info, returning error if it
// is invalid beyond repair
func (a *Agent) normalize(ts *info.TagStats, s *pb.Span) error {
	if s.TraceID == 0 {
		ts.TracesDropped.TraceIDZero.Inc()
		return fmt.Errorf("TraceID is zero (reason:trace_id_zero): %s", s)
	}
	if s.SpanID == 0 {
		ts.TracesDropped.SpanIDZero.Inc()
		return fmt.Errorf("SpanID is zero (reason:span_id_zero): %v", s)
	}

	svc, _ := a.normalizeService(ts, s.Service, ts.Lang)
	s.Service = svc

	if pSvc, ok := s.Meta[peerServiceKey]; ok {
		s.Meta[peerServiceKey] = a.normalizePeerService(ts, pSvc)
	}

	if bSvc, ok := s.Meta[baseServiceKey]; ok {
		s.Meta[baseServiceKey] = a.normalizeBaseService(ts, bSvc)
	}

	if a.conf.HasFeature("component2name") {
		if v, ok := s.Meta["component"]; ok {
			s.Name = v
		}
	}

	s.Name, _ = a.normalizeName(ts, s.Name)

	if s.Resource == "" {
		ts.SpansMalformed.ResourceEmpty.Inc()
		log.Debugf("Fixing malformed trace. Resource is empty (reason:resource_empty), setting span.resource=%s: %s", s.Name, s)
		s.Resource = s.Name
	}

	if s.ParentID == s.TraceID && s.ParentID == s.SpanID {
		s.ParentID = 0
		log.Debugf("span.normalize: `ParentID`, `TraceID` and `SpanID` are the same; `ParentID` set to 0: %d", s.TraceID)
	}

	s.Duration = a.validateAndFixDuration(ts, s.Start, s.Duration)
	s.Start = a.validateAndFixStartTime(ts, s.Start, s.Duration)

	s.Type = a.validateAndFixType(ts, s.Type)

	if env, ok := s.Meta["env"]; ok {
		s.Meta["env"] = normalizeutil.NormalizeTagValue(env)
	}

	if sc, ok := s.Meta["http.status_code"]; ok {
		if _, valid := a.validateAndFixHTTPStatusCode(ts, sc); !valid {
			delete(s.Meta, "http.status_code")
		}
	}

	if len(s.SpanLinks) > 0 {
		a.normalizeSpanLinks(s.SpanLinks)
	}
	return nil
}

// normalizeV1 makes sure an InternalSpan is properly initialized and encloses the minimum required info, returning error if it
// is invalid beyond repair
func (a *Agent) normalizeV1(ts *info.TagStats, s *idx.InternalSpan) error {
	if s.SpanID() == 0 {
		ts.TracesDropped.SpanIDZero.Inc()
		return fmt.Errorf("SpanID is zero (reason:span_id_zero): %v", s)
	}

	svc, _ := a.normalizeService(ts, s.Service(), ts.Lang)
	s.SetService(svc)

	if pSvc, ok := s.GetAttributeAsString(peerServiceKey); ok {
		s.SetStringAttribute(peerServiceKey, a.normalizePeerService(ts, pSvc))
	}

	if bSvc, ok := s.GetAttributeAsString(baseServiceKey); ok {
		s.SetStringAttribute(baseServiceKey, a.normalizeBaseService(ts, bSvc))
	}

	if a.conf.HasFeature("component2name") {
		if v, ok := s.GetAttributeAsString("component"); ok {
			s.SetName(v)
		}
	}

	newName, _ := a.normalizeName(ts, s.Name())
	s.SetName(newName)

	if s.Resource() == "" {
		ts.SpansMalformed.ResourceEmpty.Inc()
		log.Debugf("Fixing malformed trace. Resource is empty (reason:resource_empty), setting span.resource=%s: %s", s.Name, s)
		s.SetResource(s.Name())
	}

	if s.ParentID() == s.SpanID() {
		s.SetParentID(0)
		log.Debugf("span.normalize: `ParentID` and `SpanID` are the same; `ParentID` set to 0: %d", s.SpanID())
	}

	s.SetDuration(a.validateAndFixDurationV1(ts, s.Start(), s.Duration()))
	s.SetStart(a.validateAndFixStartTimeV1(ts, s.Start(), s.Duration()))

	s.SetType(a.validateAndFixType(ts, s.Type()))

	if env := s.Env(); env != "" {
		s.SetEnv(normalizeutil.NormalizeTagValue(env))
	}

	if sc, ok := s.GetAttributeAsString("http.status_code"); ok {
		if _, valid := a.validateAndFixHTTPStatusCode(ts, sc); !valid {
			s.DeleteAttribute("http.status_code")
		}
	}

	if s.LenLinks() > 0 {
		a.normalizeSpanLinksV1(s.Links())
	}
	return nil
}

// setChunkAttributes takes a trace chunk and from the root span
// * populates Origin field if it wasn't populated
// * populates Priority field if it wasn't populated
// * promotes the decision maker found in any internal span to a chunk tag
func setChunkAttributes(chunk *pb.TraceChunk, root *pb.Span) {
	// check if priority is already populated
	if chunk.Priority == int32(sampler.PriorityNone) {
		// Older tracers set sampling priority in the root span.
		if p, ok := root.Metrics[tagSamplingPriority]; ok {
			chunk.Priority = int32(p)
		} else {
			for _, s := range chunk.Spans {
				if p, ok := s.Metrics[tagSamplingPriority]; ok {
					chunk.Priority = int32(p)
					break
				}
			}
		}
	}
	if chunk.Origin == "" && root.Meta != nil {
		// Older tracers set origin in the root span.
		chunk.Origin = root.Meta[tagOrigin]
	}

	if _, ok := chunk.Tags[tagDecisionMaker]; !ok {
		for _, span := range chunk.Spans {
			// First span wins
			if dm, ok := span.Meta[tagDecisionMaker]; ok {
				chunk.Tags[tagDecisionMaker] = dm
				break
			}
			// There are downstream systems that rely on this tag being on the span
			// delete(span.Meta, tagDecisionMaker)
		}
	}
}

// normalizeTrace takes a trace and
// * rejects the trace if there is a trace ID discrepancy between 2 spans
// * rejects the trace if two spans have the same span_id
// * rejects empty traces
// * rejects traces where at least one span cannot be normalized
// * return the normalized trace and an error:
//   - nil if the trace can be accepted
//   - a reason tag explaining the reason the traces failed normalization
func (a *Agent) normalizeTrace(ts *info.TagStats, t pb.Trace) error {
	if len(t) == 0 {
		ts.TracesDropped.EmptyTrace.Inc()
		return errors.New("trace is empty (reason:empty_trace)")
	}

	spanIDs := make(map[uint64]struct{})
	firstSpan := t[0]

	for _, span := range t {
		if span == nil {
			continue
		}
		if firstSpan == nil {
			firstSpan = span
		}
		if span.TraceID != firstSpan.TraceID {
			ts.TracesDropped.ForeignSpan.Inc()
			return fmt.Errorf("trace has foreign span (reason:foreign_span): %s", span)
		}
		if err := a.normalize(ts, span); err != nil {
			return err
		}
		if _, ok := spanIDs[span.SpanID]; ok {
			ts.SpansMalformed.DuplicateSpanID.Inc()
			log.Debugf("Found malformed trace with duplicate span ID (reason:duplicate_span_id): %s", span)
		}
		spanIDs[span.SpanID] = struct{}{}
	}

	return nil
}

// normalizeTraceChunkV1 takes a trace and
// * logs a message and increments a metric if two spans have the same span_id
// * rejects empty traces
// * rejects traces where at least one span cannot be normalized
// * return the normalized trace and an error:
//   - nil if the trace can be accepted
//   - a reason tag explaining the reason the traces failed normalization
func (a *Agent) normalizeTraceChunkV1(ts *info.TagStats, t *idx.InternalTraceChunk) error {
	if len(t.Spans) == 0 {
		ts.TracesDropped.EmptyTrace.Inc()
		return errors.New("trace is empty (reason:empty_trace)")
	}

	spanIDs := make(map[uint64]struct{})
	firstSpan := t.Spans[0]

	for _, span := range t.Spans {
		if span == nil {
			continue
		}
		if firstSpan == nil {
			firstSpan = span
		}
		if err := a.normalizeV1(ts, span); err != nil {
			return err
		}
		if _, ok := spanIDs[span.SpanID()]; ok {
			ts.SpansMalformed.DuplicateSpanID.Inc()
			log.Debugf("Found malformed trace with duplicate span ID (reason:duplicate_span_id): %s", span)
		}
		spanIDs[span.SpanID()] = struct{}{}
	}

	return nil
}

func (a *Agent) normalizeStatsGroup(b *pb.ClientGroupedStats, lang string) {
	b.Name, _ = normalizeutil.NormalizeName(b.Name)
	b.Service, _ = normalizeutil.NormalizeService(b.Service, lang)
	if b.Resource == "" {
		b.Resource = b.Name
	}
	b.Resource, _ = a.TruncateResource(b.Resource)
}

func isValidStatusCode(sc string) bool {
	if code, err := strconv.ParseUint(sc, 10, 64); err == nil {
		return 100 <= code && code < 600
	}
	return false
}

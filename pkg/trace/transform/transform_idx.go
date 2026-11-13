// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transform

import (
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
)

// OtelSpanToIdxSpan converts an OTel span to a Datadog idx span whose strings are stored in strings.
//
// The result is equivalent to converting the output of OtelSpanToDDSpan with the trace API's
// pb-to-idx conversion: env, version, component and span.kind are set as span fields instead of
// attributes, the other string tags are string attributes and the numeric tags are double
// attributes, and span events and links are native idx events and links. The trace ID is not part
// of an idx span; the caller sets the full 128-bit trace ID of otelspan on the trace chunk.
func OtelSpanToIdxSpan(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	conf *config.AgentConfig,
	strings *idx.StringTable,
) *idx.InternalSpan {
	span, _ := OtelSpanToIdxSpanWithConvertedFields(otelspan, otelres, lib, conf, strings, nil, "")
	return span
}

// IdxSpanSamplingTags holds the tags of a span converted by OtelSpanToIdxSpanWithConvertedFields
// that the trace-agent OTLP receiver reads to make sampling decisions, with the values that
// OtelSpanToDDSpan sets in the span Meta. The idx span may hold a different value for them: when
// a numeric attribute has the same key, the idx span holds the number instead of the tag.
type IdxSpanSamplingTags struct {
	// DecisionMaker is the _dd.p.dm tag.
	DecisionMaker string
	// Tracestate is the w3c.tracestate tag.
	Tracestate string
}

// OtelSpanToIdxSpanWithConvertedFields is OtelSpanToIdxSpan, and it also records in fields, when
// not nil, the span values that are promoted to the trace chunk and tracer payload, the way the
// trace API's pb-to-idx conversion does for each span of a chunk: the sampling priority of the span
// replaces fields.SamplingPriority when the span has one, and the env, app version, hostname, git
// commit SHA, sampling mechanism, APM mode and origin are only set if fields doesn't have them yet.
// The trace ID fields and the root sampling priority merge are left to the caller. It also returns
// the sampling tags of the span. traceIDHex is the hex-encoded trace ID of otelspan, which callers
// converting several spans of a trace can compute once, or empty to have it computed.
func OtelSpanToIdxSpanWithConvertedFields(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	conf *config.AgentConfig,
	strings *idx.StringTable,
	fields *idx.SpanConvertedFields,
	traceIDHex string,
) (*idx.InternalSpan, IdxSpanSamplingTags) {
	isTopLevel, topLevelByKind := otelSpanTopLevel(otelspan, conf)
	// Converted to the interface once, rather than at each of the calls below.
	var spanAccessor semantics.Accessor = semantics.NewOTelSpanAccessor(otelspan.Attributes(), otelres.Attributes())
	// The conversion adds up to about 10 attributes to those of the span and resource.
	numAttrs := otelspan.Attributes().Len() + otelres.Attributes().Len() + lib.Attributes().Len() + 10
	sink := idxSpanSink{
		strings: strings,
		span: &idx.Span{
			SpanID:     OTelSpanIDToUint64(otelspan.SpanID()),
			ParentID:   OTelSpanIDToUint64(otelspan.ParentSpanID()),
			Attributes: make(map[uint32]*idx.AnyValue, numAttrs),
		},
		slabSize: numAttrs,
	}
	start, duration := convertOTelSpanMinimal(&sink, otelspan, otelres, lib, isTopLevel, topLevelByKind, conf, spanAccessor, OTelSpanTagKeys{})
	sink.span.Start = uint64(start)
	sink.span.Duration = uint64(duration)
	convertOTelSpanFull(&sink, otelspan, otelres, lib, conf, spanAccessor, traceIDHex)
	if fields != nil {
		sink.convertedFields(fields)
	}
	var tags IdxSpanSamplingTags
	tags.DecisionMaker, _ = sink.meta("_dd.p.dm")
	tags.Tracestate, _ = sink.meta("w3c.tracestate")
	return sink.finish(), tags
}

// Indexes of the promoted tags in idxSpanSink.
const (
	promotedEnv = iota
	promotedVersion
	promotedComponent
	promotedSpanKind
	numPromoted
)

// promotedIndex returns the index of k in idxSpanSink's promoted tags, or -1 if k is not promoted.
// The promoted tags are the ones the pb-to-idx conversion sets as idx span fields.
func promotedIndex(k string) int {
	switch k {
	case "env":
		return promotedEnv
	case "version":
		return promotedVersion
	case "component":
		return promotedComponent
	case "span.kind":
		return promotedSpanKind
	}
	return -1
}

type promotedMeta struct {
	value string
	ok    bool
}

// idxSpanSink is a spanSink that builds an idx span. It stores the pb Meta and Metrics in a single
// idx attribute map, where a Metrics value takes precedence over a Meta value with the same key,
// as in the pb-to-idx conversion. To keep the pb span model visible to the conversion, it keeps
// the Meta values hidden by a Metrics value in shadowMeta, and the promoted Meta values (which
// become span fields) in promoted. Metrics with a promoted key are dropped by the pb-to-idx
// conversion, but their presence is kept in promotedMetrics since the conversion can read it.
type idxSpanSink struct {
	strings *idx.StringTable
	span    *idx.Span

	svcVal, nameVal, resourceVal, typeVal string
	err                                   int32

	// lastKey and lastRef are the last key looked up and its string ref, 0 if
	// it isn't in the string table: the conversion often reads a key and then
	// sets it, which saves hashing the key twice.
	lastKey string
	lastRef uint32

	promoted        [numPromoted]promotedMeta
	promotedMetrics [numPromoted]bool
	shadowMeta      map[string]string

	// The attribute values are allocated from these slabs, rather than one by
	// one, to save two allocations per attribute.
	slabSize   int
	values     []idx.AnyValue
	stringRefs []idx.AnyValue_StringValueRef
	doubles    []idx.AnyValue_DoubleValue
}

// keyRef adds the key k to the string table and returns its ref.
func (s *idxSpanSink) keyRef(k string) uint32 {
	if s.lastRef != 0 && k == s.lastKey {
		return s.lastRef
	}
	ref := s.strings.Add(k)
	s.lastKey, s.lastRef = k, ref
	return ref
}

// newStringValue returns a new string attribute value of the string ref.
func (s *idxSpanSink) newStringValue(ref uint32) *idx.AnyValue {
	if len(s.stringRefs) == cap(s.stringRefs) {
		s.stringRefs = make([]idx.AnyValue_StringValueRef, 0, s.slabSize)
	}
	s.stringRefs = append(s.stringRefs, idx.AnyValue_StringValueRef{StringValueRef: ref})
	v := s.newValue()
	v.Value = &s.stringRefs[len(s.stringRefs)-1]
	return v
}

// newDoubleValue returns a new double attribute value of v.
func (s *idxSpanSink) newDoubleValue(v float64) *idx.AnyValue {
	if len(s.doubles) == cap(s.doubles) {
		s.doubles = make([]idx.AnyValue_DoubleValue, 0, max(s.slabSize/4, 4))
	}
	s.doubles = append(s.doubles, idx.AnyValue_DoubleValue{DoubleValue: v})
	value := s.newValue()
	value.Value = &s.doubles[len(s.doubles)-1]
	return value
}

// newValue returns a new empty attribute value.
func (s *idxSpanSink) newValue() *idx.AnyValue {
	if len(s.values) == cap(s.values) {
		s.values = make([]idx.AnyValue, 0, s.slabSize)
	}
	s.values = append(s.values, idx.AnyValue{})
	return &s.values[len(s.values)-1]
}

var _ spanSink = (*idxSpanSink)(nil)

func (s *idxSpanSink) service() string      { return s.svcVal }
func (s *idxSpanSink) setService(v string)  { s.svcVal = v }
func (s *idxSpanSink) name() string         { return s.nameVal }
func (s *idxSpanSink) setName(v string)     { s.nameVal = v }
func (s *idxSpanSink) resource() string     { return s.resourceVal }
func (s *idxSpanSink) setResource(v string) { s.resourceVal = v }
func (s *idxSpanSink) spanType() string     { return s.typeVal }
func (s *idxSpanSink) setSpanType(v string) { s.typeVal = v }
func (s *idxSpanSink) setError(v int32)     { s.err = v }

// lookup returns the attribute of k, without adding k to the string table.
func (s *idxSpanSink) lookup(k string) (*idx.AnyValue, bool) {
	ref := s.strings.Lookup(k)
	s.lastKey, s.lastRef = k, ref
	if ref == 0 && k != "" {
		return nil, false
	}
	v, ok := s.span.Attributes[ref]
	return v, ok
}

func (s *idxSpanSink) meta(k string) (string, bool) {
	if i := promotedIndex(k); i >= 0 {
		return s.promoted[i].value, s.promoted[i].ok
	}
	v, ok := s.lookup(k)
	if !ok {
		return "", false
	}
	switch v := v.Value.(type) {
	case *idx.AnyValue_StringValueRef:
		return s.strings.Get(v.StringValueRef), true
	case *idx.AnyValue_DoubleValue:
		if m, ok := s.shadowMeta[k]; ok {
			return m, true
		}
	}
	return "", false
}

func (s *idxSpanSink) setMeta(k, v string) {
	if i := promotedIndex(k); i >= 0 {
		s.promoted[i] = promotedMeta{value: v, ok: true}
		return
	}
	ref := s.keyRef(k)
	if cur, ok := s.span.Attributes[ref]; ok {
		if _, isMetric := cur.Value.(*idx.AnyValue_DoubleValue); isMetric {
			// The Metrics value takes precedence in the idx span.
			if s.shadowMeta == nil {
				s.shadowMeta = make(map[string]string, 1)
			}
			s.shadowMeta[k] = v
			return
		}
	}
	s.span.Attributes[ref] = s.newStringValue(s.strings.Add(v))
}

func (s *idxSpanSink) metric(k string) (float64, bool) {
	if i := promotedIndex(k); i >= 0 {
		return 0, s.promotedMetrics[i]
	}
	v, ok := s.lookup(k)
	if !ok {
		return 0, false
	}
	if d, ok := v.Value.(*idx.AnyValue_DoubleValue); ok {
		return d.DoubleValue, true
	}
	return 0, false
}

func (s *idxSpanSink) setMetric(k string, v float64) {
	if i := promotedIndex(k); i >= 0 {
		s.promotedMetrics[i] = true
		return
	}
	ref := s.keyRef(k)
	if cur, ok := s.span.Attributes[ref]; ok {
		switch cur := cur.Value.(type) {
		case *idx.AnyValue_DoubleValue:
			cur.DoubleValue = v
			return
		case *idx.AnyValue_StringValueRef:
			// Keep the Meta value visible to the conversion, behind the Metrics value.
			if s.shadowMeta == nil {
				s.shadowMeta = make(map[string]string, 1)
			}
			s.shadowMeta[k] = s.strings.Get(cur.StringValueRef)
		}
	}
	s.span.Attributes[ref] = s.newDoubleValue(v)
}

func (s *idxSpanSink) setEvents(events ptrace.SpanEventSlice) {
	s.span.Events = otelSpanEventsToIdx(events, s.strings)
}

func (s *idxSpanSink) setLinks(links ptrace.SpanLinkSlice) {
	s.span.Links = otelSpanLinksToIdx(links, s.strings)
}

// convertedFields records the promoted chunk and payload values of the span in fields, with the
// same lookups as the pb-to-idx conversion over the pb span Meta and Metrics.
func (s *idxSpanSink) convertedFields(fields *idx.SpanConvertedFields) {
	if p, ok := s.metric(string(semantics.ConceptSamplingPriority)); ok {
		fields.SamplingPriority = int32(p)
	}
	metaRef := func(k semantics.Concept, ref *uint32) {
		if *ref != 0 {
			return
		}
		if v, _ := s.meta(string(k)); v != "" {
			*ref = s.strings.Add(v)
		}
	}
	metaRef(semantics.ConceptDDEnv, &fields.EnvRef)
	metaRef(semantics.ConceptDDHostname, &fields.HostnameRef)
	metaRef(semantics.ConceptDDVersion, &fields.AppVersionRef)
	metaRef(semantics.ConceptDDGitCommitSHA, &fields.GitCommitShaRef)
	if dm, _ := s.meta(string(semantics.ConceptDDDecisionMaker)); dm != "" && fields.SamplingMechanism == 0 {
		dm, _ = strings.CutPrefix(dm, "-")
		samplingMechanism, err := strconv.ParseUint(dm, 10, 32)
		if err != nil {
			log.Debugf("Found invalid sampling mechanism %s: %v, Decision maker will be ignored", dm, err)
		}
		fields.SamplingMechanism = uint32(samplingMechanism)
	}
	metaRef(semantics.ConceptDDAPMMode, &fields.APMModeRef)
	metaRef(semantics.ConceptDDOrigin, &fields.OriginRef)
}

// finish sets the remaining idx span fields and returns the idx span.
func (s *idxSpanSink) finish() *idx.InternalSpan {
	sp := s.span
	sp.ServiceRef = s.strings.Add(s.svcVal)
	sp.NameRef = s.strings.Add(s.nameVal)
	sp.ResourceRef = s.strings.Add(s.resourceVal)
	sp.TypeRef = s.strings.Add(s.typeVal)
	sp.Error = s.err > 0
	sp.EnvRef = s.strings.Add(s.promoted[promotedEnv].value)
	sp.VersionRef = s.strings.Add(s.promoted[promotedVersion].value)
	sp.ComponentRef = s.strings.Add(s.promoted[promotedComponent].value)
	sp.Kind = idxSpanKind(s.promoted[promotedSpanKind].value)
	return idx.NewInternalSpan(s.strings, sp)
}

// idxSpanKind returns the idx span kind of the Datadog span.kind tag value k. As in the pb-to-idx
// conversion, a missing or unknown kind is internal.
func idxSpanKind(k string) idx.SpanKind {
	switch k {
	case "server":
		return idx.SpanKind_SPAN_KIND_SERVER
	case "client":
		return idx.SpanKind_SPAN_KIND_CLIENT
	case "producer":
		return idx.SpanKind_SPAN_KIND_PRODUCER
	case "consumer":
		return idx.SpanKind_SPAN_KIND_CONSUMER
	default:
		return idx.SpanKind_SPAN_KIND_INTERNAL
	}
}

// otelSpanEventsToIdx converts OTel span events into idx span events, like OTelSpanEventsToDD.
func otelSpanEventsToIdx(events ptrace.SpanEventSlice, strings *idx.StringTable) []*idx.SpanEvent {
	idxEvents := make([]*idx.SpanEvent, 0, events.Len())
	for i := 0; i < events.Len(); i++ {
		e := events.At(i)
		idxEvent := &idx.SpanEvent{
			Time:       uint64(e.Timestamp()),
			NameRef:    strings.Add(e.Name()),
			Attributes: make(map[uint32]*idx.AnyValue, e.Attributes().Len()),
		}
		e.Attributes().Range(func(k string, v pcommon.Value) bool {
			setIdxEventAttribute(idxEvent.Attributes, k, v, strings)
			return true
		})
		idxEvents = append(idxEvents, idxEvent)
	}
	return idxEvents
}

// setIdxEventAttribute sets the OTel attribute k=v in the idx span event attributes, with the same
// rules as setEventAttribute.
func setIdxEventAttribute(attrs map[uint32]*idx.AnyValue, k string, v pcommon.Value, strings *idx.StringTable) {
	switch v.Type() {
	case pcommon.ValueTypeStr, pcommon.ValueTypeBool, pcommon.ValueTypeInt, pcommon.ValueTypeDouble:
		attrs[strings.Add(k)] = idxScalar(v, strings)
	case pcommon.ValueTypeBytes:
		attrs[strings.Add(k)] = &idx.AnyValue{Value: &idx.AnyValue_StringValueRef{StringValueRef: strings.Add(v.AsString())}}
	case pcommon.ValueTypeSlice:
		s := v.Slice()
		if arr, ok := idxEventAttributeArray(s, strings); ok {
			attrs[strings.Add(k)] = &idx.AnyValue{Value: &idx.AnyValue_ArrayValue{ArrayValue: arr}}
			return
		}
		for i := 0; i < s.Len(); i++ {
			setIdxEventAttribute(attrs, k+"."+strconv.Itoa(i), s.At(i), strings)
		}
	case pcommon.ValueTypeMap:
		v.Map().Range(func(mk string, mv pcommon.Value) bool {
			setIdxEventAttribute(attrs, k+"."+mk, mv, strings)
			return true
		})
	}
}

// idxScalar converts the string, bool, int or double v into an idx value.
func idxScalar(v pcommon.Value, strings *idx.StringTable) *idx.AnyValue {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		return &idx.AnyValue{Value: &idx.AnyValue_StringValueRef{StringValueRef: strings.Add(v.Str())}}
	case pcommon.ValueTypeBool:
		return &idx.AnyValue{Value: &idx.AnyValue_BoolValue{BoolValue: v.Bool()}}
	case pcommon.ValueTypeInt:
		return &idx.AnyValue{Value: &idx.AnyValue_IntValue{IntValue: v.Int()}}
	case pcommon.ValueTypeDouble:
		return &idx.AnyValue{Value: &idx.AnyValue_DoubleValue{DoubleValue: v.Double()}}
	}
	return nil
}

// idxEventAttributeArray converts s into an idx array, with the same rules as eventAttributeArray.
func idxEventAttributeArray(s pcommon.Slice, strings *idx.StringTable) (*idx.ArrayValue, bool) {
	if s.Len() == 0 {
		return &idx.ArrayValue{Values: []*idx.AnyValue{}}, true
	}
	typ := s.At(0).Type()
	switch typ {
	case pcommon.ValueTypeStr, pcommon.ValueTypeBool, pcommon.ValueTypeInt, pcommon.ValueTypeDouble:
	default:
		return nil, false
	}
	for i := 1; i < s.Len(); i++ {
		if s.At(i).Type() != typ {
			return nil, false
		}
	}
	values := make([]*idx.AnyValue, 0, s.Len())
	for i := 0; i < s.Len(); i++ {
		values = append(values, idxScalar(s.At(i), strings))
	}
	return &idx.ArrayValue{Values: values}, true
}

// otelSpanLinksToIdx converts OTel span links into idx span links, like OTelSpanLinksToDD.
func otelSpanLinksToIdx(links ptrace.SpanLinkSlice, strings *idx.StringTable) []*idx.SpanLink {
	idxLinks := make([]*idx.SpanLink, 0, links.Len())
	for i := 0; i < links.Len(); i++ {
		l := links.At(i)
		traceID := l.TraceID()
		idxLink := &idx.SpanLink{
			TraceID:       append([]byte(nil), traceID[:]...),
			SpanID:        OTelSpanIDToUint64(l.SpanID()),
			TracestateRef: strings.Add(l.TraceState().AsRaw()),
			Attributes:    make(map[uint32]*idx.AnyValue, l.Attributes().Len()),
		}
		if flags := l.Flags(); flags != 0 {
			// Only the low 8 bits are W3C trace flags; OTLP uses bits 8 and 9 for is_remote. The
			// high bit marks the flags as set, telling "not sampled" apart from "unknown".
			idxLink.Flags = flags&0xff | 1<<31
		}
		l.Attributes().Range(func(k string, v pcommon.Value) bool {
			flattenIdxLinkAttribute(idxLink.Attributes, k, v, strings)
			return true
		})
		idxLinks = append(idxLinks, idxLink)
	}
	return idxLinks
}

// flattenIdxLinkAttribute sets the OTel attribute k=v in the string-valued idx span link
// attributes, with the same rules as flattenLinkAttribute.
func flattenIdxLinkAttribute(attrs map[uint32]*idx.AnyValue, k string, v pcommon.Value, strings *idx.StringTable) {
	switch v.Type() {
	case pcommon.ValueTypeSlice:
		s := v.Slice()
		for i := 0; i < s.Len(); i++ {
			flattenIdxLinkAttribute(attrs, k+"."+strconv.Itoa(i), s.At(i), strings)
		}
	case pcommon.ValueTypeMap:
		v.Map().Range(func(mk string, mv pcommon.Value) bool {
			flattenIdxLinkAttribute(attrs, k+"."+mk, mv, strings)
			return true
		})
	default:
		attrs[strings.Add(k)] = &idx.AnyValue{Value: &idx.AnyValue_StringValueRef{StringValueRef: strings.Add(v.AsString())}}
	}
}

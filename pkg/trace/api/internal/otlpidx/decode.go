// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otlpidx decodes OTLP ExportTraceServiceRequest protobuf bytes directly into the
// idx (V1) internal trace representation, without going through pdata.
//
// PROTOTYPE (see otlp-to-v1-plan.md): the Datadog semantic mapping in semantics.go covers the
// default ReceiveResourceSpansV2 configuration only. Hostname/source resolution, container tags,
// peer tags, top-level-by-kind and tracestate sample rates are not implemented yet.
package otlpidx

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

// maxAnyValueDepth bounds recursion through nested ArrayValue / KeyValueList attributes.
const maxAnyValueDepth = 32

var errMaxDepth = errors.New("otlpidx: attribute nesting exceeds maximum depth")

// Field numbers from opentelemetry/proto (collector/trace/v1, trace/v1, common/v1, resource/v1).
const (
	fExportResourceSpans = 1

	fResourceSpansResource   = 1
	fResourceSpansScopeSpans = 2

	fResourceAttributes = 1

	fScopeSpansScope = 1
	fScopeSpansSpans = 2

	fScopeName       = 1
	fScopeVersion    = 2
	fScopeAttributes = 3

	fSpanTraceID      = 1
	fSpanSpanID       = 2
	fSpanTraceState   = 3
	fSpanParentSpanID = 4
	fSpanName         = 5
	fSpanKind         = 6
	fSpanStart        = 7
	fSpanEnd          = 8
	fSpanAttributes   = 9
	fSpanEvents       = 11
	fSpanLinks        = 13
	fSpanStatus       = 15

	fEventTime       = 1
	fEventName       = 2
	fEventAttributes = 3

	fLinkTraceID    = 1
	fLinkSpanID     = 2
	fLinkTraceState = 3
	fLinkAttributes = 4
	fLinkFlags      = 6

	fStatusMessage = 2
	fStatusCode    = 3

	fKeyValueKey   = 1
	fKeyValueValue = 2

	fAnyString = 1
	fAnyBool   = 2
	fAnyInt    = 3
	fAnyDouble = 4
	fAnyArray  = 5
	fAnyKvlist = 6
	fAnyBytes  = 7

	fArrayValues  = 1
	fKvlistValues = 1
)

// field is a single decoded protobuf field. For bytes-typed fields val is set; for varint
// and fixed fields u64 holds the value.
type field struct {
	num protowire.Number
	typ protowire.Type
	val []byte
	u64 uint64
}

// next consumes one field from b. Unknown wire types are rejected, unknown field numbers are
// returned to the caller to ignore.
func next(b []byte) (field, []byte, error) {
	num, typ, n := protowire.ConsumeTag(b)
	if n < 0 {
		return field{}, nil, protowire.ParseError(n)
	}
	b = b[n:]
	f := field{num: num, typ: typ}
	switch typ {
	case protowire.VarintType:
		f.u64, n = protowire.ConsumeVarint(b)
	case protowire.Fixed64Type:
		f.u64, n = protowire.ConsumeFixed64(b)
	case protowire.Fixed32Type:
		var v uint32
		v, n = protowire.ConsumeFixed32(b)
		f.u64 = uint64(v)
	case protowire.BytesType:
		f.val, n = protowire.ConsumeBytes(b)
	default:
		n = protowire.ConsumeFieldValue(num, typ, b)
	}
	if n < 0 {
		return field{}, nil, protowire.ParseError(n)
	}
	return f, b[n:], nil
}

// One-byte tags (field number << 3 | wire type) of the fields on the attribute decoding path.
const (
	tagAnyString = fAnyString<<3 | byte(protowire.BytesType)
	tagAnyBool   = fAnyBool<<3 | byte(protowire.VarintType)
	tagAnyInt    = fAnyInt<<3 | byte(protowire.VarintType)
)

// shortBytesField decodes a length-delimited field with a one-byte tag and a length of at most
// two varint bytes (under 16KiB), the shape of nearly every field on the attribute decoding path,
// without the generic decoding in next. ok is false for any other field, which the caller then
// decodes with next.
//
// It is kept small enough to be inlined into the decoding loops.
func shortBytesField(b []byte) (num protowire.Number, val, rest []byte, ok bool) {
	// A tag under 0x80 (one byte) with the bytes wire type. Requiring three bytes keeps the
	// function small; a two-byte field (an empty value at the end of a message) uses next.
	if len(b) < 3 || b[0]&0x87 != byte(protowire.BytesType) {
		return
	}
	hdr, n := 2, int(b[1])
	if n >= 0x80 {
		if b[2] >= 0x80 {
			return
		}
		hdr, n = 3, n&0x7f|int(b[2])<<7
	}
	if hdr+n > len(b) {
		return
	}
	return protowire.Number(b[0] >> 3), b[hdr : hdr+n], b[hdr+n:], true
}

// Options controls decoding.
type Options struct {
	// Conf is the trace-agent configuration: fallback hostname, attributes translator, OTLP
	// sampling and feature flags. Required. Check Supported(Conf) before decoding.
	Conf *config.AgentConfig
	// HostFromAttributesHandler is passed to the attributes translator when resolving the source.
	HostFromAttributesHandler attributes.HostFromAttributesHandler
	// ResourceAttributesOnPayload stores resource attributes once in the payload's Attributes
	// instead of copying them into every span. The V1 intake doesn't yet propagate payload
	// attributes to spans (see otlp-to-v1-plan.md), so this is off by default.
	ResourceAttributesOnPayload bool
	// SkipSemantics disables the Datadog semantic mapping, leaving only the raw decode. Used to
	// benchmark the decode cost in isolation.
	SkipSemantics bool
}

// Payload is the decoded form of one ResourceSpans.
type Payload struct {
	TracerPayload *idx.InternalTracerPayload
	// Source is the host or serverless source resolved from the resource.
	Source source.Source
	// StatsComputed is the value of the _dd.stats_computed resource attribute, or nil when it is
	// absent and the request header decides.
	StatsComputed *bool
	// SpanCount is the number of spans decoded.
	SpanCount int
}

// Decode decodes a serialized ExportTraceServiceRequest into one Payload per ResourceSpans,
// mirroring the existing receiver which treats each ResourceSpans as a separate payload. Strings
// are copied into each payload's StringTable, so the result does not alias b.
func Decode(ctx context.Context, b []byte, opts Options) ([]Payload, error) {
	var out []Payload
	for len(b) > 0 {
		f, rest, err := next(b)
		if err != nil {
			return nil, err
		}
		b = rest
		if f.num != fExportResourceSpans || f.typ != protowire.BytesType {
			continue
		}
		p, err := decodeResourceSpans(ctx, f.val, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

type decoder struct {
	opts    Options
	strings *idx.StringTable
	slab    slab
	// resAttrs are the resource attributes. Span attributes take precedence over them.
	resAttrs map[uint32]*idx.AnyValue
	// resRenames are the resource attribute keys that the Datadog key mapping renames or drops,
	// applied to the payload attributes when resource attributes are stored on the payload.
	resRenames []rename
	// resMerge are the resource attributes copied onto spans that don't have the key.
	resMerge []resMerge
	// renames collects the current span's attribute keys to rename or drop once semantic
	// lookups, which use the original OTel keys, are done.
	renames []rename
	// resKeys and spanKeys filter the resource's and the current span's attribute keys, and
	// keys points to the one being decoded into. Semantic lookups use their union, lookupKeys.
	resKeys, spanKeys, lookupKeys keyFilter
	keys                          *keyFilter
	// scopeKeys receives the keys of scope attributes, which semantic lookups don't read.
	scopeKeys keyFilter
	// keptKeys is a bitset of string references known not to be renamed by the key mapping.
	keptKeys []uint64
	// lastAttrs is the previous span's final attribute count, used to size the next span's map.
	lastAttrs int
	// Initial backing arrays for the slices above, which stay small for most payloads.
	resRenamesBuf, renamesBuf [4]rename
	keptKeysBuf               [4]uint64
	// res holds resource-level values computed once per payload.
	res  resourceInfo
	refs constRefs
	// lastTraceID is the previous span's trace ID, and lastTraceIDRef its hex string reference.
	lastTraceID    [16]byte
	lastTraceIDRef uint32
	spanCount      int
	// topLevelByKind and scopeConvention cache the enable_otlp_compute_top_level_by_span_kind and
	// (inverted) disable_otel_scope_convention feature flags.
	topLevelByKind, scopeConvention bool
	// lastTracestate is the previous span's raw tracestate, and lastSampleRate the sampling
	// probability decoded from it (ok false if none).
	lastTracestate   string
	lastSampleRate   float64
	lastSampleRateOK bool
	// priorities are user-set sampling priorities by trace ID.
	priorities map[[16]byte]int32
	chunks     map[[16]byte]*idx.InternalTraceChunk
	order      []*idx.InternalTraceChunk
}

// rawSpan carries the OTLP fields that don't map onto idx.Span directly.
type rawSpan struct {
	traceID       [16]byte
	statusCode    uint64
	statusMsgRef  uint32
	tracestateRef uint32
	hasException  bool
}

// stringTableHint estimates the number of distinct strings in a ResourceSpans of n bytes, so the
// string table doesn't repeatedly grow on small payloads. Keys and values repeat across spans,
// so the count grows much slower than the payload and is capped.
func stringTableHint(n int) int {
	return min(max(n/32, 32), 128)
}

func decodeResourceSpans(ctx context.Context, b []byte, opts Options) (Payload, error) {
	d := &decoder{
		opts:    opts,
		strings: idx.NewStringTableWithCapacity(stringTableHint(len(b))),
		chunks:  make(map[[16]byte]*idx.InternalTraceChunk),
	}
	d.resRenames = d.resRenamesBuf[:0]
	d.renames = d.renamesBuf[:0]
	d.keptKeys = d.keptKeysBuf[:0]
	if opts.Conf != nil {
		d.topLevelByKind = opts.Conf.HasFeature("enable_otlp_compute_top_level_by_span_kind")
		d.scopeConvention = !opts.Conf.HasFeature("disable_otel_scope_convention")
	}
	// Pass 1: protobuf permits fields in any order, so locate the resource before decoding
	// spans, which need the resource attributes.
	resource, err := lastBytesField(b, fResourceSpansResource)
	if err != nil {
		return Payload{}, err
	}
	if err := d.decodeResource(ctx, resource); err != nil {
		return Payload{}, err
	}
	// Pass 2: scope spans.
	for len(b) > 0 {
		f, rest, err := next(b)
		if err != nil {
			return Payload{}, err
		}
		b = rest
		if f.num != fResourceSpansScopeSpans || f.typ != protowire.BytesType {
			continue
		}
		if err := d.decodeScopeSpans(f.val); err != nil {
			return Payload{}, err
		}
	}
	tp := &idx.InternalTracerPayload{
		Strings: d.strings,
		Chunks:  d.order,
	}
	if d.opts.ResourceAttributesOnPayload {
		tp.Attributes = d.resAttrs
	}
	d.finishPayload(tp)
	return Payload{
		TracerPayload: tp,
		Source:        d.res.source,
		StatsComputed: d.res.statsComputed,
		SpanCount:     d.spanCount,
	}, nil
}

func (d *decoder) decodeResource(ctx context.Context, resource []byte) error {
	b := resource
	d.resAttrs = make(map[uint32]*idx.AnyValue)
	d.keys = &d.resKeys
	for len(b) > 0 {
		f, rest, err := next(b)
		if err != nil {
			return err
		}
		b = rest
		if f.num != fResourceAttributes || f.typ != protowire.BytesType {
			continue
		}
		k, v, err := d.decodeKeyValue(f.val, 0)
		if err != nil {
			return err
		}
		d.resAttrs[k] = v
		if r, ok := d.renameFor(k); ok {
			d.resRenames = append(d.resRenames, r)
		}
	}
	if d.opts.SkipSemantics {
		d.spanDefaults()
		for k, v := range d.resAttrs {
			d.resMerge = append(d.resMerge, resMerge{key: k, val: v})
		}
		return nil
	}
	// Like receiveResourceSpansV2, payload-level values are read from the full resource, and
	// span-level defaults from the resource minus the attributes consumed as container tags.
	d.payloadSemantics()
	if err := d.resourceSource(ctx, resource); err != nil {
		return err
	}
	d.spanDefaults()
	return nil
}

// lastBytesField returns the value of the last bytes-typed field num in the message b, so that
// fields which must be decoded first can be found without collecting the others.
func lastBytesField(b []byte, num protowire.Number) ([]byte, error) {
	var val []byte
	for len(b) > 0 {
		f, rest, err := next(b)
		if err != nil {
			return nil, err
		}
		b = rest
		if f.num == num && f.typ == protowire.BytesType {
			val = f.val
		}
	}
	return val, nil
}

func (d *decoder) decodeScopeSpans(b []byte) error {
	scope, err := lastBytesField(b, fScopeSpansScope)
	if err != nil {
		return err
	}
	var lib scopeInfo
	for len(scope) > 0 {
		f, rest, err := next(scope)
		if err != nil {
			return err
		}
		scope = rest
		if f.typ != protowire.BytesType {
			continue
		}
		switch f.num {
		case fScopeName:
			lib.nameRef = d.strings.AddBytes(f.val)
		case fScopeVersion:
			lib.versionRef = d.strings.AddBytes(f.val)
		case fScopeAttributes:
			if !d.opts.SkipSemantics {
				if err := d.decodeScopeAttribute(&lib, f.val); err != nil {
					return err
				}
			}
		}
	}
	for len(b) > 0 {
		f, rest, err := next(b)
		if err != nil {
			return err
		}
		b = rest
		if f.num != fScopeSpansSpans || f.typ != protowire.BytesType {
			continue
		}
		span, raw, err := d.decodeSpan(f.val)
		if err != nil {
			return err
		}
		if d.opts.SkipSemantics {
			span.ServiceRef = d.res.serviceRef
			span.ResourceRef = span.NameRef
		} else {
			d.spanSemantics(span, &raw, lib)
		}
		if !d.opts.ResourceAttributesOnPayload {
			d.mergeResource(span.Attributes)
		}
		if !d.opts.SkipSemantics {
			d.applyScope(span, &lib)
		}
		d.spanCount++
		d.lastAttrs = len(span.Attributes)
		d.addSpan(raw.traceID, idx.NewInternalSpan(d.strings, span))
	}
	return nil
}

func (d *decoder) addSpan(traceID [16]byte, span *idx.InternalSpan) {
	c, ok := d.chunks[traceID]
	if !ok {
		c = idx.NewInternalTraceChunk(d.strings, 0, "", nil, nil, false, append([]byte(nil), traceID[:]...), 0)
		d.chunks[traceID] = c
		d.order = append(d.order, c)
	}
	c.Spans = append(c.Spans, span)
}

// Slabs batch the allocation of AnyValues and their oneof wrappers, as the v0.5 decoder does,
// cutting two allocations per attribute to roughly zero. Every handed-out pointer is still
// distinct, so in-place mutation stays safe. Slabs start small and double, so small payloads
// don't pay for large slabs.
const (
	minSlabSize = 16
	maxSlabSize = 256
)

type slabOf[T any] struct {
	free []T
	next int
}

func (s *slabOf[T]) take() *T {
	if len(s.free) == 0 {
		if s.next == 0 {
			s.next = minSlabSize
		}
		s.free = make([]T, s.next)
		s.next = min(s.next*2, maxSlabSize)
	}
	v := &s.free[0]
	s.free = s.free[1:]
	return v
}

type slab struct {
	values  slabOf[idx.AnyValue]
	strs    slabOf[idx.AnyValue_StringValueRef]
	ints    slabOf[idx.AnyValue_IntValue]
	doubles slabOf[idx.AnyValue_DoubleValue]
	bools   slabOf[idx.AnyValue_BoolValue]
}

func (d *decoder) newAnyValue() *idx.AnyValue { return d.slab.values.take() }

func (d *decoder) strRef(ref uint32) *idx.AnyValue_StringValueRef {
	w := d.slab.strs.take()
	w.StringValueRef = ref
	return w
}

func (d *decoder) stringValue(ref uint32) *idx.AnyValue {
	v := d.newAnyValue()
	v.Value = d.strRef(ref)
	return v
}

func (d *decoder) intValue(i int64) *idx.AnyValue {
	v := d.newAnyValue()
	w := d.slab.ints.take()
	w.IntValue = i
	v.Value = w
	return v
}

func (d *decoder) doubleValue(f float64) *idx.AnyValue {
	v := d.newAnyValue()
	w := d.slab.doubles.take()
	w.DoubleValue = f
	v.Value = w
	return v
}

// cloneValue returns a copy of v that shares no mutable state with it, so per-span copies of
// resource attributes can be mutated independently (e.g. by string table compaction).
func (d *decoder) cloneValue(v *idx.AnyValue) *idx.AnyValue {
	switch x := v.Value.(type) {
	case *idx.AnyValue_StringValueRef:
		return d.stringValue(x.StringValueRef)
	case *idx.AnyValue_IntValue:
		return d.intValue(x.IntValue)
	case *idx.AnyValue_DoubleValue:
		return d.doubleValue(x.DoubleValue)
	case *idx.AnyValue_BoolValue:
		c := d.newAnyValue()
		w := d.slab.bools.take()
		w.BoolValue = x.BoolValue
		c.Value = w
		return c
	case *idx.AnyValue_BytesValue:
		c := d.newAnyValue()
		c.Value = &idx.AnyValue_BytesValue{BytesValue: x.BytesValue}
		return c
	case *idx.AnyValue_ArrayValue:
		arr := &idx.ArrayValue{Values: make([]*idx.AnyValue, len(x.ArrayValue.Values))}
		for i, e := range x.ArrayValue.Values {
			arr.Values[i] = d.cloneValue(e)
		}
		c := d.newAnyValue()
		c.Value = &idx.AnyValue_ArrayValue{ArrayValue: arr}
		return c
	case *idx.AnyValue_KeyValueList:
		kvl := &idx.KeyValueList{KeyValues: make([]*idx.KeyValue, len(x.KeyValueList.KeyValues))}
		for i, kv := range x.KeyValueList.KeyValues {
			kvl.KeyValues[i] = &idx.KeyValue{Key: kv.Key, Value: d.cloneValue(kv.Value)}
		}
		c := d.newAnyValue()
		c.Value = &idx.AnyValue_KeyValueList{KeyValueList: kvl}
		return c
	}
	return d.stringValue(0)
}

// resMerge is a resource attribute copied onto spans under key, as a copy of val.
type resMerge struct {
	key uint32
	val *idx.AnyValue
}

// mergeResource copies the resource attributes onto a span's attributes where the span doesn't
// have the key. Like today's conversion, this runs after the span's semantic mapping.
func (d *decoder) mergeResource(attrs map[uint32]*idx.AnyValue) {
	for _, m := range d.resMerge {
		if _, ok := attrs[m.key]; !ok {
			attrs[m.key] = d.cloneValue(m.val)
		}
	}
}

// isComposite reports whether v is an array, map or bytes value.
func isComposite(v *idx.AnyValue) bool {
	switch v.Value.(type) {
	case *idx.AnyValue_ArrayValue, *idx.AnyValue_KeyValueList, *idx.AnyValue_BytesValue:
		return true
	}
	return false
}

// minAttrsHint is the attribute map size hint for the first span of a payload.
const minAttrsHint = 16

// attrsHint sizes a span's attribute map. Spans in a payload tend to look alike, so the previous
// span's final attribute count (after semantic mapping) avoids growing the map while it's filled.
func (d *decoder) attrsHint() int {
	if d.lastAttrs > 0 {
		return d.lastAttrs
	}
	if !d.opts.ResourceAttributesOnPayload {
		return minAttrsHint + len(d.resAttrs)
	}
	return minAttrsHint
}

func spanIDFromBytes(b []byte) uint64 {
	if len(b) != 8 {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}

func (d *decoder) decodeSpan(b []byte) (*idx.Span, rawSpan, error) {
	var raw rawSpan
	var start, end uint64
	d.renames = d.renames[:0]
	d.spanKeys = keyFilter{}
	d.keys = &d.spanKeys
	span := &idx.Span{
		Attributes: make(map[uint32]*idx.AnyValue, d.attrsHint()),
	}
	for len(b) > 0 {
		var f field
		if num, val, rest, ok := shortBytesField(b); ok {
			f, b = field{num: num, typ: protowire.BytesType, val: val}, rest
		} else {
			var err error
			if f, rest, err = next(b); err != nil {
				return nil, raw, err
			}
			b = rest
		}
		switch f.num {
		case fSpanTraceID:
			if len(f.val) != 16 {
				return nil, raw, fmt.Errorf("otlpidx: invalid trace_id length %d", len(f.val))
			}
			copy(raw.traceID[:], f.val)
		case fSpanSpanID:
			span.SpanID = spanIDFromBytes(f.val)
		case fSpanParentSpanID:
			span.ParentID = spanIDFromBytes(f.val)
		case fSpanTraceState:
			if len(f.val) > 0 {
				d.keys.add(keySig("w3c.tracestate"))
				raw.tracestateRef = d.strings.AddBytes(f.val)
				span.Attributes[d.strings.Add("w3c.tracestate")] = d.stringValue(raw.tracestateRef)
			}
		case fSpanName:
			span.NameRef = d.strings.AddBytes(f.val)
		case fSpanKind:
			span.Kind = idx.SpanKind(f.u64)
		case fSpanStart:
			start = f.u64
		case fSpanEnd:
			end = f.u64
		case fSpanAttributes:
			k, v, err := d.decodeKeyValue(f.val, 0)
			if err != nil {
				return nil, raw, err
			}
			if !d.opts.SkipSemantics && isComposite(v) {
				// Like today's conversion, store arrays, maps and bytes as their string form.
				s, err := attrAsString(f.val)
				if err != nil {
					return nil, raw, err
				}
				v = d.stringValue(d.strings.Add(s))
			}
			span.Attributes[k] = v
			if r, ok := d.renameFor(k); ok {
				d.renames = append(d.renames, r)
			}
		case fSpanEvents:
			ev, isException, err := d.decodeEvent(f.val)
			if err != nil {
				return nil, raw, err
			}
			raw.hasException = raw.hasException || isException
			span.Events = append(span.Events, ev)
		case fSpanLinks:
			l, err := d.decodeLink(f.val)
			if err != nil {
				return nil, raw, err
			}
			span.Links = append(span.Links, l)
		case fSpanStatus:
			msg := f.val
			for len(msg) > 0 {
				sf, srest, err := next(msg)
				if err != nil {
					return nil, raw, err
				}
				msg = srest
				switch sf.num {
				case fStatusCode:
					raw.statusCode = sf.u64
				case fStatusMessage:
					if len(sf.val) > 0 {
						raw.statusMsgRef = d.strings.AddBytes(sf.val)
					}
				}
			}
		}
	}
	span.Start = start
	if end > start {
		span.Duration = end - start
	}
	return span, raw, nil
}

func (d *decoder) decodeEvent(b []byte) (*idx.SpanEvent, bool, error) {
	ev := &idx.SpanEvent{}
	isException := false
	for len(b) > 0 {
		f, rest, err := next(b)
		if err != nil {
			return nil, false, err
		}
		b = rest
		switch f.num {
		case fEventTime:
			ev.Time = f.u64
		case fEventName:
			ev.NameRef = d.strings.AddBytes(f.val)
			isException = string(f.val) == "exception"
		case fEventAttributes:
			k, v, err := d.decodeKeyValue(f.val, 0)
			if err != nil {
				return nil, false, err
			}
			if ev.Attributes == nil {
				ev.Attributes = make(map[uint32]*idx.AnyValue)
			}
			ev.Attributes[k] = v
		}
	}
	return ev, isException, nil
}

func (d *decoder) decodeLink(b []byte) (*idx.SpanLink, error) {
	l := &idx.SpanLink{}
	for len(b) > 0 {
		f, rest, err := next(b)
		if err != nil {
			return nil, err
		}
		b = rest
		switch f.num {
		case fLinkTraceID:
			l.TraceID = append([]byte(nil), f.val...)
		case fLinkSpanID:
			l.SpanID = spanIDFromBytes(f.val)
		case fLinkTraceState:
			l.TracestateRef = d.strings.AddBytes(f.val)
		case fLinkFlags:
			l.Flags = uint32(f.u64)
		case fLinkAttributes:
			k, v, err := d.decodeKeyValue(f.val, 0)
			if err != nil {
				return nil, err
			}
			if l.Attributes == nil {
				l.Attributes = make(map[uint32]*idx.AnyValue)
			}
			l.Attributes[k] = v
		}
	}
	return l, nil
}

func (d *decoder) decodeKeyValue(b []byte, depth int) (uint32, *idx.AnyValue, error) {
	var key uint32
	var val *idx.AnyValue
	for len(b) > 0 {
		if num, fv, rest, ok := shortBytesField(b); ok {
			b = rest
			switch num {
			case fKeyValueKey:
				key = d.strings.AddBytes(fv)
				d.keys.add(keySig(fv))
			case fKeyValueValue:
				var err error
				if val, err = d.decodeAnyValue(fv, depth); err != nil {
					return 0, nil, err
				}
			}
			continue
		}
		f, rest, err := next(b)
		if err != nil {
			return 0, nil, err
		}
		b = rest
		switch f.num {
		case fKeyValueKey:
			key = d.strings.AddBytes(f.val)
			d.keys.add(keySig(f.val))
		case fKeyValueValue:
			if val, err = d.decodeAnyValue(f.val, depth); err != nil {
				return 0, nil, err
			}
		}
	}
	if val == nil {
		// An unset AnyValue is an empty value; represent it as the empty string.
		val = d.stringValue(0)
	}
	return key, val, nil
}

func (d *decoder) decodeAnyValue(b []byte, depth int) (*idx.AnyValue, error) {
	if depth > maxAnyValueDepth {
		return nil, errMaxDepth
	}
	v := d.newAnyValue()
	for len(b) > 0 {
		// Fast paths for the common scalar members, falling back to next for anything else.
		if b[0] == tagAnyString {
			if _, s, rest, ok := shortBytesField(b); ok {
				v.Value = d.strRef(d.strings.AddBytes(s))
				b = rest
				continue
			}
		} else if len(b) >= 2 && b[1] < 0x80 {
			switch b[0] {
			case tagAnyInt:
				w := d.slab.ints.take()
				w.IntValue = int64(b[1])
				v.Value = w
				b = b[2:]
				continue
			case tagAnyBool:
				w := d.slab.bools.take()
				w.BoolValue = b[1] != 0
				v.Value = w
				b = b[2:]
				continue
			}
		}
		f, rest, err := next(b)
		if err != nil {
			return nil, err
		}
		b = rest
		// Last oneof member wins, matching protobuf semantics.
		switch f.num {
		case fAnyString:
			v.Value = d.strRef(d.strings.AddBytes(f.val))
		case fAnyBool:
			w := d.slab.bools.take()
			w.BoolValue = f.u64 != 0
			v.Value = w
		case fAnyInt:
			w := d.slab.ints.take()
			w.IntValue = int64(f.u64)
			v.Value = w
		case fAnyDouble:
			w := d.slab.doubles.take()
			w.DoubleValue = math.Float64frombits(f.u64)
			v.Value = w
		case fAnyBytes:
			v.Value = &idx.AnyValue_BytesValue{BytesValue: append([]byte(nil), f.val...)}
		case fAnyArray:
			arr := &idx.ArrayValue{}
			inner := f.val
			for len(inner) > 0 {
				af, arest, err := next(inner)
				if err != nil {
					return nil, err
				}
				inner = arest
				if af.num != fArrayValues {
					continue
				}
				av, err := d.decodeAnyValue(af.val, depth+1)
				if err != nil {
					return nil, err
				}
				arr.Values = append(arr.Values, av)
			}
			v.Value = &idx.AnyValue_ArrayValue{ArrayValue: arr}
		case fAnyKvlist:
			kvl := &idx.KeyValueList{}
			inner := f.val
			for len(inner) > 0 {
				kf, krest, err := next(inner)
				if err != nil {
					return nil, err
				}
				inner = krest
				if kf.num != fKvlistValues {
					continue
				}
				k, kv, err := d.decodeKeyValue(kf.val, depth+1)
				if err != nil {
					return nil, err
				}
				kvl.KeyValues = append(kvl.KeyValues, &idx.KeyValue{Key: k, Value: kv})
			}
			v.Value = &idx.AnyValue_KeyValueList{KeyValueList: kvl}
		}
	}
	if v.Value == nil {
		// An AnyValue with no member set is an empty value.
		v.Value = d.strRef(0)
	}
	return v, nil
}

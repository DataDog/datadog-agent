// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

// Parity tests between the native OTLP to idx (V1) conversion and the legacy one.
//
// The oracle is the legacy pb conversion followed by ConvertToIdx:
//   - span level: ConvertToIdx(transform.OtelSpanToDDSpan(...)) against
//     transform.OtelSpanToIdxSpanWithConvertedFields(...);
//   - receiver level: ConvertToIdx(receiveResourceSpansV2(...)) against receiveResourceSpansV2Idx(...).
//
// Both outputs are resolved into plain Go values (every string ref is looked up in its string
// table, typed attribute values keep their type) and compared field by field. The only accepted
// differences are the ones of otlpIdxParityExceptions.

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"maps"
	"math"
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil/dupcheck"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
	normalizeutil "github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
	"github.com/DataDog/datadog-agent/pkg/trace/transform"
)

// ---------------------------------------------------------------------------------------------
// Expected differences
// ---------------------------------------------------------------------------------------------

// parityDiff is one difference between the oracle and the native conversion.
type parityDiff struct {
	path   string // where the difference is, e.g. "chunks[1].spans[0].attributes[env]"
	field  string // what differs, e.g. "span.start" or "span.attribute"
	oracle any
	native any
	// span is the input OTel span of a span-level difference, if hasSpan.
	span    ptrace.Span
	hasSpan bool
}

func (d parityDiff) String() string {
	return fmt.Sprintf("%s (%s): oracle=%v native=%v", d.path, d.field, d.oracle, d.native)
}

// parityException is an accepted difference between the oracle and the native conversion.
type parityException struct {
	name   string
	reason string
	match  func(d parityDiff) bool
}

// otlpIdxParityExceptions is the complete list of accepted differences. Keep each entry as narrow
// as possible and explain why the difference is expected; never add an entry to hide a bug of the
// native conversion.
var otlpIdxParityExceptions = []parityException{
	{
		name: "otel-trace-id-overwritten-by-scope-attribute",
		reason: "ConvertToIdx takes the upper 64 bits of the chunk trace ID from the otel.trace_id tag, which " +
			"OtelSpanToDDSpan sets to the OTel trace ID but a scope attribute of the same key then overwrites; " +
			"the native conversion uses the OTel trace ID. Only accepted when the native trace ID is the OTel " +
			"trace ID of the first span and only the upper 64 bits differ, so the oracle is the wrong one",
		match: func(d parityDiff) bool {
			if d.field != "chunk.trace_id" || !d.hasSpan {
				return false
			}
			o, ok1 := d.oracle.(string) // hex of the 16-byte trace ID
			n, ok2 := d.native.(string)
			tid := d.span.TraceID()
			return ok1 && ok2 && len(o) == 32 && n == hex.EncodeToString(tid[:]) && o[16:] == n[16:]
		},
	},
	{
		name: "start-before-year-2000",
		reason: "normalizeutil.FixStartTime replaces a start before year 2000 with time.Now() - duration, " +
			"which each conversion evaluates at its own time; only the start of such spans may differ, by less than a minute",
		match: func(d parityDiff) bool {
			if d.field != "span.start" || !d.hasSpan {
				return false
			}
			if int64(d.span.StartTimestamp()) >= normalizeutil.Year2000NanosecTS {
				return false
			}
			o, ok1 := d.oracle.(uint64)
			n, ok2 := d.native.(uint64)
			if !ok1 || !ok2 {
				return false
			}
			delta := o - n
			if n > o {
				delta = n - o
			}
			return delta <= uint64(time.Minute)
		},
	},
	{
		name: "container-tags-order",
		reason: "the _dd.tags.container payload attribute is built by flatten() from a Go map, so its tag order " +
			"is random on every call, for the legacy receiver too; only the order of its tags may differ",
		match: func(d parityDiff) bool {
			if d.field != "payload.attribute" || d.path != `payload.attributes["_dd.tags.container"]` {
				return false
			}
			o, ok1 := d.oracle.(typedValue)
			n, ok2 := d.native.(typedValue)
			if !ok1 || !ok2 || o.Kind != "string" || n.Kind != "string" {
				return false
			}
			ot, nt := strings.Split(o.Str, ","), strings.Split(n.Str, ",")
			slices.Sort(ot)
			slices.Sort(nt)
			return slices.Equal(ot, nt)
		},
	},
}

// unexpectedDiffs returns the differences that no exception accepts.
func unexpectedDiffs(diffs []parityDiff) []parityDiff {
	var out []parityDiff
	for _, d := range diffs {
		accepted := false
		for _, e := range otlpIdxParityExceptions {
			if e.match(d) {
				accepted = true
				break
			}
		}
		if !accepted {
			out = append(out, d)
		}
	}
	return out
}

// ---------------------------------------------------------------------------------------------
// Resolved views
// ---------------------------------------------------------------------------------------------

// typedValue is an idx.AnyValue with its string refs resolved. Doubles are kept as their bits so
// that NaN values compare equal.
type typedValue struct {
	Kind string
	Str  string
	Bits uint64
	Arr  []typedValue
	KV   map[string]typedValue
}

func (v typedValue) String() string {
	switch v.Kind {
	case "string":
		return strconv.Quote(v.Str)
	case "bytes":
		return "bytes(" + hex.EncodeToString([]byte(v.Str)) + ")"
	case "double":
		return "double(" + strconv.FormatFloat(math.Float64frombits(v.Bits), 'g', -1, 64) + ")"
	case "int":
		return "int(" + strconv.FormatInt(int64(v.Bits), 10) + ")"
	case "bool":
		return "bool(" + strconv.FormatBool(v.Bits != 0) + ")"
	case "array":
		parts := make([]string, len(v.Arr))
		for i, e := range v.Arr {
			parts[i] = e.String()
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case "kvlist":
		keys := slices.Sorted(maps.Keys(v.KV))
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = k + ":" + v.KV[k].String()
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return v.Kind
}

func resolveValue(v *idx.AnyValue, st *idx.StringTable) typedValue {
	if v == nil {
		return typedValue{Kind: "nil"}
	}
	switch x := v.Value.(type) {
	case *idx.AnyValue_StringValueRef:
		return typedValue{Kind: "string", Str: st.Get(x.StringValueRef)}
	case *idx.AnyValue_BoolValue:
		tv := typedValue{Kind: "bool"}
		if x.BoolValue {
			tv.Bits = 1
		}
		return tv
	case *idx.AnyValue_DoubleValue:
		return typedValue{Kind: "double", Bits: math.Float64bits(x.DoubleValue)}
	case *idx.AnyValue_IntValue:
		return typedValue{Kind: "int", Bits: uint64(x.IntValue)}
	case *idx.AnyValue_BytesValue:
		return typedValue{Kind: "bytes", Str: string(x.BytesValue)}
	case *idx.AnyValue_ArrayValue:
		if x.ArrayValue == nil {
			return typedValue{Kind: "array(nil)"}
		}
		arr := make([]typedValue, 0, len(x.ArrayValue.Values))
		for _, e := range x.ArrayValue.Values {
			arr = append(arr, resolveValue(e, st))
		}
		return typedValue{Kind: "array", Arr: arr}
	case *idx.AnyValue_KeyValueList:
		kv := map[string]typedValue{}
		if x.KeyValueList != nil {
			for _, e := range x.KeyValueList.KeyValues {
				if e == nil {
					continue
				}
				kv[st.Get(e.Key)] = resolveValue(e.Value, st)
			}
		}
		return typedValue{Kind: "kvlist", KV: kv}
	case nil:
		return typedValue{Kind: "unset"}
	default:
		return typedValue{Kind: fmt.Sprintf("unknown(%T)", x)}
	}
}

func resolveAttributes(attrs map[uint32]*idx.AnyValue, st *idx.StringTable) map[string]typedValue {
	out := make(map[string]typedValue, len(attrs))
	for k, v := range attrs {
		out[st.Get(k)] = resolveValue(v, st)
	}
	return out
}

type eventView struct {
	Time       uint64
	Name       string
	Attributes map[string]typedValue
}

type linkView struct {
	TraceID    string // hex of the 16-byte trace ID
	SpanID     uint64
	Tracestate string
	Flags      uint32
	Attributes map[string]typedValue
}

type spanView struct {
	Service, Name, Resource, Type string
	SpanID, ParentID              uint64
	Start, Duration               uint64
	Error                         bool
	Env, Version, Component       string
	Kind                          idx.SpanKind
	Attributes                    map[string]typedValue
	Events                        []eventView
	Links                         []linkView
}

func resolveSpan(s *idx.InternalSpan) spanView {
	v := spanView{
		Service:    s.Service(),
		Name:       s.Name(),
		Resource:   s.Resource(),
		Type:       s.Type(),
		SpanID:     s.SpanID(),
		ParentID:   s.ParentID(),
		Start:      s.Start(),
		Duration:   s.Duration(),
		Error:      s.Error(),
		Env:        s.Env(),
		Version:    s.Version(),
		Component:  s.Component(),
		Kind:       s.Kind(),
		Attributes: resolveAttributes(s.Attributes(), s.Strings),
		Events:     []eventView{},
		Links:      []linkView{},
	}
	for _, e := range s.Events() {
		v.Events = append(v.Events, eventView{
			Time:       e.Time(),
			Name:       e.Name(),
			Attributes: resolveAttributes(e.Attributes(), s.Strings),
		})
	}
	for _, l := range s.Links() {
		v.Links = append(v.Links, linkView{
			TraceID:    hex.EncodeToString(l.TraceID()),
			SpanID:     l.SpanID(),
			Tracestate: l.Tracestate(),
			Flags:      l.Flags(),
			Attributes: resolveAttributes(l.Attributes(), s.Strings),
		})
	}
	return v
}

type chunkView struct {
	TraceID           string // hex of the 16-byte trace ID
	Priority          int32
	SamplingMechanism uint32
	Origin            string
	DroppedTrace      bool
	Attributes        map[string]typedValue
	Spans             []spanView
}

func resolveChunk(c *idx.InternalTraceChunk) chunkView {
	v := chunkView{
		TraceID:           hex.EncodeToString(c.TraceID),
		Priority:          c.Priority,
		SamplingMechanism: c.SamplingMechanism(),
		Origin:            c.Origin(),
		DroppedTrace:      c.DroppedTrace,
		Attributes:        resolveAttributes(c.Attributes, c.Strings),
	}
	for _, s := range c.Spans {
		v.Spans = append(v.Spans, resolveSpan(s))
	}
	return v
}

type payloadView struct {
	Hostname, Env, ContainerID, LanguageName, LanguageVersion, TracerVersion, AppVersion, RuntimeID string
	HasContainerDebug                                                                               bool
	Attributes                                                                                      map[string]typedValue
	Chunks                                                                                          []chunkView
}

func resolvePayload(tp *idx.InternalTracerPayload) payloadView {
	v := payloadView{
		Hostname:          tp.Hostname(),
		Env:               tp.Env(),
		ContainerID:       tp.ContainerID(),
		LanguageName:      tp.LanguageName(),
		LanguageVersion:   tp.LanguageVersion(),
		TracerVersion:     tp.TracerVersion(),
		AppVersion:        tp.AppVersion(),
		RuntimeID:         tp.RuntimeID(),
		HasContainerDebug: tp.ContainerDebug != nil,
		Attributes:        resolveAttributes(tp.Attributes, tp.Strings),
	}
	for _, c := range tp.Chunks {
		v.Chunks = append(v.Chunks, resolveChunk(c))
	}
	return v
}

// ---------------------------------------------------------------------------------------------
// Comparison
// ---------------------------------------------------------------------------------------------

type parityDiffer struct {
	diffs []parityDiff
}

func (d *parityDiffer) add(path, field string, oracle, native any, span ptrace.Span, hasSpan bool) {
	d.diffs = append(d.diffs, parityDiff{path: path, field: field, oracle: oracle, native: native, span: span, hasSpan: hasSpan})
}

// check records a difference if oracle and native differ. Values must be comparable with ==, or
// be typedValue, which is compared deeply.
func check[T comparable](d *parityDiffer, path, field string, oracle, native T, span ptrace.Span, hasSpan bool) {
	if oracle != native {
		d.add(path, field, oracle, native, span, hasSpan)
	}
}

func typedValueEqual(a, b typedValue) bool {
	if a.Kind != b.Kind || a.Str != b.Str || a.Bits != b.Bits || len(a.Arr) != len(b.Arr) || len(a.KV) != len(b.KV) {
		return false
	}
	for i := range a.Arr {
		if !typedValueEqual(a.Arr[i], b.Arr[i]) {
			return false
		}
	}
	for k, av := range a.KV {
		bv, ok := b.KV[k]
		if !ok || !typedValueEqual(av, bv) {
			return false
		}
	}
	return true
}

func (d *parityDiffer) attributes(path, field string, oracle, native map[string]typedValue, span ptrace.Span, hasSpan bool) {
	keys := make(map[string]struct{}, len(oracle)+len(native))
	for k := range oracle {
		keys[k] = struct{}{}
	}
	for k := range native {
		keys[k] = struct{}{}
	}
	for _, k := range slices.Sorted(maps.Keys(keys)) {
		ov, oOK := oracle[k]
		nv, nOK := native[k]
		p := fmt.Sprintf("%s[%q]", path, k)
		switch {
		case !oOK:
			d.add(p, field, "<missing>", nv, span, hasSpan)
		case !nOK:
			d.add(p, field, ov, "<missing>", span, hasSpan)
		case !typedValueEqual(ov, nv):
			d.add(p, field, ov, nv, span, hasSpan)
		}
	}
}

func (d *parityDiffer) span(path string, o, n spanView, in ptrace.Span, hasSpan bool) {
	check(d, path+".service", "span.service", o.Service, n.Service, in, hasSpan)
	check(d, path+".name", "span.name", o.Name, n.Name, in, hasSpan)
	check(d, path+".resource", "span.resource", o.Resource, n.Resource, in, hasSpan)
	check(d, path+".type", "span.type", o.Type, n.Type, in, hasSpan)
	check(d, path+".span_id", "span.span_id", o.SpanID, n.SpanID, in, hasSpan)
	check(d, path+".parent_id", "span.parent_id", o.ParentID, n.ParentID, in, hasSpan)
	check(d, path+".start", "span.start", o.Start, n.Start, in, hasSpan)
	check(d, path+".duration", "span.duration", o.Duration, n.Duration, in, hasSpan)
	check(d, path+".error", "span.error", o.Error, n.Error, in, hasSpan)
	check(d, path+".env", "span.env", o.Env, n.Env, in, hasSpan)
	check(d, path+".version", "span.version", o.Version, n.Version, in, hasSpan)
	check(d, path+".component", "span.component", o.Component, n.Component, in, hasSpan)
	check(d, path+".kind", "span.kind", o.Kind, n.Kind, in, hasSpan)
	d.attributes(path+".attributes", "span.attribute", o.Attributes, n.Attributes, in, hasSpan)

	check(d, path+".events.len", "span.events", len(o.Events), len(n.Events), in, hasSpan)
	for i := range min(len(o.Events), len(n.Events)) {
		p := fmt.Sprintf("%s.events[%d]", path, i)
		check(d, p+".time", "span.event.time", o.Events[i].Time, n.Events[i].Time, in, hasSpan)
		check(d, p+".name", "span.event.name", o.Events[i].Name, n.Events[i].Name, in, hasSpan)
		d.attributes(p+".attributes", "span.event.attribute", o.Events[i].Attributes, n.Events[i].Attributes, in, hasSpan)
	}
	check(d, path+".links.len", "span.links", len(o.Links), len(n.Links), in, hasSpan)
	for i := range min(len(o.Links), len(n.Links)) {
		p := fmt.Sprintf("%s.links[%d]", path, i)
		check(d, p+".trace_id", "span.link.trace_id", o.Links[i].TraceID, n.Links[i].TraceID, in, hasSpan)
		check(d, p+".span_id", "span.link.span_id", o.Links[i].SpanID, n.Links[i].SpanID, in, hasSpan)
		check(d, p+".tracestate", "span.link.tracestate", o.Links[i].Tracestate, n.Links[i].Tracestate, in, hasSpan)
		check(d, p+".flags", "span.link.flags", o.Links[i].Flags, n.Links[i].Flags, in, hasSpan)
		d.attributes(p+".attributes", "span.link.attribute", o.Links[i].Attributes, n.Links[i].Attributes, in, hasSpan)
	}
}

// chunk compares two chunks. inputs are the input OTel spans of the chunk, in order, if known.
func (d *parityDiffer) chunk(path string, o, n chunkView, inputs []ptrace.Span) {
	none := ptrace.NewSpan()
	// The chunk trace ID is the one of its first span.
	first, hasFirst := none, false
	if len(inputs) > 0 {
		first, hasFirst = inputs[0], true
	}
	check(d, path+".trace_id", "chunk.trace_id", o.TraceID, n.TraceID, first, hasFirst)
	check(d, path+".priority", "chunk.priority", o.Priority, n.Priority, none, false)
	check(d, path+".sampling_mechanism", "chunk.sampling_mechanism", o.SamplingMechanism, n.SamplingMechanism, none, false)
	check(d, path+".origin", "chunk.origin", o.Origin, n.Origin, none, false)
	check(d, path+".dropped_trace", "chunk.dropped_trace", o.DroppedTrace, n.DroppedTrace, none, false)
	d.attributes(path+".attributes", "chunk.attribute", o.Attributes, n.Attributes, none, false)
	check(d, path+".spans.len", "chunk.spans", len(o.Spans), len(n.Spans), none, false)
	for i := range min(len(o.Spans), len(n.Spans)) {
		in, hasSpan := none, false
		if len(inputs) == len(o.Spans) {
			in, hasSpan = inputs[i], true
		}
		d.span(fmt.Sprintf("%s.spans[%d]", path, i), o.Spans[i], n.Spans[i], in, hasSpan)
	}
}

// payload compares two tracer payloads. inputs are the input OTel spans of each chunk, if known.
func (d *parityDiffer) payload(o, n payloadView, inputs [][]ptrace.Span) {
	none := ptrace.NewSpan()
	check(d, "payload.hostname", "payload.hostname", o.Hostname, n.Hostname, none, false)
	check(d, "payload.env", "payload.env", o.Env, n.Env, none, false)
	check(d, "payload.container_id", "payload.container_id", o.ContainerID, n.ContainerID, none, false)
	check(d, "payload.language_name", "payload.language_name", o.LanguageName, n.LanguageName, none, false)
	check(d, "payload.language_version", "payload.language_version", o.LanguageVersion, n.LanguageVersion, none, false)
	check(d, "payload.tracer_version", "payload.tracer_version", o.TracerVersion, n.TracerVersion, none, false)
	check(d, "payload.app_version", "payload.app_version", o.AppVersion, n.AppVersion, none, false)
	check(d, "payload.runtime_id", "payload.runtime_id", o.RuntimeID, n.RuntimeID, none, false)
	check(d, "payload.container_debug", "payload.container_debug", o.HasContainerDebug, n.HasContainerDebug, none, false)
	d.attributes("payload.attributes", "payload.attribute", o.Attributes, n.Attributes, none, false)
	check(d, "payload.chunks.len", "payload.chunks", len(o.Chunks), len(n.Chunks), none, false)
	for i := range min(len(o.Chunks), len(n.Chunks)) {
		var in []ptrace.Span
		if len(inputs) == len(o.Chunks) {
			in = inputs[i]
		}
		d.chunk(fmt.Sprintf("payload.chunks[%d]", i), o.Chunks[i], n.Chunks[i], in)
	}
}

// reportParityDiffs fails t with the unexpected differences, if any, and a dump of the input.
func reportParityDiffs(t testing.TB, what string, diffs []parityDiff, input func() string) {
	t.Helper()
	bad := unexpectedDiffs(diffs)
	if len(bad) == 0 {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d unexpected difference(s) between the oracle (ConvertToIdx of the pb conversion) and the native idx conversion:\n", what, len(bad))
	for i, d := range bad {
		if i == 40 {
			fmt.Fprintf(&b, "  ... and %d more\n", len(bad)-i)
			break
		}
		fmt.Fprintf(&b, "  %s\n", d)
	}
	in := input()
	if len(in) > 20000 {
		in = in[:20000] + "...(truncated)"
	}
	fmt.Fprintf(&b, "input: %s", in)
	t.Error(b.String())
}

// ---------------------------------------------------------------------------------------------
// Input snapshots
// ---------------------------------------------------------------------------------------------

func resourceSpansJSON(rspans ptrace.ResourceSpans) []byte {
	td := ptrace.NewTraces()
	rspans.CopyTo(td.ResourceSpans().AppendEmpty())
	b, err := (&ptrace.JSONMarshaler{}).MarshalTraces(td)
	if err != nil {
		return []byte(fmt.Sprintf("<marshal error: %v>", err))
	}
	return b
}

// spanInput wraps a span with its resource and scope into a ResourceSpans, to snapshot it.
func spanInput(otelspan ptrace.Span, res pcommon.Resource, lib pcommon.InstrumentationScope) ptrace.ResourceSpans {
	rs := ptrace.NewResourceSpans()
	res.CopyTo(rs.Resource())
	ss := rs.ScopeSpans().AppendEmpty()
	lib.CopyTo(ss.Scope())
	otelspan.CopyTo(ss.Spans().AppendEmpty())
	return rs
}

func cloneResourceSpans(rspans ptrace.ResourceSpans) ptrace.ResourceSpans {
	c := ptrace.NewResourceSpans()
	rspans.CopyTo(c)
	return c
}

// ---------------------------------------------------------------------------------------------
// Span-level parity
// ---------------------------------------------------------------------------------------------

// requireOTelSpanIdxParity checks that transform.OtelSpanToIdxSpanWithConvertedFields (and
// transform.OtelSpanToIdxSpan) is equivalent to ConvertToIdx applied to transform.OtelSpanToDDSpan
// for one span, including the fields promoted from the span to its chunk and tracer payload.
func requireOTelSpanIdxParity(t testing.TB, otelspan ptrace.Span, res pcommon.Resource, lib pcommon.InstrumentationScope, conf *config.AgentConfig) {
	t.Helper()
	before := resourceSpansJSON(spanInput(otelspan, res, lib))

	// Oracle: the pb span in a one-span chunk, converted with ConvertToIdx.
	ddspan := transform.OtelSpanToDDSpan(otelspan, res, lib, conf)
	oracle := ConvertToIdx(&pb.TracerPayload{
		Chunks: []*pb.TraceChunk{{Priority: int32(sampler.PriorityNone), Spans: []*pb.Span{ddspan}}},
	}, "")

	// Native: the idx span, with its converted fields applied to a scratch chunk and payload the
	// same way ConvertToIdx applies them.
	strs := idx.NewStringTable()
	fields := idx.NewSpanConvertedFields()
	tid := [16]byte(otelspan.TraceID())
	fields.TraceIDUpper = binary.BigEndian.Uint64(tid[:8])
	fields.TraceIDLower = transform.OTelTraceIDToUint64(tid)
	span, _ := transform.OtelSpanToIdxSpanWithConvertedFields(otelspan, res, lib, conf, strs, fields)
	var rootSampling idx.RootSamplingMergeState
	rootSampling.ReconcileSamplingPriorityAfterChunkSpan(fields, span.ParentID())
	chunk := &idx.InternalTraceChunk{Strings: strs, Attributes: map[uint32]*idx.AnyValue{}, Spans: []*idx.InternalSpan{span}}
	var chunkFields idx.ChunkConvertedFields
	chunk.ApplyPromotedFields(fields, &chunkFields)
	native := &idx.InternalTracerPayload{Strings: strs, Attributes: map[uint32]*idx.AnyValue{}, Chunks: []*idx.InternalTraceChunk{chunk}}
	native.ApplyPromotedFields(&chunkFields)

	inputs := [][]ptrace.Span{{otelspan}}
	var d parityDiffer
	d.payload(resolvePayload(oracle), resolvePayload(native), inputs)

	// OtelSpanToIdxSpan must build the same span as OtelSpanToIdxSpanWithConvertedFields.
	plain := transform.OtelSpanToIdxSpan(otelspan, res, lib, conf, idx.NewStringTable())
	var dPlain parityDiffer
	dPlain.span("OtelSpanToIdxSpan", resolveSpan(span), resolveSpan(plain), otelspan, true)
	// The span path tells these differences apart; the field is kept so the exceptions apply.
	d.diffs = append(d.diffs, dPlain.diffs...)

	if after := resourceSpansJSON(spanInput(otelspan, res, lib)); !bytes.Equal(before, after) {
		t.Errorf("span conversion mutated its input:\nbefore: %s\nafter:  %s", before, after)
	}
	reportParityDiffs(t, "span parity", d.diffs, func() string { return string(before) })
}

// ---------------------------------------------------------------------------------------------
// Receiver-level parity
// ---------------------------------------------------------------------------------------------

// groupSpansByTrace returns the spans of rspans grouped into the chunks the receivers build: by
// the low 64 bits of the trace ID, in the order the traces are first seen.
func groupSpansByTrace(rspans ptrace.ResourceSpans) [][]ptrace.Span {
	index := map[uint64]int{}
	var groups [][]ptrace.Span
	for i := 0; i < rspans.ScopeSpans().Len(); i++ {
		spans := rspans.ScopeSpans().At(i).Spans()
		for j := 0; j < spans.Len(); j++ {
			s := spans.At(j)
			id := transform.OTelTraceIDToUint64(s.TraceID())
			k, ok := index[id]
			if !ok {
				k = len(groups)
				index[id] = k
				groups = append(groups, nil)
			}
			groups[k] = append(groups[k], s)
		}
	}
	return groups
}

// requireReceiverIdxParity checks that receiveResourceSpansV2Idx is equivalent to ConvertToIdx
// applied to the payload of receiveResourceSpansV2.
func requireReceiverIdxParity(t testing.TB, conf *config.AgentConfig, rspans ptrace.ResourceSpans, clientComputedStats bool) {
	t.Helper()
	before := resourceSpansJSON(rspans)
	o := NewOTLPReceiver(nil, conf, &statsd.NoOpClient{}, &timing.NoopReporter{})
	ctx := context.Background()

	// Each builder gets its own copy of the input, and the input must not be mutated by either.
	legacyIn := cloneResourceSpans(rspans)
	legacy, legacySrc := o.receiveResourceSpansV2(ctx, legacyIn, clientComputedStats, nil)
	nativeIn := cloneResourceSpans(rspans)
	native, nativeSrc := o.receiveResourceSpansV2Idx(ctx, nativeIn, clientComputedStats, nil)
	for name, rs := range map[string]ptrace.ResourceSpans{"receiveResourceSpansV2": legacyIn, "receiveResourceSpansV2Idx": nativeIn} {
		if after := resourceSpansJSON(rs); !bytes.Equal(before, after) {
			t.Errorf("%s mutated its input:\nbefore: %s\nafter:  %s", name, before, after)
		}
	}

	var d parityDiffer
	none := ptrace.NewSpan()
	check(&d, "source", "source", fmt.Sprintf("%+v", legacySrc), fmt.Sprintf("%+v", nativeSrc), none, false)
	check(&d, "Source.Tags", "payload.source", fmt.Sprintf("%+v", legacy.Source.Tags), fmt.Sprintf("%+v", native.Source.Tags), none, false)
	check(&d, "ClientComputedStats", "payload.client_computed_stats", legacy.ClientComputedStats, native.ClientComputedStats, none, false)
	check(&d, "ClientComputedTopLevel", "payload.client_computed_top_level", legacy.ClientComputedTopLevel, native.ClientComputedTopLevel, none, false)
	check(&d, "ClientDroppedP0s", "payload.client_dropped_p0s", legacy.ClientDroppedP0s, native.ClientDroppedP0s, none, false)
	check(&d, "ProcessTags", "payload.process_tags", legacy.ProcessTags, native.ProcessTags, none, false)
	check(&d, "ContainerTags", "payload.container_tags", strings.Join(legacy.ContainerTags, ","), strings.Join(native.ContainerTags, ","), none, false)
	d.payload(resolvePayload(ConvertToIdx(legacy.TracerPayload, "")), resolvePayload(native.TracerPayload), groupSpansByTrace(rspans))
	reportParityDiffs(t, "receiver parity", d.diffs, func() string { return string(before) })
}

// ---------------------------------------------------------------------------------------------
// Configurations
// ---------------------------------------------------------------------------------------------

type parityConfig struct {
	name string
	// spanOnly configs are only used for the span-level parity: they select the legacy V1
	// receiver, which is out of scope.
	spanOnly bool
	conf     *config.AgentConfig
}

func newParityTestConfig(t testing.TB) *config.AgentConfig {
	cfg := config.New()
	attributesTranslator, err := attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
	if err != nil {
		t.Fatal(err)
	}
	cfg.OTLPReceiver.AttributesTranslator = attributesTranslator
	cfg.Hostname = "parity-fallback-host"
	return cfg
}

// otlpIdxParityConfigs returns the configurations the parity tests run with: the default one and
// one per option the OTLP V2 conversion reads.
func otlpIdxParityConfigs(t testing.TB) []parityConfig {
	withFeatures := func(features ...string) *config.AgentConfig {
		cfg := newParityTestConfig(t)
		for _, f := range features {
			cfg.Features[f] = struct{}{}
		}
		return cfg
	}
	spanNameAsResourceName := newParityTestConfig(t)
	spanNameAsResourceName.OTLPReceiver.SpanNameAsResourceName = true
	spanNameRemappings := newParityTestConfig(t)
	spanNameRemappings.OTLPReceiver.SpanNameRemappings = map[string]string{
		"opentelemetry.server":       "remapped.server",
		"go.opentelemetry.io_client": "remapped.client",
		"http.server.request":        "remapped.http",
		"internal":                   "remapped.internal",
	}
	probabilisticSamplerEnabled := newParityTestConfig(t)
	probabilisticSamplerEnabled.ProbabilisticSamplerEnabled = true
	probabilisticSampling := newParityTestConfig(t)
	probabilisticSampling.OTLPReceiver.ProbabilisticSampling = 50
	containerTagsV1 := withFeatures("disable_otlp_container_tags_v2")
	containerTagsV1.ContainerTags = func(cid string) ([]string, error) {
		if cid == "" {
			return nil, nil
		}
		return []string{"kube_cluster_name:parity", "container_id_tag:" + cid}, nil
	}
	combined := withFeatures("disable_operation_and_resource_name_logic_v2", "enable_otlp_compute_top_level_by_span_kind", "disable_otel_scope_convention")
	combined.OTLPReceiver.ProbabilisticSampling = 30
	combined.OTLPReceiver.SpanNameRemappings = map[string]string{"opentelemetry.client": "remapped.client"}

	return []parityConfig{
		{name: "default", conf: newParityTestConfig(t)},
		{name: "disable_operation_and_resource_name_logic_v2", conf: withFeatures("disable_operation_and_resource_name_logic_v2")},
		{name: "enable_otlp_compute_top_level_by_span_kind", conf: withFeatures("enable_otlp_compute_top_level_by_span_kind")},
		{name: "disable_otlp_container_tags_v2", conf: containerTagsV1},
		{name: "disable_otel_scope_convention", conf: withFeatures("disable_otel_scope_convention")},
		{name: "SpanNameAsResourceName", conf: spanNameAsResourceName},
		{name: "SpanNameRemappings", conf: spanNameRemappings},
		{name: "ProbabilisticSamplerEnabled", conf: probabilisticSamplerEnabled},
		{name: "ProbabilisticSampling=50", conf: probabilisticSampling},
		{name: "combined", conf: combined},
		{name: "disable_receive_resource_spans_v2", spanOnly: true, conf: withFeatures("disable_receive_resource_spans_v2")},
	}
}

// requireIdxParity runs the receiver parity (unless the config is span-only) and the span parity
// of every span of rspans.
func requireIdxParity(t testing.TB, pc parityConfig, rspans ptrace.ResourceSpans, clientComputedStats bool) {
	t.Helper()
	if !pc.spanOnly {
		requireReceiverIdxParity(t, pc.conf, rspans, clientComputedStats)
	}
	for i := 0; i < rspans.ScopeSpans().Len(); i++ {
		ss := rspans.ScopeSpans().At(i)
		for j := 0; j < ss.Spans().Len(); j++ {
			requireOTelSpanIdxParity(t, ss.Spans().At(j), rspans.Resource(), ss.Scope(), pc.conf)
		}
	}
}

// ---------------------------------------------------------------------------------------------
// Hand-written inputs
// ---------------------------------------------------------------------------------------------

// parityTS is a start timestamp after year 2000.
const parityTS = uint64(1_700_000_000_000_000_000)

func parityTraceID(upper, lower uint64) [16]byte {
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], upper)
	binary.BigEndian.PutUint64(b[8:], lower)
	return b
}

func paritySpanID(id uint64) pcommon.SpanID {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], id)
	return b
}

// putParityAttrs sets the attributes of m from raw values, in key order.
func putParityAttrs(m pcommon.Map, attrs map[string]any) {
	for _, k := range slices.Sorted(maps.Keys(attrs)) {
		if err := m.PutEmpty(k).FromRaw(attrs[k]); err != nil {
			panic(err)
		}
	}
}

func newParityRS(resAttrs map[string]any) ptrace.ResourceSpans {
	rs := ptrace.NewResourceSpans()
	putParityAttrs(rs.Resource().Attributes(), resAttrs)
	return rs
}

func addParityScope(rs ptrace.ResourceSpans, name, version string, attrs map[string]any) ptrace.ScopeSpans {
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName(name)
	ss.Scope().SetVersion(version)
	putParityAttrs(ss.Scope().Attributes(), attrs)
	return ss
}

// addParitySpan adds a server span named "op" to ss, with the given IDs and attributes.
func addParitySpan(ss ptrace.ScopeSpans, tid [16]byte, sid, pid uint64, attrs map[string]any, opts ...func(ptrace.Span)) ptrace.Span {
	s := ss.Spans().AppendEmpty()
	s.SetTraceID(tid)
	s.SetSpanID(paritySpanID(sid))
	if pid != 0 {
		s.SetParentSpanID(paritySpanID(pid))
	}
	s.SetName("op")
	s.SetKind(ptrace.SpanKindServer)
	s.SetStartTimestamp(pcommon.Timestamp(parityTS + sid*1000))
	s.SetEndTimestamp(pcommon.Timestamp(parityTS + sid*1000 + 12345))
	putParityAttrs(s.Attributes(), attrs)
	for _, o := range opts {
		o(s)
	}
	return s
}

type parityCase struct {
	name string
	rs   ptrace.ResourceSpans
}

func withKind(k ptrace.SpanKind) func(ptrace.Span) { return func(s ptrace.Span) { s.SetKind(k) } }
func withStatus(c ptrace.StatusCode, msg string) func(ptrace.Span) {
	return func(s ptrace.Span) { s.Status().SetCode(c); s.Status().SetMessage(msg) }
}
func withTraceState(ts string) func(ptrace.Span) {
	return func(s ptrace.Span) { s.TraceState().FromRaw(ts) }
}
func withName(n string) func(ptrace.Span) { return func(s ptrace.Span) { s.SetName(n) } }
func withEvent(name string, ts uint64, attrs map[string]any) func(ptrace.Span) {
	return func(s ptrace.Span) {
		e := s.Events().AppendEmpty()
		e.SetName(name)
		e.SetTimestamp(pcommon.Timestamp(ts))
		putParityAttrs(e.Attributes(), attrs)
	}
}
func withLink(tid [16]byte, sid uint64, ts string, flags uint32, attrs map[string]any) func(ptrace.Span) {
	return func(s ptrace.Span) {
		l := s.Links().AppendEmpty()
		l.SetTraceID(tid)
		l.SetSpanID(paritySpanID(sid))
		l.TraceState().FromRaw(ts)
		l.SetFlags(flags)
		putParityAttrs(l.Attributes(), attrs)
	}
}
func withTimes(start, end uint64) func(ptrace.Span) {
	return func(s ptrace.Span) {
		s.SetStartTimestamp(pcommon.Timestamp(start))
		s.SetEndTimestamp(pcommon.Timestamp(end))
	}
}

func handWrittenParityCases() []parityCase {
	var cases []parityCase
	add := func(name string, rs ptrace.ResourceSpans) { cases = append(cases, parityCase{name, rs}) }
	baseRes := map[string]any{"service.name": "parity-svc", "telemetry.sdk.language": "go", "telemetry.sdk.version": "1.2.3"}

	{ // 128-bit trace IDs: same low bits with different upper bits share a chunk.
		rs := newParityRS(baseRes)
		ss := addParityScope(rs, "lib", "1.0", nil)
		addParitySpan(ss, parityTraceID(0xaaaa, 1), 10, 0, nil)
		addParitySpan(ss, parityTraceID(0xbbbb, 1), 11, 10, nil, withKind(ptrace.SpanKindClient))
		addParitySpan(ss, parityTraceID(0, 2), 12, 0, nil)
		addParitySpan(ss, parityTraceID(math.MaxUint64, math.MaxUint64), 13, 0, nil)
		addParitySpan(ss, parityTraceID(0, 0), 14, 0, nil)
		add("128-bit trace IDs", rs)
	}
	{ // Multiple traces, interleaved, over multiple scopes, with every span kind.
		rs := newParityRS(baseRes)
		ss1 := addParityScope(rs, "scope.one", "v1", map[string]any{"scope.attr": "one", "scope.num": 3})
		ss2 := addParityScope(rs, "", "", nil)
		ss3 := addParityScope(rs, "scope.three", "", map[string]any{"scope.attr": "three"})
		kinds := []ptrace.SpanKind{ptrace.SpanKindUnspecified, ptrace.SpanKindInternal, ptrace.SpanKindServer, ptrace.SpanKindClient, ptrace.SpanKindProducer, ptrace.SpanKindConsumer}
		for i, k := range kinds {
			ss := []ptrace.ScopeSpans{ss1, ss2, ss3}[i%3]
			tid := parityTraceID(0x10, uint64(100+i%3))
			var parent uint64
			if i >= 3 {
				parent = uint64(200 + i%3)
			}
			addParitySpan(ss, tid, uint64(200+i), parent, map[string]any{"i": i}, withKind(k), withName(fmt.Sprintf("op-%d", i)))
		}
		add("multiple traces and scopes", rs)
	}
	{ // Errors and HTTP status codes.
		rs := newParityRS(baseRes)
		ss := addParityScope(rs, "lib", "1.0", nil)
		tid := parityTraceID(0, 300)
		addParitySpan(ss, tid, 301, 0, nil, withStatus(ptrace.StatusCodeError, "boom"))
		addParitySpan(ss, tid, 302, 301, map[string]any{"http.status_code": 404}, withStatus(ptrace.StatusCodeError, ""), withKind(ptrace.SpanKindClient))
		addParitySpan(ss, tid, 303, 301, map[string]any{"http.status_code": "503"}, withStatus(ptrace.StatusCodeError, ""))
		addParitySpan(ss, tid, 304, 301, map[string]any{"http.response.status_code": 500, "http.request.method": "POST"}, withStatus(ptrace.StatusCodeError, ""))
		addParitySpan(ss, tid, 305, 301, map[string]any{"http.response.status_code": "418", "http.status_code": 200}, withStatus(ptrace.StatusCodeError, ""))
		addParitySpan(ss, tid, 306, 301, map[string]any{"http.status_code": -1, "error.msg": "explicit"}, withStatus(ptrace.StatusCodeError, "status msg"))
		addParitySpan(ss, tid, 307, 301, nil, withStatus(ptrace.StatusCodeError, ""), withEvent("exception", parityTS, map[string]any{
			"exception.message": "it broke", "exception.type": "Err", "exception.stacktrace": "a\nb", "exception.escaped": true,
		}))
		addParitySpan(ss, tid, 308, 301, map[string]any{"error.type": "T", "error.stack": "S"}, withStatus(ptrace.StatusCodeOk, "fine"))
		addParitySpan(ss, tid, 309, 301, map[string]any{"http.status_code": 999}, withStatus(ptrace.StatusCodeError, ""))
		add("errors", rs)
	}
	{ // Sampling priorities.
		rs := newParityRS(baseRes)
		ss := addParityScope(rs, "lib", "1.0", nil)
		a, b, c, d, e, f := parityTraceID(0, 400), parityTraceID(0, 401), parityTraceID(0, 402), parityTraceID(0, 403), parityTraceID(0, 404), parityTraceID(0, 405)
		addParitySpan(ss, a, 410, 0, map[string]any{"sampling.priority": 2})
		addParitySpan(ss, a, 411, 410, map[string]any{"sampling.priority": -1.0})
		addParitySpan(ss, b, 420, 0, nil)
		addParitySpan(ss, b, 421, 420, map[string]any{"sampling.priority": 1.0})
		addParitySpan(ss, c, 430, 0, map[string]any{"_sampling_priority_v1": 0.0, "sampling.priority": 2})
		addParitySpan(ss, c, 431, 430, map[string]any{"sampling.priority": 2})
		addParitySpan(ss, d, 440, 0, nil)
		addParitySpan(ss, e, 450, 0, map[string]any{"sampling.priority": "2"})
		addParitySpan(ss, f, 461, 460, map[string]any{"sampling.priority": 1})
		addParitySpan(ss, f, 460, 0, map[string]any{"sampling.priority": -1})
		addParitySpan(ss, f, 462, 460, map[string]any{"sampling.priority": 2})
		add("sampling priorities", rs)
	}
	{ // Decision makers.
		rs := newParityRS(baseRes)
		ss := addParityScope(rs, "lib", "1.0", nil)
		addParitySpan(ss, parityTraceID(0, 500), 501, 0, map[string]any{"_dd.p.dm": "-12", "sampling.priority": 1})
		addParitySpan(ss, parityTraceID(0, 510), 511, 0, map[string]any{"sampling.priority": 2}, withTraceState("dd=s:2;t.dm:-12"))
		addParitySpan(ss, parityTraceID(0, 520), 521, 0, nil, withTraceState("foo=bar,dd=s:1;t.dm:-3"))
		addParitySpan(ss, parityTraceID(0, 530), 531, 0, map[string]any{"_dd.p.dm": "abc", "sampling.priority": 1})
		addParitySpan(ss, parityTraceID(0, 540), 541, 0, map[string]any{"_dd.p.dm": 5, "sampling.priority": 1})
		addParitySpan(ss, parityTraceID(0, 550), 551, 0, map[string]any{"sampling.priority": 1})
		addParitySpan(ss, parityTraceID(0, 550), 552, 551, map[string]any{"_dd.p.dm": "-7"}, withTraceState("dd=t.dm:-8"))
		addParitySpan(ss, parityTraceID(0, 560), 561, 0, map[string]any{"_dd.p.dm": "-12"})
		addParitySpan(ss, parityTraceID(0, 570), 571, 0, map[string]any{"_dd.p.dm": "-"}, withTraceState("dd=t.dm:-x"))
		add("decision makers", rs)
	}
	{ // Sample rates, measured, analytics events.
		rs := newParityRS(baseRes)
		ss := addParityScope(rs, "lib", "1.0", nil)
		tid := parityTraceID(0, 600)
		addParitySpan(ss, tid, 601, 0, map[string]any{"_sample_rate": 0.5})
		addParitySpan(ss, tid, 602, 601, nil, withTraceState("ot=th:8"))
		addParitySpan(ss, tid, 603, 601, map[string]any{"_sample_rate": "0.25"}, withTraceState("ot=th:c"))
		addParitySpan(ss, tid, 604, 601, map[string]any{"_dd.measured": 1})
		addParitySpan(ss, tid, 605, 601, map[string]any{"_dd.measured": "1"}, withKind(ptrace.SpanKindClient))
		addParitySpan(ss, tid, 606, 601, map[string]any{"analytics.event": "true"}, withKind(ptrace.SpanKindProducer))
		addParitySpan(ss, tid, 607, 601, map[string]any{"analytics.event": false})
		addParitySpan(ss, tid, 608, 601, map[string]any{"analytics.event": "maybe", "_dd1.sr.eausr": 0.3})
		addParitySpan(ss, tid, 609, 601, map[string]any{"_dd.measured": 2.0, "_top_level": 1})
		add("sample rates and measured", rs)
	}
	{ // Span events with every attribute type.
		rs := newParityRS(baseRes)
		ss := addParityScope(rs, "lib", "1.0", nil)
		addParitySpan(ss, parityTraceID(0, 700), 701, 0, nil,
			withEvent("exception", parityTS+1, map[string]any{"exception.message": "m"}),
			withEvent("mixed", parityTS+2, map[string]any{
				"str": "s", "int": 7, "double": 1.5, "bool": true, "bytes": []byte{0, 1, 0xff},
				"homogeneous": []any{1, 2, 3}, "strs": []any{"a", "b"}, "mixed": []any{1, "a", true},
				"empty_slice": []any{}, "nested_slice": []any{[]any{1}, []any{"x"}},
				"map": map[string]any{"a": map[string]any{"b": 1}, "c": []any{"d"}}, "empty_map": map[string]any{},
				"empty": nil, "slice_of_maps": []any{map[string]any{"k": "v"}},
			}),
			withEvent("", 0, nil),
		)
		add("events", rs)
	}
	{ // Span links.
		rs := newParityRS(baseRes)
		ss := addParityScope(rs, "lib", "1.0", nil)
		addParitySpan(ss, parityTraceID(0, 800), 801, 0, nil,
			withLink(parityTraceID(0xdead, 0xbeef), 0x1234, "dd=s:1", 0, nil),
			withLink(parityTraceID(0, 1), 1, "", 1, map[string]any{"a": "b", "n": 1, "d": 2.5, "flag": true}),
			withLink(parityTraceID(0, 0), 0, "ot=th:8,dd=t.dm:-4", 0x301, map[string]any{
				"nested": map[string]any{"x": []any{1, "y", map[string]any{"z": true}}}, "empty": nil, "bytes": []byte("hi"),
			}),
			withLink(parityTraceID(1, 2), 3, "", 0x80000001, map[string]any{"s": []any{}}),
		)
		add("links", rs)
	}
	{ // Scope attributes overriding Datadog keys.
		rs := newParityRS(map[string]any{"service.name": "res-svc", "deployment.environment": "res-env", "service.version": "res-ver"})
		ss := addParityScope(rs, "lib", "9.9", map[string]any{
			"env": "scope-env", "span.kind": "consumer", "version": "scope-ver", "component": "scope-comp",
			"service.name": "scope-svc", "_dd.origin": "scope-origin", "otel.trace_id": "not-hex", "_dd.hostname": "scope-host",
			"_dd.p.dm": "-3", "w3c.tracestate": "dd=t.dm:-5", "http.status_code": "201", "num": 4,
		})
		tid := parityTraceID(0xabc, 900)
		addParitySpan(ss, tid, 901, 0, map[string]any{"env": "span-env", "sampling.priority": 1})
		addParitySpan(ss, tid, 902, 901, map[string]any{"_dd.p.dm": 7, "w3c.tracestate": 3.0, "num": "x"})
		addParitySpan(ss, parityTraceID(0, 910), 911, 0, map[string]any{"sampling.priority": 2})
		add("scope attribute overrides", rs)
	}
	{ // Scope attribute with a numeric span attribute of the same Datadog key.
		rs := newParityRS(baseRes)
		ss := addParityScope(rs, "lib", "1.0", map[string]any{"_dd.p.dm": "-3"})
		addParitySpan(ss, parityTraceID(0, 950), 951, 0, map[string]any{"_dd.p.dm": 7, "sampling.priority": 1})
		add("scope _dd.p.dm under numeric span _dd.p.dm", rs)
	}
	{ // Numeric span attribute w3c.tracestate with a W3C tracestate holding a decision maker.
		rs := newParityRS(baseRes)
		ss := addParityScope(rs, "lib", "1.0", nil)
		addParitySpan(ss, parityTraceID(0, 960), 961, 0, map[string]any{"w3c.tracestate": 3.0, "sampling.priority": 1}, withTraceState("dd=s:1;t.dm:-12"))
		add("numeric w3c.tracestate attribute over tracestate", rs)
	}
	{ // Resource vs span attribute collisions with different types.
		rs := newParityRS(map[string]any{
			"service.name": "res-svc", "http.status_code": "200", "foo": 1, "bar": "res-bar", "baz": 2.5,
			"deployment.environment": "res-env", "env": "res-dd-env", "version": "res-version", "component": "res-comp",
			"span.kind": "producer", "_dd.measured": "1", "sampling.priority": 2,
		})
		ss := addParityScope(rs, "lib", "1.0", nil)
		tid := parityTraceID(0, 1000)
		addParitySpan(ss, tid, 1001, 0, map[string]any{"http.status_code": 404, "foo": "span-foo", "bar": 1.5, "baz": "span-baz"})
		addParitySpan(ss, tid, 1002, 1001, map[string]any{"deployment.environment.name": "span-env", "version": 3, "component": 1})
		addParitySpan(ss, tid, 1003, 1001, map[string]any{"env": 1, "span.kind": 2})
		add("resource vs span collisions", rs)
	}
	{ // Container tags and resource identity attributes.
		rs := newParityRS(map[string]any{
			"service.name": "svc", "container.id": "cid-123", "k8s.pod.uid": "pod-uid", "container.name": "cname",
			"k8s.container.name": "kcname", "container.image.name": "img", "container.image.tag": "latest",
			"k8s.namespace.name": "ns", "k8s.deployment.name": "dep", "cloud.provider": "aws", "cloud.region": "us-east-1",
			"aws.ecs.task.arn": "arn:aws:ecs:task", "aws.ecs.launchtype": "fargate", "datadog.container.tag.custom": "cv",
			"host.name": "res-host", "telemetry.sdk.language": "java", "telemetry.sdk.version": "2.0",
		})
		ss := addParityScope(rs, "lib", "1.0", nil)
		addParitySpan(ss, parityTraceID(0, 1100), 1101, 0, map[string]any{"container.id": "span-cid", "k8s.pod.name": "p"})
		add("container tags", rs)
	}
	{ // k8s pod UID only (used as container ID with disable_otlp_container_tags_v2).
		rs := newParityRS(map[string]any{"service.name": "svc", "k8s.pod.uid": "pod-uid-only", "datadog.host.name": "dd-host", "_dd.stats_computed": "true"})
		ss := addParityScope(rs, "lib", "1.0", nil)
		addParitySpan(ss, parityTraceID(0, 1150), 1151, 0, nil)
		add("pod uid and datadog host", rs)
	}
	{ // Datadog fields promoted to the chunk and payload, and special keys.
		rs := newParityRS(map[string]any{"service.name": "svc", "deployment.environment.name": "prod", "service.version": "1.0.0"})
		ss := addParityScope(rs, "lib", "1.0", nil)
		tid := parityTraceID(0x77, 1200)
		addParitySpan(ss, tid, 1201, 0, map[string]any{
			"deployment.environment": "stage", "_dd.hostname": "span-host", "_dd.origin": "synthetics",
			"_dd.git.commit.sha": "abc123", "_dd.apm_mode": "edge", "component": "net/http", "db.namespace": "dbns",
			"db.system": "postgresql", "peer.service": "Peer Svc!", "_dd.base_service": "Base Svc", "db.statement": "SELECT 1",
		})
		addParitySpan(ss, tid, 1202, 1201, map[string]any{
			"env": "", "version": "", "service.name": "", "component": "", "_dd.origin": "", "_dd.hostname": "",
			"_dd.git.commit.sha": "", "_dd.apm_mode": "", "deployment.environment": "",
		}, withKind(ptrace.SpanKindClient))
		addParitySpan(ss, tid, 1203, 1201, map[string]any{
			"service.name": "span-svc", "operation.name": "custom.op", "resource.name": "custom-res", "span.type": "custom-type",
			"db.name": "explicit-db", "db.namespace": "ignored", "_dd.origin": "rum", "_dd.git.commit.sha": "def456",
		})
		addParitySpan(ss, tid, 1204, 1201, map[string]any{
			"service.name": 5, "operation.name": 1.5, "resource.name": true, "span.type": []any{"a"}, "_dd.hostname": 3,
		})
		addParitySpan(ss, parityTraceID(0, 1210), 1211, 0, map[string]any{"_dd.apm_mode": "other", "env": "e2"})
		add("promoted Datadog fields", rs)
	}
	{ // Timestamps, including starts before year 2000 and negative durations.
		rs := newParityRS(baseRes)
		ss := addParityScope(rs, "lib", "1.0", nil)
		tid := parityTraceID(0, 1300)
		addParitySpan(ss, tid, 1301, 0, nil, withTimes(0, 0))
		addParitySpan(ss, tid, 1302, 1301, nil, withTimes(12345, 20000))
		addParitySpan(ss, tid, 1303, 1301, nil, withTimes(parityTS, parityTS-1))
		addParitySpan(ss, tid, 1304, 1301, nil, withTimes(math.MaxUint64, 1))
		addParitySpan(ss, tid, 1305, 1301, nil, withTimes(parityTS, math.MaxUint64))
		add("timestamps", rs)
	}
	{ // An empty resource with no scope spans, and a scope without spans.
		add("empty", newParityRS(nil))
		rs := newParityRS(baseRes)
		addParityScope(rs, "lib", "1.0", nil)
		add("scope without spans", rs)
	}
	return cases
}

// ---------------------------------------------------------------------------------------------
// Fuzz input generator
// ---------------------------------------------------------------------------------------------

// parityFuzzKeys are the attribute keys the fuzz generator picks from, besides random keys: the
// keys the OTLP conversion and ConvertToIdx interpret, and the agent keys of the dupcheck corpus.
var parityFuzzKeys = func() []string {
	keys := []string{
		"service.name", "operation.name", "resource.name", "span.type", "sampling.priority", "_sampling_priority_v1",
		"_dd.p.dm", "w3c.tracestate", "_sample_rate", "_dd.measured", "_top_level", "analytics.event", "_dd1.sr.eausr",
		"env", "span.kind", "version", "component", "service.version", "deployment.environment", "deployment.environment.name",
		"db.namespace", "db.name", "db.system", "db.statement", "peer.service", "_dd.base_service",
		"http.status_code", "http.response.status_code", "http.method", "http.request.method", "http.route", "url.full",
		"rpc.grpc.status_code", "rpc.system", "rpc.service", "rpc.method", "messaging.system", "messaging.operation",
		"_dd.hostname", "_dd.origin", "_dd.git.commit.sha", "_dd.apm_mode", "_dd.p.tid", "otel.trace_id",
		"error.msg", "error.type", "error.stack", "exception.message", "exception.type", "exception.stacktrace",
		"container.id", "k8s.pod.uid", "container.name", "k8s.container.name", "container.image.name",
		"telemetry.sdk.language", "telemetry.sdk.version", "host.name", "datadog.host.name", "_dd.stats_computed",
		"otel.status_code", "otel.status_description", "otel.library.name", "otel.scope.name",
		"_dd.span_events.has_exception", "dupcheck.random.a", "a", "a.b", "",
	}
	keys = append(keys, dupcheck.CorpusAgentKeys...)
	slices.Sort(keys)
	return slices.Compact(keys)
}()

var parityFuzzStrings = []string{
	"", "x", "prod", "server", "client", "internal", "producer", "consumer", "-4", "-12", "12", "-", "abc",
	"true", "false", "1", "0", "2", "-1", "0.5", "200", "404", "503", "GET", "Peer Svc!", "net/http", "edge",
	"synthetics", "a.b", " ", "üñî", "\x00", "dd=s:2;t.dm:-12", "SELECT * FROM t WHERE a = 1",
	"00000000000000aa0000000000000001", "aa", strings.Repeat("long", 40),
}

var parityFuzzTraceStates = []string{
	"", "dd=s:2;t.dm:-12", "dd=t.dm:-4;s:1,foo=bar", "foo=bar,dd=s:1;t.dm:-3", "dd=t.dm:-x", "dd=s:-1",
	"ot=th:8", "ot=th:c;rv:abcdef01234567,dd=s:1", "ot=th:0", "ot=th:fffffffffffffff", "foo=bar", "dd=", ",,,",
}

var parityFuzzInts = []int64{0, 1, 2, -1, -3, 3, 5, 12, 200, 404, 418, 500, 503, 999, 1 << 40, math.MaxInt64, math.MinInt64}

var parityFuzzDoubles = []float64{0, 0.5, 1, 2, -1, 1.5, 200, 404, 0.25, math.NaN(), math.Inf(1), math.Inf(-1), math.Copysign(0, -1), 1e300, -1e-300}

var parityFuzzSpanNames = []string{"", "op", "GET /users", "opentelemetry.server", "internal", "SELECT", "a b c", "💥"}

// parityFuzzReader consumes fuzz bytes; once they are exhausted it returns zeros.
type parityFuzzReader struct {
	data []byte
	pos  int
}

func (r *parityFuzzReader) byte() byte {
	if r.pos >= len(r.data) {
		return 0
	}
	b := r.data[r.pos]
	r.pos++
	return b
}

func (r *parityFuzzReader) intn(n int) int {
	if n <= 1 {
		return 0
	}
	if n <= 256 {
		return int(r.byte()) % n
	}
	return int(uint16(r.byte())<<8|uint16(r.byte())) % n
}

func (r *parityFuzzReader) bool() bool { return r.byte()&1 == 1 }

func (r *parityFuzzReader) u32() uint32 {
	return uint32(r.byte())<<24 | uint32(r.byte())<<16 | uint32(r.byte())<<8 | uint32(r.byte())
}

func (r *parityFuzzReader) u64() uint64 { return uint64(r.u32())<<32 | uint64(r.u32()) }

func (r *parityFuzzReader) rawBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = r.byte()
	}
	return b
}

func (r *parityFuzzReader) str() string {
	if r.intn(4) == 0 {
		return string(r.rawBytes(r.intn(10)))
	}
	return parityFuzzStrings[r.intn(len(parityFuzzStrings))]
}

func (r *parityFuzzReader) key() string {
	if r.intn(5) == 0 {
		return string(r.rawBytes(r.intn(6)))
	}
	return parityFuzzKeys[r.intn(len(parityFuzzKeys))]
}

func (r *parityFuzzReader) int64() int64 {
	if r.intn(4) == 0 {
		return int64(r.u64())
	}
	return parityFuzzInts[r.intn(len(parityFuzzInts))]
}

func (r *parityFuzzReader) double() float64 {
	if r.intn(4) == 0 {
		return math.Float64frombits(r.u64())
	}
	return parityFuzzDoubles[r.intn(len(parityFuzzDoubles))]
}

// scalar sets v to a scalar of the given type: 0 str, 1 int, 2 double, 3 bool, 4 bytes.
func (r *parityFuzzReader) scalar(v pcommon.Value, typ int) {
	switch typ {
	case 0:
		v.SetStr(r.str())
	case 1:
		v.SetInt(r.int64())
	case 2:
		v.SetDouble(r.double())
	case 3:
		v.SetBool(r.bool())
	default:
		v.SetEmptyBytes().FromRaw(r.rawBytes(r.intn(6)))
	}
}

// value sets v to a random value of any type.
func (r *parityFuzzReader) value(v pcommon.Value, depth int) {
	switch t := r.intn(12); {
	case t <= 5:
		// Strings are the most frequent: they cover most of the conversion logic.
		r.scalar(v, max(0, t-1))
	case t == 6: // homogeneous slice
		s := v.SetEmptySlice()
		typ := r.intn(5)
		for range r.intn(4) {
			r.scalar(s.AppendEmpty(), typ)
		}
	case t == 7 || t == 8: // mixed or nested slice
		s := v.SetEmptySlice()
		for range r.intn(4) {
			if depth < 2 {
				r.value(s.AppendEmpty(), depth+1)
			} else {
				r.scalar(s.AppendEmpty(), r.intn(5))
			}
		}
	case t == 9 || t == 10:
		m := v.SetEmptyMap()
		if depth < 2 {
			r.attributes(m, r.intn(4), depth+1)
		}
	default:
		// Leave v empty.
	}
}

func (r *parityFuzzReader) attributes(m pcommon.Map, n, depth int) {
	for range n {
		r.value(m.PutEmpty(r.key()), depth)
	}
}

func (r *parityFuzzReader) timestamps(s ptrace.Span) {
	var start uint64
	switch r.intn(16) {
	case 14: // before year 2000
		start = uint64(r.u32()) * uint64(r.intn(3))
	case 15: // anything, including values that overflow int64
		start = r.u64()
	default:
		start = parityTS + uint64(r.u32())*1000
	}
	var end uint64
	switch r.intn(8) {
	case 0:
		end = 0
	case 1:
		end = start - uint64(r.intn(100))
	case 2:
		end = r.u64()
	default:
		end = start + uint64(r.u32())
	}
	s.SetStartTimestamp(pcommon.Timestamp(start))
	s.SetEndTimestamp(pcommon.Timestamp(end))
}

// parityFuzzInput decodes data into an OTLP ResourceSpans, the index of a parity config and the
// client computed stats header value.
func parityFuzzInput(data []byte, numConfigs int) (rs ptrace.ResourceSpans, configIndex int, clientComputedStats bool) {
	r := &parityFuzzReader{data: data}
	configIndex = r.intn(numConfigs)
	clientComputedStats = r.intn(4) == 0
	rs = ptrace.NewResourceSpans()
	r.attributes(rs.Resource().Attributes(), r.intn(7), 1)

	var traceIDs [][16]byte
	var spanIDs []uint64
	newID := func() uint64 {
		if r.intn(4) == 0 {
			return uint64(1 + r.intn(4)) // small IDs, to make collisions
		}
		return r.u64()
	}
	totalSpans := 0
	for range 1 + r.intn(3) {
		ss := rs.ScopeSpans().AppendEmpty()
		ss.Scope().SetName(r.str())
		ss.Scope().SetVersion(r.str())
		r.attributes(ss.Scope().Attributes(), r.intn(4)*r.intn(2), 1)
		for range 1 + r.intn(5) {
			if totalSpans >= 24 {
				break
			}
			totalSpans++
			s := ss.Spans().AppendEmpty()

			var tid [16]byte
			switch mode := r.intn(4); {
			case mode == 0 && len(traceIDs) > 0: // same trace
				tid = traceIDs[r.intn(len(traceIDs))]
			case mode == 1 && len(traceIDs) > 0: // same low bits, other high bits
				tid = traceIDs[r.intn(len(traceIDs))]
				binary.BigEndian.PutUint64(tid[:8], r.u64())
			default:
				if r.bool() {
					binary.BigEndian.PutUint64(tid[:8], r.u64())
				}
				binary.BigEndian.PutUint64(tid[8:], newID())
			}
			traceIDs = append(traceIDs, tid)
			s.SetTraceID(tid)

			sid := newID()
			s.SetSpanID(paritySpanID(sid))
			switch r.intn(3) {
			case 0: // root: all-zero parent ID
			case 1:
				if len(spanIDs) > 0 {
					s.SetParentSpanID(paritySpanID(spanIDs[r.intn(len(spanIDs))]))
				}
			default:
				s.SetParentSpanID(paritySpanID(newID()))
			}
			spanIDs = append(spanIDs, sid)

			if r.intn(3) == 0 {
				s.SetName(r.str())
			} else {
				s.SetName(parityFuzzSpanNames[r.intn(len(parityFuzzSpanNames))])
			}
			s.SetKind(ptrace.SpanKind(r.intn(7))) // includes one invalid kind
			r.timestamps(s)
			s.Status().SetCode(ptrace.StatusCode(r.intn(3)))
			if r.bool() {
				s.Status().SetMessage(r.str())
			}
			if r.bool() {
				s.TraceState().FromRaw(parityFuzzTraceStates[r.intn(len(parityFuzzTraceStates))])
			}
			r.attributes(s.Attributes(), r.intn(10), 0)
			for range r.intn(3) {
				e := s.Events().AppendEmpty()
				if r.bool() {
					e.SetName("exception")
				} else {
					e.SetName(r.str())
				}
				e.SetTimestamp(pcommon.Timestamp(r.u64()))
				r.attributes(e.Attributes(), r.intn(5), 0)
			}
			for range r.intn(3) {
				l := s.Links().AppendEmpty()
				var ltid [16]byte
				if r.bool() {
					binary.BigEndian.PutUint64(ltid[:8], r.u64())
					binary.BigEndian.PutUint64(ltid[8:], r.u64())
				}
				l.SetTraceID(ltid)
				l.SetSpanID(paritySpanID(newID()))
				l.TraceState().FromRaw(parityFuzzTraceStates[r.intn(len(parityFuzzTraceStates))])
				switch r.intn(4) {
				case 0:
				case 1:
					l.SetFlags(1)
				case 2:
					l.SetFlags(0x301)
				default:
					l.SetFlags(r.u32())
				}
				r.attributes(l.Attributes(), r.intn(4), 0)
			}
		}
	}
	return rs, configIndex, clientComputedStats
}

// ---------------------------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------------------------

func TestOTLPIdxParityCorpus(t *testing.T) {
	configs := otlpIdxParityConfigs(t)
	cases := handWrittenParityCases()
	var batches [][]dupcheck.TraceSpec
	for _, seed := range []int64{1, 42} {
		batches = append(batches, dupcheck.NewCorpus(seed).Batches(8)...)
	}
	for _, pc := range configs {
		t.Run(pc.name, func(t *testing.T) {
			t.Run("hand-written", func(t *testing.T) {
				for _, c := range cases {
					t.Run(c.name, func(t *testing.T) {
						requireIdxParity(t, pc, c.rs, false)
						if !pc.spanOnly {
							requireReceiverIdxParity(t, pc.conf, c.rs, true)
						}
					})
				}
			})
			t.Run("dupcheck", func(t *testing.T) {
				for i, b := range batches {
					t.Run(strconv.Itoa(i), func(t *testing.T) {
						requireIdxParity(t, pc, dupcheck.ResourceSpans(b), false)
					})
				}
			})
		})
	}
	t.Run("generated", func(t *testing.T) {
		// Pseudo-random inputs of the fuzz generator, so that the regular test run covers more
		// than the fuzz seed corpus.
		rng := rand.New(rand.NewSource(1))
		for i := range 300 {
			data := make([]byte, 64+rng.Intn(1024))
			rng.Read(data)
			rs, ci, ccs := parityFuzzInput(data, len(configs))
			pc := configs[ci]
			t.Run(fmt.Sprintf("%d/%s", i, pc.name), func(t *testing.T) {
				requireIdxParity(t, pc, rs, ccs)
			})
		}
	})
}

func FuzzOTLPIdxParity(f *testing.F) {
	configs := otlpIdxParityConfigs(f)
	f.Add([]byte{})
	f.Add([]byte{0, 0, 3, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	rng := rand.New(rand.NewSource(7))
	for range 8 {
		data := make([]byte, 128+rng.Intn(512))
		rng.Read(data)
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		rs, ci, ccs := parityFuzzInput(data, len(configs))
		pc := configs[ci]
		t.Logf("config: %s", pc.name)
		requireIdxParity(t, pc, rs, ccs)
	})
}

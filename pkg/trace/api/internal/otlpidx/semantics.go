// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otlpidx

import (
	"encoding/hex"
	"math"
	"net/http"
	"strconv"
	"sync/atomic"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
	normalizeutil "github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
	"github.com/DataDog/datadog-agent/pkg/trace/transform"
)

// attrAccessor implements semantics.Accessor over idx attribute maps. Span attributes take
// precedence over resource attributes, and values are only returned for the matching type, like
// semantics.OTelSpanAccessor over pdata.
type attrAccessor struct {
	strings *idx.StringTable
	keys    *keyFilter
	span    map[uint32]*idx.AnyValue
	res     map[uint32]*idx.AnyValue
}

// lookup returns the string reference of an attribute key, or 0 if no attribute has that key.
func (a attrAccessor) lookup(key string) uint32 {
	if !a.keys.mayContain(keySig(key)) {
		return 0
	}
	return a.strings.Lookup(key)
}

// keyFilter is a bitset over a cheap signature of the attribute keys decoded in a payload. Each
// span runs dozens of registry lookups, most of them for keys the payload doesn't have, and the
// filter rejects most of those without hashing the key into the string table.
type keyFilter [keyFilterBits / 64]uint64

const keyFilterBits = 1024

// keySig derives a key's filter bit from its length and first, middle and last bytes.
func keySig[T string | []byte](k T) uint {
	if len(k) == 0 {
		return 0
	}
	h := uint32(len(k)) | uint32(k[0])<<8 | uint32(k[len(k)/2])<<16 | uint32(k[len(k)-1])<<24
	return uint((h * 0x9E3779B1) >> (32 - 10))
}

func (f *keyFilter) add(sig uint) { f[sig/64] |= 1 << (sig % 64) }

func (f *keyFilter) mayContain(sig uint) bool { return f[sig/64]&(1<<(sig%64)) != 0 }

// addConcept adds the attribute keys the concept's lookup reads, including the attributes of its
// conditions.
func (f *keyFilter) addConcept(reg semantics.Registry, concept semantics.Concept) {
	for _, t := range reg.GetAttributePrecedence(concept) {
		f.add(keySig(t.Name))
		for _, c := range t.When {
			f.add(keySig(c.Attribute))
		}
	}
}

// intersects reports whether the two filters may have a key in common. When a payload's filter
// doesn't intersect a concept's, the concept lookup can't find anything.
func (f *keyFilter) intersects(o *keyFilter) bool {
	for i := range f {
		if f[i]&o[i] != 0 {
			return true
		}
	}
	return false
}

// conceptFilters holds the filter bits of the attribute keys read by the lookups of concepts that
// spanSemantics skips when none of their keys can be present, built once per registry.
type conceptFilters struct {
	fingerprint                               string
	service, env, version, httpCode, grpcCode keyFilter
}

var cachedConceptFilters atomic.Pointer[conceptFilters]

func conceptFiltersFor(reg semantics.Registry) *conceptFilters {
	fp := reg.Fingerprint()
	if cf := cachedConceptFilters.Load(); cf != nil && cf.fingerprint == fp {
		return cf
	}
	cf := &conceptFilters{fingerprint: fp}
	cf.service.addConcept(reg, semantics.ConceptServiceName)
	cf.env.addConcept(reg, semantics.ConceptDeploymentEnv)
	cf.version.addConcept(reg, semantics.ConceptServiceVersion)
	cf.httpCode.addConcept(reg, semantics.ConceptHTTPStatusCode)
	cf.grpcCode.addConcept(reg, semantics.ConceptGRPCStatusCode)
	cachedConceptFilters.Store(cf)
	return cf
}

func (a attrAccessor) GetString(key string) string {
	ref := a.lookup(key)
	if ref == 0 {
		return ""
	}
	if v, ok := a.span[ref].GetValue().(*idx.AnyValue_StringValueRef); ok && v.StringValueRef != 0 {
		return a.strings.Get(v.StringValueRef)
	}
	if v, ok := a.res[ref].GetValue().(*idx.AnyValue_StringValueRef); ok {
		return a.strings.Get(v.StringValueRef)
	}
	return ""
}

func (a attrAccessor) GetInt64(key string) (int64, bool) {
	ref := a.lookup(key)
	if ref == 0 {
		return 0, false
	}
	if v, ok := a.span[ref].GetValue().(*idx.AnyValue_IntValue); ok {
		return v.IntValue, true
	}
	if v, ok := a.res[ref].GetValue().(*idx.AnyValue_IntValue); ok {
		return v.IntValue, true
	}
	return 0, false
}

func (a attrAccessor) GetFloat64(key string) (float64, bool) {
	ref := a.lookup(key)
	if ref == 0 {
		return 0, false
	}
	if v, ok := a.span[ref].GetValue().(*idx.AnyValue_DoubleValue); ok {
		return v.DoubleValue, true
	}
	if v, ok := a.res[ref].GetValue().(*idx.AnyValue_DoubleValue); ok {
		return v.DoubleValue, true
	}
	return 0, false
}

// stringOnlyAccessor hides numeric attributes, matching semantics.StringMapAccessor over a v04
// span's Meta.
type stringOnlyAccessor struct{ attrAccessor }

func (stringOnlyAccessor) GetInt64(string) (int64, bool)     { return 0, false }
func (stringOnlyAccessor) GetFloat64(string) (float64, bool) { return 0, false }

// spanView implements transform.SpanKindNamer for a span being decoded.
type spanView struct {
	kind ptrace.SpanKind
	name string
}

func (s spanView) Kind() ptrace.SpanKind { return s.kind }
func (s spanView) Name() string          { return s.name }

// rename moves an attribute from src to the Datadog key dst, or drops it when dst is empty.
type rename struct {
	src uint32
	dst string
}

// renameFor reports whether the Datadog key mapping (transform.GetDDKeyForOTLPAttribute)
// renames or drops the attribute key k. Most keys are kept as-is, so those are remembered in a
// bitset indexed by string reference to avoid re-mapping keys repeated across spans.
func (d *decoder) renameFor(k uint32) (rename, bool) {
	w, bit := k/64, uint64(1)<<(k%64)
	if int(w) < len(d.keptKeys) && d.keptKeys[w]&bit != 0 {
		return rename{}, false
	}
	key := d.strings.Get(k)
	mapped := transform.GetDDKeyForOTLPAttribute(key)
	if mapped == key {
		for int(w) >= len(d.keptKeys) {
			d.keptKeys = append(d.keptKeys, 0)
		}
		d.keptKeys[w] |= bit
		return rename{}, false
	}
	return rename{src: k, dst: mapped}, true
}

// applyRenames applies renames to attrs. When the Datadog key is already present it wins and the
// OTel key is dropped.
func (d *decoder) applyRenames(attrs map[uint32]*idx.AnyValue, renames []rename) {
	for _, r := range renames {
		v, ok := attrs[r.src]
		if !ok {
			continue
		}
		delete(attrs, r.src)
		if r.dst == "" {
			continue
		}
		dk := d.strings.Add(r.dst)
		if _, exists := attrs[dk]; !exists {
			attrs[dk] = v
		}
	}
}

// constRefs caches the string references of keys added to every span.
type constRefs struct {
	traceID, scopeName, libraryName, scopeVersion, libraryVersion, statusCode uint32
}

// constRef interns s once per payload, caching its reference in *ref.
func (d *decoder) constRef(ref *uint32, s string) uint32 {
	if *ref == 0 {
		*ref = d.strings.Add(s)
	}
	return *ref
}

type resourceInfo struct {
	serviceRef uint32
	// service, env and version are the span-level values when the span itself has none of the
	// attributes they are looked up from.
	service string
	env     string
	version string
	// payloadEnv is the payload-level env, read from the full resource.
	payloadEnv    string
	containerID   string
	lang          string
	sdkVersion    string
	hostname      string
	containerTags string
	source        source.Source
	statsComputed *bool
	// promoted holds the resource attributes with a promoted key, which spans fall back to.
	promoted promoted
	// analyticsRate is the event extraction rate from the resource's analytics.event attribute.
	analyticsRate    float64
	hasAnalyticsRate bool
}

type scopeInfo struct {
	nameRef    uint32
	versionRef uint32
	// attrs are the scope attributes without a promoted key, in their native type.
	attrs []resMerge
	// promoted holds the scope attributes with a promoted key.
	promoted promoted
}

// Bits of the attribute keys that ConvertToIdx moves onto span fields (isPromotedTag).
const (
	promotedEnv uint8 = 1 << iota
	promotedVersion
	promotedComponent
	promotedKind
)

var promotedKeys = [...]struct {
	name string
	bit  uint8
}{
	{"env", promotedEnv},
	{"version", promotedVersion},
	{"component", promotedComponent},
	{"span.kind", promotedKind},
}

// promotedKey returns the bit of a promoted attribute key, or 0.
func promotedKey(k string) uint8 {
	for _, p := range promotedKeys {
		if p.name == k {
			return p.bit
		}
	}
	return 0
}

// promoted holds the values of promoted attribute keys, which set span fields instead of
// attributes.
type promoted struct {
	set                     uint8
	env, version, component uint32
	kind                    idx.SpanKind
}

// put records the value ref, whose string is s, for the promoted key bit.
func (p *promoted) put(bit uint8, ref uint32, s string) {
	p.set |= bit
	switch bit {
	case promotedEnv:
		p.env = ref
	case promotedVersion:
		p.version = ref
	case promotedComponent:
		p.component = ref
	case promotedKind:
		p.kind = spanKindFromName(s)
	}
}

// spanKindFromName mirrors ConvertToIdx: unknown names map to internal, the OTel default.
func spanKindFromName(s string) idx.SpanKind {
	switch s {
	case "server":
		return idx.SpanKind_SPAN_KIND_SERVER
	case "client":
		return idx.SpanKind_SPAN_KIND_CLIENT
	case "producer":
		return idx.SpanKind_SPAN_KIND_PRODUCER
	case "consumer":
		return idx.SpanKind_SPAN_KIND_CONSUMER
	}
	return idx.SpanKind_SPAN_KIND_INTERNAL
}

// decodeScopeAttribute decodes the serialized KeyValue kv of an InstrumentationScope attribute.
func (d *decoder) decodeScopeAttribute(lib *scopeInfo, kv []byte) error {
	d.keys = &d.scopeKeys
	k, v, err := d.decodeKeyValue(kv, 0)
	if err != nil {
		return err
	}
	bit := promotedKey(d.strings.Get(k))
	if bit == 0 {
		lib.attrs = append(lib.attrs, resMerge{key: k, val: v})
		return nil
	}
	// Span fields are strings: use the string form today's conversion stores scope attributes in.
	if sv, ok := v.Value.(*idx.AnyValue_StringValueRef); ok {
		lib.promoted.put(bit, sv.StringValueRef, d.strings.Get(sv.StringValueRef))
		return nil
	}
	s, err := attrAsString(kv)
	if err != nil {
		return err
	}
	lib.promoted.put(bit, d.strings.Add(s), s)
	return nil
}

// applyScope copies the scope attributes onto a span. Like transform.OtelSpanToDDSpan, this runs
// last and overwrites span, resource and computed attributes, without key mapping. Unlike it, the
// values keep their native type rather than being stringified.
func (d *decoder) applyScope(span *idx.Span, lib *scopeInfo) {
	for _, a := range lib.attrs {
		span.Attributes[a.key] = d.cloneValue(a.val)
	}
	p := &lib.promoted
	if p.set&promotedEnv != 0 {
		span.EnvRef = p.env
	}
	if p.set&promotedVersion != 0 {
		span.VersionRef = p.version
	}
	if p.set&promotedComponent != 0 {
		span.ComponentRef = p.component
	}
	if p.set&promotedKind != 0 {
		span.Kind = p.kind
	}
}

// samplingPriority mirrors SetMetricOTLPIfEmpty for a numeric sampling.priority span attribute, which
// today's conversion stores as the _sampling_priority_v1 metric, the key the sampling priority concept
// reads (the chunk priority, user-set sampling mechanism and span attribute all follow from it).
// String values stay plain attributes, as today.
func (d *decoder) samplingPriority(attrs map[uint32]*idx.AnyValue) {
	const key, ddKey = "sampling.priority", "_sampling_priority_v1"
	if !d.spanKeys.mayContain(keySig(key)) {
		return
	}
	ref := d.strings.Lookup(key)
	v, ok := attrs[ref]
	if ref == 0 || !ok {
		return
	}
	var p float64
	switch x := v.Value.(type) {
	case *idx.AnyValue_IntValue:
		p = float64(x.IntValue)
	case *idx.AnyValue_DoubleValue:
		p = x.DoubleValue
	default:
		return
	}
	delete(attrs, ref)
	ddRef := d.strings.Add(ddKey)
	if _, exists := attrs[ddRef]; !exists {
		attrs[ddRef] = d.doubleValue(p)
		d.spanKeys.add(keySig(ddKey))
	}
}

// keyAnalyticsEvent is the attribute that today's conversion (SetMetaOTLPIfEmpty) turns into the
// event extraction rate metric.
const keyAnalyticsEvent = "analytics.event"

// analyticsRate parses an analytics.event value into the event extraction rate, like
// SetMetaOTLPIfEmpty.
func analyticsRate(s string) (float64, bool) {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return 0, false
	}
	if v {
		return 1, true
	}
	return 0, true
}

// analyticsEvent mirrors SetMetaOTLPIfEmpty for analytics.event: a non-numeric span attribute is
// replaced by the event extraction rate metric when it parses as a bool, and dropped otherwise.
// Without one, the resource's value applies. Numeric span attributes stay as they are, as today.
func (d *decoder) analyticsEvent(attrs map[uint32]*idx.AnyValue) {
	rate, ok := d.res.analyticsRate, d.res.hasAnalyticsRate
	if d.spanKeys.mayContain(keySig(keyAnalyticsEvent)) {
		ref := d.strings.Lookup(keyAnalyticsEvent)
		if v, has := attrs[ref]; ref != 0 && has {
			var s string
			switch x := v.Value.(type) {
			case *idx.AnyValue_IntValue, *idx.AnyValue_DoubleValue:
				return
			case *idx.AnyValue_StringValueRef:
				s = d.strings.Get(x.StringValueRef)
			case *idx.AnyValue_BoolValue:
				s = strconv.FormatBool(x.BoolValue)
			}
			delete(attrs, ref)
			if r, parsed := analyticsRate(s); parsed {
				rate, ok = r, true
			}
		}
	}
	if ok {
		attrs[d.strings.Add(sampler.KeySamplingRateEventExtraction)] = d.doubleValue(rate)
	}
}

// takeSpanPromoted removes the span attributes with a promoted key and returns their values.
// Like ConvertToIdx, numeric values (v04 Metrics) are dropped, and today's conversion ignores a
// span.kind attribute in favour of the OTel span kind.
func (d *decoder) takeSpanPromoted(attrs map[uint32]*idx.AnyValue) promoted {
	var p promoted
	for _, k := range promotedKeys {
		if !d.spanKeys.mayContain(keySig(k.name)) {
			continue
		}
		ref := d.strings.Lookup(k.name)
		v, ok := attrs[ref]
		if ref == 0 || !ok {
			continue
		}
		delete(attrs, ref)
		if k.bit == promotedKind {
			continue
		}
		switch x := v.Value.(type) {
		case *idx.AnyValue_StringValueRef:
			p.put(k.bit, x.StringValueRef, "")
		case *idx.AnyValue_BoolValue:
			p.put(k.bit, d.strings.Add(strconv.FormatBool(x.BoolValue)), "")
		}
	}
	return p
}

// resourceAccessor returns an accessor over the resource attributes alone.
func (d *decoder) resourceAccessor() attrAccessor {
	d.lookupKeys = d.resKeys
	return attrAccessor{strings: d.strings, keys: &d.lookupKeys, res: d.resAttrs}
}

// payloadSemantics computes the payload-level values derived from resource attributes.
func (d *decoder) payloadSemantics() {
	acc := d.resourceAccessor()
	d.res.payloadEnv = transform.LookupSemanticStringWithAccessor(acc, semantics.ConceptDeploymentEnv, true)
	d.res.containerID = transform.LookupSemanticStringWithAccessor(acc, semantics.ConceptContainerID, true)
	d.res.lang = normalizeutil.NormalizeTagValue(acc.GetString("telemetry.sdk.language"))
	d.res.sdkVersion = normalizeutil.NormalizeTagValue(acc.GetString("telemetry.sdk.version"))
}

// spanDefaults computes the resource-level values spans fall back to.
func (d *decoder) spanDefaults() {
	acc := d.resourceAccessor()
	if v, ok := d.resAttrs[d.strings.Lookup("service.name")].GetValue().(*idx.AnyValue_StringValueRef); ok {
		d.res.serviceRef = v.StringValueRef
	}
	d.res.service = transform.GetOTelServiceWithAccessor(acc, true)
	d.res.env = transform.LookupSemanticStringWithAccessor(acc, semantics.ConceptDeploymentEnv, true)
	d.res.version = transform.LookupSemanticStringWithAccessor(acc, semantics.ConceptServiceVersion, true)
}

// spanSemantics maps an OTel span onto Datadog conventions, mirroring transform.OtelSpanToDDSpan
// on the ReceiveResourceSpansV2 path (see Supported).
func (d *decoder) spanSemantics(span *idx.Span, raw *rawSpan, lib scopeInfo) {
	// Resource attributes are only merged into the span after this, so lookups see the span's
	// own attributes and the typed resource attributes, like transform.OtelSpanToDDSpan.
	attrs := span.Attributes
	d.samplingPriority(attrs)
	for i := range d.lookupKeys {
		d.lookupKeys[i] = d.spanKeys[i] | d.resKeys[i]
	}
	acc := attrAccessor{strings: d.strings, keys: &d.lookupKeys, span: attrs, res: d.resAttrs}
	view := spanView{kind: ptrace.SpanKind(span.Kind), name: d.strings.Get(span.NameRef)}
	reg := semantics.DefaultRegistry()
	cf := conceptFiltersFor(reg)

	// Semantic lookups use the original OTel keys, so they run before the key renames below.
	// Service, env and version usually come from the resource. When the span has none of their
	// attributes, the lookup can only find what resourceSemantics already found.
	service, env, version := d.res.service, d.res.env, d.res.version
	if d.spanKeys.intersects(&cf.service) {
		service = transform.GetOTelServiceWithAccessor(acc, true)
	}
	if d.spanKeys.intersects(&cf.env) {
		env = transform.LookupSemanticStringWithAccessor(acc, semantics.ConceptDeploymentEnv, true)
	}
	if d.spanKeys.intersects(&cf.version) {
		version = transform.LookupSemanticStringWithAccessor(acc, semantics.ConceptServiceVersion, true)
	}
	name := transform.GetOTelOperationNameV2WithAccessor(view, acc)
	name, _ = normalizeutil.NormalizeName(name)
	resource := transform.GetOTelResourceV2WithAccessor(view, acc)
	typ := transform.GetOTelSpanTypeWithAccessor(view, acc)
	var httpCode int64
	var hasHTTPCode bool
	if d.lookupKeys.intersects(&cf.httpCode) {
		httpCode, hasHTTPCode = semantics.LookupInt64(reg, acc, semantics.ConceptHTTPStatusCode)
	}
	var grpcCode string
	if d.lookupKeys.intersects(&cf.grpcCode) {
		grpcCode = semantics.LookupString(reg, acc, semantics.ConceptGRPCStatusCode)
	}
	measured, _ := semantics.LookupInt64(reg, acc, semantics.ConceptDDMeasured)
	// db.name falls back to db.namespace unless the span or the resource (copied to the span
	// first by today's conversion) has it.
	var dbName string
	dbNameRef := d.strings.Lookup("db.name")
	_, spanHas := attrs[dbNameRef]
	_, resHas := d.resAttrs[dbNameRef]
	if dbNameRef == 0 || !spanHas && !resHas {
		dbName = transform.LookupSemanticStringWithAccessor(acc, semantics.ConceptDBNamespace, false)
	}
	if p, ok := semantics.LookupFloat64(reg, acc, semantics.ConceptSamplingPriority); ok {
		if d.priorities == nil {
			d.priorities = make(map[[16]byte]int32)
		}
		d.priorities[raw.traceID] = int32(p)
	}

	// Promoted keys set span fields. Today's conversion lets the span's own env and version win
	// over the semantic lookups, which win over the resource's.
	sp, rp := d.takeSpanPromoted(attrs), &d.res.promoted
	switch {
	case sp.set&promotedEnv != 0:
		env = d.strings.Get(sp.env)
	case env == "" && rp.set&promotedEnv != 0:
		env = d.strings.Get(rp.env)
	}
	switch {
	case sp.set&promotedVersion != 0:
		version = d.strings.Get(sp.version)
	case version == "" && rp.set&promotedVersion != 0:
		version = d.strings.Get(rp.version)
	}
	switch {
	case sp.set&promotedComponent != 0:
		span.ComponentRef = sp.component
	case rp.set&promotedComponent != 0:
		span.ComponentRef = rp.component
	}

	d.applyRenames(attrs, d.renames)
	d.analyticsEvent(attrs)

	span.ServiceRef = d.strings.Add(service)
	span.NameRef = d.strings.Add(name)
	span.ResourceRef = d.strings.Add(resource)
	span.TypeRef = d.strings.Add(typ)
	span.EnvRef = d.strings.Add(env)
	span.VersionRef = d.strings.Add(version)

	// Spans of a trace are usually adjacent, so reuse the previous span's trace ID string.
	if raw.traceID != d.lastTraceID || d.lastTraceIDRef == 0 {
		d.lastTraceID = raw.traceID
		d.lastTraceIDRef = d.strings.Add(hex.EncodeToString(raw.traceID[:]))
	}
	attrs[d.constRef(&d.refs.traceID, "otel.trace_id")] = d.stringValue(d.lastTraceIDRef)
	if hasHTTPCode && httpCode >= 0 {
		attrs[d.strings.Add("http.status_code")] = d.intValue(httpCode)
	}
	if grpcCode != "" {
		// Today's conversion emits the gRPC code both as a Meta string and, when the attribute is
		// numeric, as a Metric. V1 holds one value per key, so keep a native numeric attribute.
		if k := d.strings.Add(string(semantics.ConceptGRPCStatusCode)); attrs[k] == nil {
			attrs[k] = d.stringValue(d.strings.Add(grpcCode))
		}
	}
	// Mirrors the top-level and measured logic of transform.OtelSpanToDDSpan.
	if d.topLevelByKind {
		if span.ParentID == 0 || view.kind == ptrace.SpanKindServer || view.kind == ptrace.SpanKindConsumer {
			attrs[d.strings.Add("_top_level")] = d.doubleValue(1)
		}
	}
	if measured == 1 || d.topLevelByKind && (view.kind == ptrace.SpanKindClient || view.kind == ptrace.SpanKindProducer) {
		attrs[d.strings.Add("_dd.measured")] = d.doubleValue(1)
	}
	// Mirrors transform.SetSampleRateFromAttribute and SetSampleRateFromTracestate: a numeric
	// _sample_rate attribute wins, otherwise the head-sampling probability from the tracestate.
	if raw.tracestateRef != 0 {
		rateKey := d.strings.Add("_sample_rate")
		switch attrs[rateKey].GetValue().(type) {
		case *idx.AnyValue_IntValue, *idx.AnyValue_DoubleValue:
		default:
			// Spans of a trace usually share a tracestate, so reuse the previous decode.
			if ts := d.strings.Get(raw.tracestateRef); ts != d.lastTracestate || d.lastTracestate == "" {
				d.lastTracestate = ts
				d.lastSampleRate, d.lastSampleRateOK = transform.SampleRateFromTracestate(ts)
			}
			if d.lastSampleRateOK {
				attrs[rateKey] = d.doubleValue(d.lastSampleRate)
			}
		}
	}
	if raw.hasException {
		attrs[d.strings.Add("_dd.span_events.has_exception")] = d.stringValue(d.strings.Add("true"))
	}
	if lib.nameRef != 0 {
		if d.scopeConvention {
			attrs[d.constRef(&d.refs.scopeName, "otel.scope.name")] = d.stringValue(lib.nameRef)
		}
		attrs[d.constRef(&d.refs.libraryName, "otel.library.name")] = d.stringValue(lib.nameRef)
	}
	if lib.versionRef != 0 {
		if d.scopeConvention {
			attrs[d.constRef(&d.refs.scopeVersion, "otel.scope.version")] = d.stringValue(lib.versionRef)
		}
		attrs[d.constRef(&d.refs.libraryVersion, "otel.library.version")] = d.stringValue(lib.versionRef)
	}
	status := ptrace.StatusCode(raw.statusCode)
	attrs[d.constRef(&d.refs.statusCode, "otel.status_code")] = d.stringValue(d.strings.Add(status.String()))
	if raw.statusMsgRef != 0 {
		attrs[d.strings.Add("otel.status_description")] = d.stringValue(raw.statusMsgRef)
	}
	// Mirrors transform.Status2Error, which falls back to the HTTP status code read through a
	// string-only Meta accessor: numeric status codes (stored in Metrics) never qualify.
	if status == ptrace.StatusCodeError {
		span.Error = true
		errMsgKey := d.strings.Add("error.msg")
		if _, ok := attrs[errMsgKey]; !ok {
			if raw.statusMsgRef != 0 {
				attrs[errMsgKey] = d.stringValue(raw.statusMsgRef)
			} else if code := semantics.LookupString(reg, stringOnlyAccessor{acc}, semantics.ConceptHTTPStatusCode); code != "" {
				msg := code
				if codeInt, err := strconv.Atoi(code); err == nil {
					if text := http.StatusText(codeInt); text != "" {
						msg += " " + text
					}
				}
				attrs[errMsgKey] = d.stringValue(d.strings.Add(msg))
			}
		}
	}
	if dbName != "" {
		attrs[d.strings.Add("db.name")] = d.stringValue(d.strings.Add(dbName))
	}
	// Today's span.kind tag is "unspecified" for unspecified and unknown kinds, which ConvertToIdx maps
	// to internal, the OTel default. The lookups above still see the original kind, as they do today.
	if span.Kind < idx.SpanKind_SPAN_KIND_INTERNAL || span.Kind > idx.SpanKind_SPAN_KIND_CONSUMER {
		span.Kind = idx.SpanKind_SPAN_KIND_INTERNAL
	}
}

// knuthFactor is the multiplier of the OTLP receiver's probabilistic sampler. It must not
// change: it is shared with other probabilistic samplers in the agent, the Datadog libraries and
// OpenTelemetry, which keeps sampling decisions consistent across a distributed system.
const knuthFactor = uint64(1111111111111111111)

// SamplingRate returns the OTLP receiver's probabilistic sampling rate in [0, 1]. Like
// OTLPReceiver.samplingRate, rates outside (0, 1) keep every trace.
func SamplingRate(conf *config.AgentConfig) float64 {
	rate := conf.OTLPReceiver.ProbabilisticSampling / 100
	if rate <= 0 || rate >= 1 {
		return 1
	}
	return rate
}

// Sample returns the sampling priority the OTLP receiver applies to the trace whose lower 64 trace
// ID bits are tid, at the given rate. It must match OTLPReceiver.sample.
func Sample(tid uint64, rate float64) sampler.SamplingPriority {
	if rate == 1 {
		return sampler.PriorityAutoKeep
	}
	if tid*knuthFactor < uint64(rate*math.MaxUint64) {
		return sampler.PriorityAutoKeep
	}
	return sampler.PriorityAutoDrop
}

// Decision maker values the OTLP receiver records for a kept trace.
const (
	samplingMechanismManual        = 4 // a user-set sampling priority
	samplingMechanismProbabilistic = 9 // the OTLP receiver's probabilistic sampler
)

// finishPayload sets payload- and chunk-level fields once all spans are decoded.
func (d *decoder) finishPayload(tp *idx.InternalTracerPayload) {
	if d.opts.ResourceAttributesOnPayload {
		d.applyRenames(d.resAttrs, d.resRenames)
	}
	if d.opts.SkipSemantics {
		return
	}
	tp.SetHostname(d.res.hostname)
	if d.res.payloadEnv != "" {
		tp.SetEnv(d.res.payloadEnv)
	}
	if d.res.containerID != "" {
		tp.SetContainerID(d.res.containerID)
	}
	tp.SetLanguageName(d.res.lang)
	tp.SetTracerVersion("otlp-" + d.res.sdkVersion)
	if d.res.containerTags != "" {
		tp.SetStringAttribute("_dd.tags.container", d.res.containerTags)
	}
	// Mirrors OTLPReceiver.createChunks.
	rate := SamplingRate(d.opts.Conf)
	rateTag := strconv.FormatFloat(rate, 'f', 2, 64)
	for traceID, c := range d.chunks {
		c.SetStringAttribute("_dd.otlp_sr", rateTag)
		if d.opts.Conf.ProbabilisticSamplerEnabled {
			// The probabilistic and errors samplers decide later.
			c.Priority = int32(sampler.PriorityNone)
			continue
		}
		var priority sampler.SamplingPriority
		mechanism := uint32(samplingMechanismProbabilistic)
		if p, ok := d.priorities[traceID]; ok {
			priority, mechanism = sampler.SamplingPriority(p), samplingMechanismManual
		} else {
			priority = Sample(transform.OTelTraceIDToUint64(traceID), rate)
		}
		c.Priority = int32(priority)
		// Like _dd.p.dm, the decision maker is only recorded for a keep decision.
		if priority.IsKeep() {
			c.SetSamplingMechanism(mechanism)
		}
	}
}

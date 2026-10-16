// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otlpidx

import (
	"encoding/hex"
	"net/http"
	"strconv"
	"sync/atomic"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
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
	service     string
	env         string
	version     string
	containerID string
	lang        string
	sdkVersion  string
}

type scopeInfo struct {
	nameRef    uint32
	versionRef uint32
}

// resourceSemantics computes the payload-level values derived from resource attributes.
func (d *decoder) resourceSemantics() {
	d.lookupKeys = d.resKeys
	acc := attrAccessor{strings: d.strings, keys: &d.lookupKeys, res: d.resAttrs}
	if v, ok := d.resAttrs[d.strings.Lookup("service.name")].GetValue().(*idx.AnyValue_StringValueRef); ok {
		d.res.serviceRef = v.StringValueRef
	}
	d.res.service = transform.GetOTelServiceWithAccessor(acc, true)
	d.res.env = transform.LookupSemanticStringWithAccessor(acc, semantics.ConceptDeploymentEnv, true)
	d.res.version = transform.LookupSemanticStringWithAccessor(acc, semantics.ConceptServiceVersion, true)
	d.res.containerID = transform.LookupSemanticStringWithAccessor(acc, semantics.ConceptContainerID, true)
	d.res.lang = normalizeutil.NormalizeTagValue(acc.GetString("telemetry.sdk.language"))
	d.res.sdkVersion = normalizeutil.NormalizeTagValue(acc.GetString("telemetry.sdk.version"))
}

// spanSemantics maps an OTel span onto Datadog conventions, mirroring transform.OtelSpanToDDSpan
// under the default configuration (operation and resource name logic v2, receive resource spans
// v2, scope convention enabled).
func (d *decoder) spanSemantics(span *idx.Span, raw *rawSpan, lib scopeInfo) {
	res := d.resAttrs
	if d.opts.CopyResourceAttributesToSpans {
		res = nil // already merged into the span
	}
	attrs := span.Attributes
	for i := range d.lookupKeys {
		d.lookupKeys[i] = d.spanKeys[i] | d.resKeys[i]
	}
	acc := attrAccessor{strings: d.strings, keys: &d.lookupKeys, span: attrs, res: res}
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
	var dbName string
	if _, ok := attrs[d.strings.Lookup("db.name")]; !ok {
		dbName = transform.LookupSemanticStringWithAccessor(acc, semantics.ConceptDBNamespace, false)
	}
	if p, ok := semantics.LookupFloat64(reg, acc, semantics.ConceptSamplingPriority); ok {
		if d.priorities == nil {
			d.priorities = make(map[[16]byte]int32)
		}
		d.priorities[raw.traceID] = int32(p)
	}

	d.applyRenames(attrs, d.renames)

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
	if measured == 1 {
		attrs[d.strings.Add("_dd.measured")] = d.doubleValue(1)
	}
	if raw.hasException {
		attrs[d.strings.Add("_dd.span_events.has_exception")] = d.stringValue(d.strings.Add("true"))
	}
	if lib.nameRef != 0 {
		attrs[d.constRef(&d.refs.scopeName, "otel.scope.name")] = d.stringValue(lib.nameRef)
		attrs[d.constRef(&d.refs.libraryName, "otel.library.name")] = d.stringValue(lib.nameRef)
	}
	if lib.versionRef != 0 {
		attrs[d.constRef(&d.refs.scopeVersion, "otel.scope.version")] = d.stringValue(lib.versionRef)
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
}

// otlpSampleRate is the _dd.otlp_sr chunk tag value for the default probabilistic sampling rate
// (100%).
const otlpSampleRate = "1.00"

// finishPayload sets payload- and chunk-level fields once all spans are decoded.
func (d *decoder) finishPayload(tp *idx.InternalTracerPayload) {
	if !d.opts.CopyResourceAttributesToSpans {
		d.applyRenames(d.resAttrs, d.resRenames)
	}
	if d.opts.SkipSemantics {
		return
	}
	if d.res.env != "" {
		tp.SetEnv(d.res.env)
	}
	if d.res.containerID != "" {
		tp.SetContainerID(d.res.containerID)
	}
	tp.SetLanguageName(d.res.lang)
	tp.SetTracerVersion("otlp-" + d.res.sdkVersion)
	// Mirrors OTLPReceiver.createChunks with the default sampling configuration (100% rate,
	// probabilistic sampler disabled).
	for traceID, c := range d.chunks {
		c.SetStringAttribute("_dd.otlp_sr", otlpSampleRate)
		if p, ok := d.priorities[traceID]; ok {
			c.Priority = p
			if sampler.SamplingPriority(p).IsKeep() {
				c.SetSamplingMechanism(4)
			}
		} else {
			c.Priority = int32(sampler.PriorityAutoKeep)
			c.SetSamplingMechanism(9)
		}
	}
}

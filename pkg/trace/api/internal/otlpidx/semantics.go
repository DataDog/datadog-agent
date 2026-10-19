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
}

type scopeInfo struct {
	nameRef    uint32
	versionRef uint32
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

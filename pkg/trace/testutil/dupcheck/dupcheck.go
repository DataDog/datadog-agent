// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dupcheck is a test-only checker that finds string values the
// trace-agent writes to more than one location of the trace payloads it sends
// to the intake.
//
// Every check compares a stage input (what the agent received) with the stage
// output (what the agent would send). It looks at each pair of output
// locations holding the same value and reports it when the agent made them
// equal:
//   - attribute vs attribute: always compared;
//   - attribute vs top-level field: only compared when the attribute was
//     created or changed by the agent in that stage;
//   - top-level vs top-level: never compared.
//
// A pair whose two locations already held equal values in the stage input is a
// duplicate sent by the SDK and is never reported, even if the agent rewrote
// both locations. Only string values are checked: metrics, numeric attribute
// values and meta_struct are skipped.
//
// Nothing in this package knows about a specific integration: semantic
// synonyms come from the semantics registry (mappings.json).
package dupcheck

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
)

// MinLen is the minimum length of a value for the dup rule, so that short
// values such as "true", "GET" or "200" don't collide.
const MinLen = 8

// Rules reported by the checks.
const (
	// RuleDup reports two locations sharing a value of at least MinLen bytes.
	RuleDup = "dup"
	// RuleConceptDup reports two attributes whose keys belong to the same
	// semantic concept and share a value, whatever its length.
	RuleConceptDup = "concept-dup"
	// RuleCrossLevel reports the same key:value set in more than one tag map
	// (agent payload, tracer payload, trace chunk and span).
	RuleCrossLevel = "cross-level"
)

// Change kinds of a finding, describing what the agent did to the locations.
const (
	// ChangeCreated means that the agent created at least one of the locations.
	ChangeCreated = "created"
	// ChangeOverwritten means that the agent replaced the value of one location
	// with a copy of a location it left alone.
	ChangeOverwritten = "overwritten"
	// ChangeBothChanged means that the agent changed both locations, which held
	// different values in the stage input.
	ChangeBothChanged = "both-changed"
)

// Location identifies where a value is stored in a payload, for example
// "span.resource", "span.meta[sql.query]", "span.events[0].attributes[k]",
// or "chunk.tags[_dd.p.dm]".
type Location string

// Locations of the top-level span fields.
const (
	LocationService   Location = "span.service"
	LocationName      Location = "span.name"
	LocationResource  Location = "span.resource"
	LocationType      Location = "span.type"
	LocationEnv       Location = "span.env"
	LocationVersion   Location = "span.version"
	LocationComponent Location = "span.component"
	LocationKind      Location = "span.kind"
)

// MetaLocation returns the location of the v0.4 span meta entry key.
func MetaLocation(key string) Location {
	return Location("span.meta[" + key + "]")
}

// AttributeLocation returns the location of the v1 span attribute key.
func AttributeLocation(key string) Location {
	return Location("span.attributes[" + key + "]")
}

// Finding is an output location the agent made equal to other locations.
type Finding struct {
	// Stage is the pipeline stage that produced the finding, set by Report.Add.
	Stage string
	// Rule is one of the Rule* constants.
	Rule string
	// Change is one of the Change* constants.
	Change string
	// Value is the shared value, it may be long and is truncated when printed.
	Value string
	// Locations are the locations holding Value: the one the agent touched
	// first, then the locations it duplicates, which were equal to each other
	// in the stage input.
	Locations []Location
	// SpanType is the type of the span of the finding.
	SpanType string
	// SpanID is the ID of the span of the finding, 0 for payload and chunk findings.
	SpanID uint64
	// Service and Name are the service and name of the span of the finding,
	// empty for payload and chunk findings.
	Service, Name string
	// Context describes the span, payload or chunk to debug quickly.
	Context string
}

var (
	// indexPattern matches the index of a span event or link, not an attribute
	// key made of digits.
	indexPattern     = regexp.MustCompile(`\.(events|links)\[\d+\]`)
	randomKeyPattern = regexp.MustCompile(regexp.QuoteMeta(RandomKeyPrefix) + `[^\]]*`)
)

// normalizeLocation strips list indexes and the names of the random attributes
// of the generated corpus from l, so that equivalent locations compare equal.
func normalizeLocation(l Location) string {
	s := indexPattern.ReplaceAllString(string(l), ".$1[*]")
	return randomKeyPattern.ReplaceAllString(s, RandomKeyPrefix+"*")
}

// Fingerprint returns a key that groups equivalent findings and is stable
// across runs: it is made of the stage, the change kind, the rule, the first
// two locations, normalized, and the span type, but no value.
func (f Finding) Fingerprint() string {
	locations := make([]string, 0, 2)
	for _, l := range f.Locations[:min(2, len(f.Locations))] {
		locations = append(locations, normalizeLocation(l))
	}
	return fmt.Sprintf("[%s] %s %s %s type=%s", f.Stage, f.Change, f.Rule, strings.Join(locations, " == "), f.SpanType)
}

// level is the payload level a field belongs to.
type level string

const (
	spanLevel    level = "span"
	chunkLevel   level = "chunk"
	payloadLevel level = "payload"
	agentLevel   level = "agent"
)

// field is one string value of a payload.
type field struct {
	loc   Location
	value string
	// attribute is false for top-level fields.
	attribute bool
	// key is the attribute key, used to match semantic concepts and tag maps.
	key   string
	level level
	// tag reports whether the attribute lives in a tag map that is compared
	// across levels (span meta or attributes, chunk, payload and agent tags).
	tag bool
}

type fieldList []field

func (l *fieldList) topLevel(lvl level, loc Location, value string) {
	*l = append(*l, field{loc: loc, value: value, level: lvl})
}

func (l *fieldList) attribute(lvl level, loc Location, key, value string, tag bool) {
	*l = append(*l, field{loc: loc, value: value, attribute: true, key: key, level: lvl, tag: tag})
}

func (l *fieldList) stringMap(lvl level, prefix string, m map[string]string, tag bool) {
	for _, k := range slices.Sorted(maps.Keys(m)) {
		l.attribute(lvl, Location(prefix+"["+k+"]"), k, m[k], tag)
	}
}

// anyValueMap adds the string values of v1 attributes.
func (l *fieldList) anyValueMap(lvl level, prefix string, strs *idx.StringTable, attrs map[uint32]*idx.AnyValue, tag bool) {
	l.stringMap(lvl, prefix, stringValues(strs, attrs), tag)
}

// stringValues returns the string values of v1 attributes by key, other value
// types are skipped.
func stringValues(strs *idx.StringTable, attrs map[uint32]*idx.AnyValue) map[string]string {
	values := make(map[string]string, len(attrs))
	for keyRef, v := range attrs {
		if sv, ok := v.GetValue().(*idx.AnyValue_StringValueRef); ok {
			values[strs.Get(keyRef)] = strs.Get(sv.StringValueRef)
		}
	}
	return values
}

func concat(lists ...fieldList) fieldList {
	var out fieldList
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}

// state is what the agent did to a location in a stage.
type state int

const (
	unchanged state = iota
	changed
	created
)

// pairFilter restricts the pairs considered by compareFields.
type pairFilter func(a, b field) bool

// compareFields reports the out locations sharing a value that the agent made
// equal, following the pairing rule of the package documentation. Partners of
// a location that were equal to each other in the stage input are one
// duplicate sent by the SDK: they are reported as a single finding per rule
// listing all of them, the most telling one first. base provides the context
// copied into each finding.
func compareFields(in, out fieldList, keep pairFilter, base Finding) []Finding {
	inValues := make(map[Location]string, len(in))
	for _, f := range in {
		inValues[f.loc] = f.value
	}
	stateOf := func(f field) state {
		v, ok := inValues[f.loc]
		switch {
		case !ok:
			return created
		case v != f.value:
			return changed
		}
		return unchanged
	}
	type located struct {
		field
		state state
	}
	byValue := make(map[string][]located)
	var values []string
	for _, f := range out {
		if f.value == "" {
			continue
		}
		if _, ok := byValue[f.value]; !ok {
			values = append(values, f.value)
		}
		byValue[f.value] = append(byValue[f.value], located{f, stateOf(f)})
	}
	eligible := func(a, b located) bool {
		if !a.attribute && !b.attribute {
			return false
		}
		if keep != nil && !keep(a.field, b.field) {
			return false
		}
		if (!a.attribute && b.state == unchanged) || (!b.attribute && a.state == unchanged) {
			// An attribute the agent left alone is never compared with
			// top-level fields: a top-level field derived from it is a copy
			// made into the top-level field.
			return false
		}
		if ia, ok := inValues[a.loc]; ok {
			if ib, ok := inValues[b.loc]; ok && ia == ib {
				// Equal in the stage input: the SDK sent the duplicate.
				return false
			}
		}
		return classify(a.field, b.field, a.value) != ""
	}
	var findings []Finding
	for _, v := range values {
		group := byValue[v]
		slices.SortStableFunc(group, func(a, b located) int {
			switch {
			case touchedFirst(a.field, a.state, b.field, b.state):
				return -1
			case touchedFirst(b.field, b.state, a.field, a.state):
				return 1
			}
			return 0
		})
		for i, a := range group {
			if a.state == unchanged {
				// Only touched locations start a pair, and they sort first.
				break
			}
			classes := make(map[string][]located)
			var order []string
			for _, b := range group[i+1:] {
				if !eligible(a, b) {
					continue
				}
				class := "created\x00" + string(b.loc)
				if inValue, ok := inValues[b.loc]; ok {
					class = "input\x00" + inValue
				}
				if _, ok := classes[class]; !ok {
					order = append(order, class)
				}
				classes[class] = append(classes[class], b)
			}
			for _, class := range order {
				partners := classes[class]
				slices.SortStableFunc(partners, func(x, y located) int {
					if rx, ry := partnerRank(a.field, x.field), partnerRank(a.field, y.field); rx != ry {
						return rx - ry
					}
					return strings.Compare(string(x.loc), string(y.loc))
				})
				// Partners falling under different rules are separate findings.
				byRule := make(map[string][]located)
				var rules []string
				for _, p := range partners {
					rule := classify(a.field, p.field, v)
					if _, ok := byRule[rule]; !ok {
						rules = append(rules, rule)
					}
					byRule[rule] = append(byRule[rule], p)
				}
				for _, rule := range rules {
					f := base
					f.Rule = rule
					f.Change = pairChange(a.state, byRule[rule][0].state)
					f.Value = v
					f.Locations = []Location{a.loc}
					for _, p := range byRule[rule] {
						f.Locations = append(f.Locations, p.loc)
					}
					findings = append(findings, f)
				}
			}
		}
	}
	return findings
}

// partnerRank orders the partners of location a that were one SDK duplicate,
// the most telling one first: a tag of another level with the same key, a
// semantic synonym, an attribute, a top-level field, then a random attribute
// of the generated corpus.
func partnerRank(a, b field) int {
	switch rule := classify(a, b, ""); {
	case rule == RuleCrossLevel:
		return 0
	case rule == RuleConceptDup:
		return 1
	case b.attribute && !strings.HasPrefix(b.key, RandomKeyPrefix):
		return 2
	case !b.attribute:
		return 3
	}
	return 4
}

// touchedFirst reports whether location a should be listed before b in a
// finding: the location the agent touched the most comes first, then
// attributes before top-level fields, then by name.
func touchedFirst(a field, sa state, b field, sb state) bool {
	if sa != sb {
		return sa > sb
	}
	if a.attribute != b.attribute {
		return a.attribute
	}
	return a.loc < b.loc
}

// classify returns the rule a pair of locations sharing value falls under, or
// "" if it isn't a duplicate.
func classify(a, b field, value string) string {
	if a.tag && b.tag && a.level != b.level && a.key == b.key {
		return RuleCrossLevel
	}
	if a.attribute && b.attribute && a.key != b.key && sameConcept(a.key, b.key) {
		return RuleConceptDup
	}
	if len(value) >= MinLen {
		return RuleDup
	}
	return ""
}

// pairChange returns the change kind of a pair, sa being the state of the
// location the agent touched the most.
func pairChange(sa, sb state) string {
	switch {
	case sa == created:
		return ChangeCreated
	case sb == changed:
		return ChangeBothChanged
	}
	return ChangeOverwritten
}

var (
	conceptsOnce sync.Once
	conceptsOf   map[string][]semantics.Concept
)

// sameConcept reports whether two attribute keys are synonyms in the semantics registry.
func sameConcept(k1, k2 string) bool {
	conceptsOnce.Do(func() {
		conceptsOf = make(map[string][]semantics.Concept)
		for concept, infos := range semantics.DefaultRegistry().GetAllEquivalences() {
			for _, info := range infos {
				if !slices.Contains(conceptsOf[info.Name], concept) {
					conceptsOf[info.Name] = append(conceptsOf[info.Name], concept)
				}
			}
		}
	})
	for _, c := range conceptsOf[k1] {
		if slices.Contains(conceptsOf[k2], c) {
			return true
		}
	}
	return false
}

func spanBase(service, name, typ string, spanID uint64) Finding {
	return Finding{
		SpanType: typ,
		SpanID:   spanID,
		Service:  service,
		Name:     name,
		Context:  fmt.Sprintf("service=%s name=%s span_id=%d", service, name, spanID),
	}
}

// spanFields returns the string fields of a v0.4 span.
func spanFields(s *pb.Span) fieldList {
	var l fieldList
	if s == nil {
		return l
	}
	l.topLevel(spanLevel, LocationService, s.Service)
	l.topLevel(spanLevel, LocationName, s.Name)
	l.topLevel(spanLevel, LocationResource, s.Resource)
	l.topLevel(spanLevel, LocationType, s.Type)
	l.stringMap(spanLevel, "span.meta", s.Meta, true)
	for i, e := range s.SpanEvents {
		values := make(map[string]string, len(e.Attributes))
		for k, v := range e.Attributes {
			if v != nil && v.Type == pb.AttributeAnyValue_STRING_VALUE {
				values[k] = v.StringValue
			}
		}
		l.stringMap(spanLevel, fmt.Sprintf("span.events[%d].attributes", i), values, false)
	}
	for i, link := range s.SpanLinks {
		l.stringMap(spanLevel, fmt.Sprintf("span.links[%d].attributes", i), link.Attributes, false)
	}
	return l
}

// spanFieldsV1 returns the string fields of a v1 span.
func spanFieldsV1(s *idx.InternalSpan) fieldList {
	var l fieldList
	if s == nil {
		return l
	}
	l.topLevel(spanLevel, LocationService, s.Service())
	l.topLevel(spanLevel, LocationName, s.Name())
	l.topLevel(spanLevel, LocationResource, s.Resource())
	l.topLevel(spanLevel, LocationType, s.Type())
	l.topLevel(spanLevel, LocationEnv, s.Env())
	l.topLevel(spanLevel, LocationVersion, s.Version())
	l.topLevel(spanLevel, LocationComponent, s.Component())
	if s.Kind() != idx.SpanKind_SPAN_KIND_UNSPECIFIED {
		l.topLevel(spanLevel, LocationKind, s.SpanKind())
	}
	l.anyValueMap(spanLevel, "span.attributes", s.Strings, s.Attributes(), true)
	for i, e := range s.Events() {
		l.anyValueMap(spanLevel, fmt.Sprintf("span.events[%d].attributes", i), e.Strings, e.Attributes(), false)
	}
	for i, link := range s.Links() {
		l.anyValueMap(spanLevel, fmt.Sprintf("span.links[%d].attributes", i), link.Strings, link.Attributes(), false)
	}
	return l
}

func agentPayloadFields(p *pb.AgentPayload) fieldList {
	var l fieldList
	if p == nil {
		return l
	}
	l.topLevel(agentLevel, "agent.hostname", p.HostName)
	l.topLevel(agentLevel, "agent.env", p.Env)
	l.topLevel(agentLevel, "agent.agent_version", p.AgentVersion)
	l.stringMap(agentLevel, "agent.tags", p.Tags, true)
	return l
}

func tracerPayloadFields(p *pb.TracerPayload) fieldList {
	var l fieldList
	if p == nil {
		return l
	}
	l.topLevel(payloadLevel, "payload.hostname", p.Hostname)
	l.topLevel(payloadLevel, "payload.env", p.Env)
	l.topLevel(payloadLevel, "payload.app_version", p.AppVersion)
	l.stringMap(payloadLevel, "payload.tags", p.Tags, true)
	return l
}

func tracerPayloadFieldsV1(p *idx.InternalTracerPayload) fieldList {
	var l fieldList
	if p == nil {
		return l
	}
	l.topLevel(payloadLevel, "payload.hostname", p.Hostname())
	l.topLevel(payloadLevel, "payload.env", p.Env())
	l.topLevel(payloadLevel, "payload.app_version", p.AppVersion())
	l.anyValueMap(payloadLevel, "payload.attributes", p.Strings, p.Attributes, true)
	return l
}

func chunkFields(c *pb.TraceChunk) fieldList {
	var l fieldList
	if c == nil {
		return l
	}
	l.topLevel(chunkLevel, "chunk.origin", c.Origin)
	l.stringMap(chunkLevel, "chunk.tags", c.Tags, true)
	return l
}

func chunkFieldsV1(c *idx.InternalTraceChunk) fieldList {
	var l fieldList
	if c == nil {
		return l
	}
	l.topLevel(chunkLevel, "chunk.origin", c.Origin())
	l.anyValueMap(chunkLevel, "chunk.attributes", c.Strings, c.Attributes, true)
	return l
}

// involvesChunk keeps the pairs where at least one location is chunk-level, so
// that payload-level pairs are only reported once per payload.
func involvesChunk(a, b field) bool {
	return a.level == chunkLevel || b.level == chunkLevel
}

// spanCrossLevel keeps the cross-level pairs between a span tag and a tag of an
// upper level.
func spanCrossLevel(a, b field) bool {
	return a.tag && b.tag && a.key == b.key && (a.level == spanLevel) != (b.level == spanLevel)
}

// CheckSpan checks a v0.4 span against its stage input.
func CheckSpan(in, out *pb.Span) []Finding {
	return compareFields(spanFields(in), spanFields(out), nil, spanBase(out.Service, out.Name, out.Type, out.SpanID))
}

// CheckSpanV1 checks a v1 span against its stage input.
func CheckSpanV1(in, out *idx.InternalSpan) []Finding {
	return compareFields(spanFieldsV1(in), spanFieldsV1(out), nil, spanBase(out.Service(), out.Name(), out.Type(), out.SpanID()))
}

type spanKey struct {
	traceID uint64
	spanID  uint64
}

// CheckTracerPayload checks the agent payload sent for the v0.4 tracer payload
// in: its spans, its payload-level and chunk-level tags and fields, and the
// span tags repeated in an upper tag map.
func CheckTracerPayload(in *pb.TracerPayload, out *pb.AgentPayload) []Finding {
	inChunks := make(map[uint64]*pb.TraceChunk)
	inSpans := make(map[spanKey]*pb.Span)
	if in != nil {
		for _, c := range in.Chunks {
			for _, s := range c.Spans {
				inChunks[s.TraceID] = c
				inSpans[spanKey{s.TraceID, s.SpanID}] = s
			}
		}
	}
	inPayload := tracerPayloadFields(in)
	outAgent := agentPayloadFields(out)
	var findings []Finding
	for _, tp := range out.TracerPayloads {
		outPayload := concat(outAgent, tracerPayloadFields(tp))
		findings = append(findings, compareFields(inPayload, outPayload, nil, payloadBase(tp.Hostname))...)
		for _, c := range tp.Chunks {
			if len(c.Spans) == 0 {
				continue
			}
			traceID := c.Spans[0].TraceID
			inUpper := concat(inPayload, chunkFields(inChunks[traceID]))
			outUpper := concat(outPayload, chunkFields(c))
			findings = append(findings, compareFields(inUpper, outUpper, involvesChunk, chunkBase(strconv.FormatUint(traceID, 10)))...)
			for _, s := range c.Spans {
				inSpan := inSpans[spanKey{s.TraceID, s.SpanID}]
				findings = append(findings, CheckSpan(inSpan, s)...)
				base := spanBase(s.Service, s.Name, s.Type, s.SpanID)
				findings = append(findings, compareFields(concat(spanFields(inSpan), inUpper), concat(spanFields(s), outUpper), spanCrossLevel, base)...)
			}
		}
	}
	return findings
}

// CheckTracerPayloadV1 is the v1 equivalent of CheckTracerPayload, reading the
// idx tracer payloads of out.
func CheckTracerPayloadV1(in *idx.InternalTracerPayload, out *pb.AgentPayload) []Finding {
	type spanKeyV1 struct {
		traceID string
		spanID  uint64
	}
	inChunks := make(map[string]*idx.InternalTraceChunk)
	inSpans := make(map[spanKeyV1]*idx.InternalSpan)
	if in != nil {
		for _, c := range in.Chunks {
			inChunks[string(c.TraceID)] = c
			for _, s := range c.Spans {
				inSpans[spanKeyV1{string(c.TraceID), s.SpanID()}] = s
			}
		}
	}
	inPayload := tracerPayloadFieldsV1(in)
	outAgent := agentPayloadFields(out)
	var findings []Finding
	for _, ptp := range out.IdxTracerPayloads {
		tp := idx.FromProto(ptp)
		outPayload := concat(outAgent, tracerPayloadFieldsV1(tp))
		findings = append(findings, compareFields(inPayload, outPayload, nil, payloadBase(tp.Hostname()))...)
		for _, c := range tp.Chunks {
			traceID := string(c.TraceID)
			inUpper := concat(inPayload, chunkFieldsV1(inChunks[traceID]))
			outUpper := concat(outPayload, chunkFieldsV1(c))
			findings = append(findings, compareFields(inUpper, outUpper, involvesChunk, chunkBase(hex.EncodeToString(c.TraceID)))...)
			for _, s := range c.Spans {
				inSpan := inSpans[spanKeyV1{traceID, s.SpanID()}]
				findings = append(findings, CheckSpanV1(inSpan, s)...)
				base := spanBase(s.Service(), s.Name(), s.Type(), s.SpanID())
				findings = append(findings, compareFields(concat(spanFieldsV1(inSpan), inUpper), concat(spanFieldsV1(s), outUpper), spanCrossLevel, base)...)
			}
		}
	}
	return findings
}

// CheckOTLPConversion checks the conversion of OTLP resource spans into a
// tracer payload. The stage input of a converted span is the set of its OTLP
// span, resource and scope attributes, keyed like the span meta they are copied
// to, with the precedence of the conversion: span attributes, then resource
// attributes for the keys left empty, then scope attributes, written last,
// over both. Top-level fields
// don't exist yet. Attributes copied through unchanged are skipped, any
// attribute that the conversion creates or changes is compared with
// everything else.
func CheckOTLPConversion(in ptrace.ResourceSpans, out *pb.TracerPayload) []Finding {
	resourceAttrs := stringAttributes(in.Resource().Attributes())
	inMeta := make(map[uint64]map[string]string)
	for i := 0; i < in.ScopeSpans().Len(); i++ {
		ss := in.ScopeSpans().At(i)
		// The conversion writes every scope attribute to the meta with
		// AsString, numeric ones included, but stringAttributes drops numeric
		// values: a numeric scope attribute would be reported as created by
		// the agent. The corpus sets no scope attribute that is numeric.
		scopeAttrs := stringAttributes(ss.Scope().Attributes())
		for j := 0; j < ss.Spans().Len(); j++ {
			s := ss.Spans().At(j)
			spanID := s.SpanID()
			meta := stringAttributes(s.Attributes())
			for k, v := range resourceAttrs {
				if meta[k] == "" {
					meta[k] = v
				}
			}
			maps.Copy(meta, scopeAttrs)
			inMeta[binary.BigEndian.Uint64(spanID[:])] = meta
		}
	}
	outPayload := tracerPayloadFields(out)
	findings := compareFields(nil, outPayload, nil, payloadBase(out.Hostname))
	for _, c := range out.Chunks {
		if len(c.Spans) == 0 {
			continue
		}
		outUpper := concat(outPayload, chunkFields(c))
		findings = append(findings, compareFields(nil, outUpper, involvesChunk, chunkBase(strconv.FormatUint(c.Spans[0].TraceID, 10)))...)
		for _, s := range c.Spans {
			var inSpan fieldList
			if meta, ok := inMeta[s.SpanID]; ok {
				inSpan.stringMap(spanLevel, "span.meta", meta, true)
			}
			base := spanBase(s.Service, s.Name, s.Type, s.SpanID)
			findings = append(findings, compareFields(inSpan, spanFields(s), nil, base)...)
			findings = append(findings, compareFields(inSpan, concat(spanFields(s), outUpper), spanCrossLevel, base)...)
		}
	}
	return findings
}

// stringAttributes returns the OTLP attributes that the conversion stores as
// strings: numeric values become metrics and are skipped.
func stringAttributes(attrs pcommon.Map) map[string]string {
	m := make(map[string]string, attrs.Len())
	attrs.Range(func(k string, v pcommon.Value) bool {
		if v.Type() != pcommon.ValueTypeInt && v.Type() != pcommon.ValueTypeDouble {
			m[k] = v.AsString()
		}
		return true
	})
	return m
}

// payloadBase returns the base finding of a payload. Its context leaves out
// the env, which the agent may take from any of the traces of the payload, so
// that the printed examples are reproducible.
func payloadBase(hostname string) Finding {
	if hostname == "" {
		return Finding{Context: "payload"}
	}
	return Finding{Context: "payload hostname=" + hostname}
}

func chunkBase(traceID string) Finding {
	return Finding{Context: "chunk trace_id=" + traceID}
}

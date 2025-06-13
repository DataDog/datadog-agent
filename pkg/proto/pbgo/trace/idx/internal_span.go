// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package idx

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// StringTable is a table of strings that is used to store the de-duplicated strings in a trace
type StringTable struct {
	strings []string
	lookup  map[string]uint32
}

// NewStringTable creates a new string table, always starts with an empty string
func NewStringTable() *StringTable {
	return &StringTable{
		strings: []string{""},
		lookup:  map[string]uint32{"": 0},
	}
}

// addUnchecked adds a string to the string table without checking for duplicates
func (s *StringTable) addUnchecked(str string) uint32 {
	s.strings = append(s.strings, str)
	s.lookup[str] = uint32(len(s.strings) - 1)
	return uint32(len(s.strings) - 1)
}

// Add adds a string to the string table if it doesn't already exist and returns the index of the string
func (s *StringTable) Add(str string) uint32 {
	if idx, ok := s.lookup[str]; ok {
		return idx
	}
	return s.addUnchecked(str)
}

// Get returns the string at the given index - panics if out of bounds
func (s *StringTable) Get(idx uint32) string {
	return s.strings[idx]
}

// Len returns the number of strings in the string table
func (s *StringTable) Len() int {
	return len(s.strings)
}

// Lookup returns the index of the string in the string table, or 0 if the string is not found
func (s *StringTable) Lookup(str string) uint32 {
	if idx, ok := s.lookup[str]; ok {
		return idx
	}
	return 0
}

// InternalTracerPayload is a tracer payload structure that is optimized for trace-agent usage
// Namely it stores Attributes as a map for fast key lookups
type InternalTracerPayload struct {
	// array of strings referenced in this tracer payload, its chunks and spans
	Strings *StringTable
	// containerID specifies the ref in the strings table of the ID of the container where the tracer is running on.
	ContainerIDRef uint32
	// languageName specifies the ref in the strings table of the language of the tracer.
	LanguageNameRef uint32
	// languageVersion specifies the ref in the strings table of the language version of the tracer.
	LanguageVersionRef uint32
	// tracerVersion specifies the ref in the strings table of the version of the tracer.
	TracerVersionRef uint32
	// runtimeID specifies the ref in the strings table of the V4 UUID representation of a tracer session.
	RuntimeIDRef uint32
	// env specifies the ref in the strings table of the `env` tag that set with the tracer.
	EnvRef uint32
	// hostname specifies the ref in the strings table of the hostname of where the tracer is running.
	HostnameRef uint32
	// version specifies the ref in the strings table of the `version` tag that set with the tracer.
	AppVersionRef uint32
	// a collection of key to value pairs common in all `chunks`
	Attributes map[uint32]*AnyValue
	// chunks specifies list of containing trace chunks.
	Chunks []*InternalTraceChunk
}

func (tp *InternalTracerPayload) Hostname() string {
	return tp.Strings.Get(tp.HostnameRef)
}

func (tp *InternalTracerPayload) AppVersion() string {
	return tp.Strings.Get(tp.AppVersionRef)
}

func (tp *InternalTracerPayload) LanguageName() string {
	return tp.Strings.Get(tp.LanguageNameRef)
}

// SetLanguageName sets the language name in the string table
func (tp *InternalTracerPayload) SetLanguageName(name string) {
	tp.LanguageNameRef = tp.Strings.Add(name)
}

func (tp *InternalTracerPayload) LanguageVersion() string {
	return tp.Strings.Get(tp.LanguageVersionRef)
}

// SetLanguageVersion sets the language version in the string table
func (tp *InternalTracerPayload) SetLanguageVersion(version string) {
	tp.LanguageVersionRef = tp.Strings.Add(version)
}

func (tp *InternalTracerPayload) TracerVersion() string {
	return tp.Strings.Get(tp.TracerVersionRef)
}

// SetTracerVersion sets the tracer version in the string table
func (tp *InternalTracerPayload) SetTracerVersion(version string) {
	tp.TracerVersionRef = tp.Strings.Add(version)
}

func (tp *InternalTracerPayload) ContainerID() string {
	return tp.Strings.Get(tp.ContainerIDRef)
}

func (tp *InternalTracerPayload) Env() string {
	return tp.Strings.Get(tp.EnvRef)
}

func (tp *InternalTracerPayload) SetEnv(env string) {
	tp.EnvRef = tp.Strings.Add(env)
}

// RemoveChunk removes a chunk by its index.
func (tp *InternalTracerPayload) RemoveChunk(i int) {
	if i < 0 || i >= len(tp.Chunks) {
		return
	}
	tp.Chunks[i] = tp.Chunks[len(tp.Chunks)-1]
	tp.Chunks = tp.Chunks[:len(tp.Chunks)-1]
}

// ReplaceChunk replaces a chunk by its index.
func (tp *InternalTracerPayload) ReplaceChunk(i int, chunk *InternalTraceChunk) {
	tp.Chunks[i] = chunk
}

// AddString deduplicates the provided string and returns the index to reference it in the string table
func (tp *InternalTracerPayload) AddString(s string) uint32 {
	return tp.Strings.Add(s)
}

func (tp *InternalTracerPayload) SetStringAttribute(key, value string) {
	// TODO: How should we handle removing a tag? Can we just let the string dangle?
	tp.Attributes[tp.Strings.Add(key)] = &AnyValue{
		Value: &AnyValue_StringValueRef{
			StringValueRef: tp.Strings.Add(value),
		},
	}
}

// InternalTraceChunk is a trace chunk structure that is optimized for trace-agent usage
// Namely it stores Attributes as a map for fast key lookups and holds a pointer to the strings slice
// so a trace chunk holds all local context necessary to understand all fields
type InternalTraceChunk struct {
	Strings          *StringTable
	Priority         int32
	OriginRef        uint32
	Attributes       map[uint32]*AnyValue
	Spans            []*InternalSpan
	DroppedTrace     bool
	TraceID          []byte
	DecisionMakerRef uint32
}

// TODO: add a test to verify we have all fields
func (c *InternalTraceChunk) ShallowCopy() *InternalTraceChunk {
	return &InternalTraceChunk{
		Strings:          c.Strings,
		Priority:         c.Priority,
		OriginRef:        c.OriginRef,
		Attributes:       c.Attributes,
		Spans:            c.Spans,
		DroppedTrace:     c.DroppedTrace,
		TraceID:          c.TraceID,
		DecisionMakerRef: c.DecisionMakerRef,
	}
}

// LegacyTraceID returns the trace ID of the trace chunk as a uint64, the lowest order 8 bytes of the trace ID are the legacy trace ID
func (c *InternalTraceChunk) LegacyTraceID() uint64 {
	return binary.BigEndian.Uint64(c.TraceID[8:])
}

func (c *InternalTraceChunk) Origin() string {
	return c.Strings.Get(c.OriginRef)
}

func (c *InternalTraceChunk) SetOrigin(origin string) {
	c.OriginRef = c.Strings.Add(origin)
}

func (c *InternalTraceChunk) DecisionMaker() string {
	return c.Strings.Get(c.DecisionMakerRef)
}

func (c *InternalTraceChunk) SetDecisionMaker(decisionMaker string) {
	c.DecisionMakerRef = c.Strings.Add(decisionMaker)
}

// GetAttributeAsString returns the attribute as a string, or an empty string if the attribute is not found
func (c *InternalTraceChunk) GetAttributeAsString(key string) (string, bool) {
	if attr, ok := c.Attributes[c.Strings.Lookup(key)]; ok {
		return attr.AsString(c.Strings), true
	}
	return "", false
}

func (c *InternalTraceChunk) SetStringAttribute(key, value string) {
	// TODO: How should we handle removing a tag? Can we just let the string dangle?
	c.Attributes[c.Strings.Add(key)] = &AnyValue{
		Value: &AnyValue_StringValueRef{
			StringValueRef: c.Strings.Add(value),
		},
	}
}

// InternalSpan is a span structure that is optimized for trace-agent usage
// Namely it stores Attributes as a map for fast key lookups and holds a pointer to the strings slice
// so a span holds all local context necessary to understand all fields
type InternalSpan struct {
	// Strings is a pointer to the strings slice (Shared across a tracer payload)
	Strings *StringTable
	// service is the name of the service with which this span is associated.
	ServiceRef uint32
	// name is the operation name of this span.
	NameRef uint32
	// resource is the resource name of this span, also sometimes called the endpoint (for web spans).
	ResourceRef uint32
	// spanID is the ID of this span.
	SpanID uint64
	// parentID is the ID of this span's parent, or zero if this span has no parent.
	ParentID uint64
	// start is the number of nanoseconds between the Unix epoch and the beginning of this span.
	Start uint64
	// duration is the time length of this span in nanoseconds.
	Duration uint64
	// if there is an error associated with this span
	Error bool
	// meta is a mapping from tag name to tag value for string-valued tags.
	Attributes map[uint32]*AnyValue
	// type is the type of the service with which this span is associated.  Example values: web, db, lambda.
	TypeRef uint32
	// span_links represents a collection of links, where each link defines a causal relationship between two spans.
	SpanLinks []*InternalSpanLink
	// spanEvents represent an event at an instant in time related to this span, but not necessarily during the span.
	SpanEvents []*InternalSpanEvent
	// the optional string environment of this span
	EnvRef uint32
	// the optional string version of this span
	VersionRef uint32
	// the string component name of this span
	ComponentRef uint32
	// the SpanKind of this span as defined in the OTEL Specification
	Kind SpanKind
}

// TODO: add a test to verify we have all fields
func (s *InternalSpan) ShallowCopy() *InternalSpan {
	return &InternalSpan{
		Strings:      s.Strings,
		ServiceRef:   s.ServiceRef,
		NameRef:      s.NameRef,
		ResourceRef:  s.ResourceRef,
		SpanID:       s.SpanID,
		ParentID:     s.ParentID,
		Start:        s.Start,
		Duration:     s.Duration,
		Error:        s.Error,
		Attributes:   s.Attributes,
		TypeRef:      s.TypeRef,
		SpanLinks:    s.SpanLinks,
		SpanEvents:   s.SpanEvents,
		EnvRef:       s.EnvRef,
		VersionRef:   s.VersionRef,
		ComponentRef: s.ComponentRef,
		Kind:         s.Kind,
	}
}

// SpanKind returns the string representation of the span kind
func (s *InternalSpan) SpanKind() string {
	switch s.Kind {
	case SpanKind_SPAN_KIND_INTERNAL:
		return "internal"
	case SpanKind_SPAN_KIND_SERVER:
		return "server"
	case SpanKind_SPAN_KIND_CLIENT:
		return "client"
	case SpanKind_SPAN_KIND_PRODUCER:
		return "producer"
	case SpanKind_SPAN_KIND_CONSUMER:
		return "consumer"
	default:
		return "unknown"
	}
}

func (s *InternalSpan) Service() string {
	return s.Strings.Get(s.ServiceRef)
}

func (s *InternalSpan) SetService(svc string) {
	// TODO: remove old string?
	s.ServiceRef = s.Strings.Add(svc)
}

func (s *InternalSpan) Name() string {
	return s.Strings.Get(s.NameRef)
}

func (s *InternalSpan) SetName(name string) {
	// TODO: remove old string?
	s.NameRef = s.Strings.Add(name)
}

func (s *InternalSpan) Resource() string {
	return s.Strings.Get(s.ResourceRef)
}

func (s *InternalSpan) SetResource(resource string) {
	s.ResourceRef = s.Strings.Add(resource)
}

func (s *InternalSpan) Type() string {
	return s.Strings.Get(s.TypeRef)
}

func (s *InternalSpan) SetType(t string) {
	s.TypeRef = s.Strings.Add(t)
}

func (s *InternalSpan) Env() string {
	return s.Strings.Get(s.EnvRef)
}

func (s *InternalSpan) SetEnv(e string) {
	s.EnvRef = s.Strings.Add(e)
}

// GetAttributeAsString returns the attribute as a string, or an empty string if the attribute is not found
func (s *InternalSpan) GetAttributeAsString(key string) (string, bool) {
	if attr, ok := s.Attributes[s.Strings.Lookup(key)]; ok {
		return attr.AsString(s.Strings), true
	}
	return "", false
}

// GetAttributeAsFloat64 returns the attribute as a float64 and a boolean indicating if the attribute was found
func (s *InternalSpan) GetAttributeAsFloat64(key string) (float64, bool) {
	if attr, ok := s.Attributes[s.Strings.Lookup(key)]; ok {
		doubleVal, err := attr.AsDoubleValue(s.Strings)
		if err != nil {
			return 0, false
		}
		return doubleVal, true
	}
	return 0, false
}

func (s *InternalSpan) SetStringAttribute(key, value string) {
	// TODO: removing a string
	s.Attributes[s.Strings.Add(key)] = &AnyValue{
		Value: &AnyValue_StringValueRef{
			StringValueRef: s.Strings.Add(value),
		},
	}
}

func (s *InternalSpan) SetFloat64Attribute(key string, value float64) {
	// TODO: removing a string
	s.Attributes[s.Strings.Add(key)] = &AnyValue{
		Value: &AnyValue_DoubleValue{
			DoubleValue: value,
		},
	}
}

// SetAttributeFromString sets the attribute from a string, attempting to use the most backwards compatible type possible
// for the attribute value. Meaning we will prefer DoubleValue > IntValue > StringValue to match the previous metrics vs meta behavior
func (s *InternalSpan) SetAttributeFromString(key, value string) {
	// TODO: removing a string
	s.Attributes[s.Strings.Add(key)] = FromString(s.Strings, value)
}

func (s *InternalSpan) DeleteAttribute(key string) {
	// TODO: removing a string
	keyIdx := s.Strings.Lookup(key)
	if keyIdx != 0 {
		delete(s.Attributes, keyIdx)
	}
}

func (s *InternalSpan) DeleteAttributeIdx(keyIdx uint32) {
	delete(s.Attributes, keyIdx)
}

func (s *InternalSpan) MapStringAttributes(f func(k, v string) string) {
	for k, v := range s.Attributes {
		// TODO: we could cache the results of these transformations
		vString := v.AsString(s.Strings)
		newV := f(s.Strings.Get(k), vString)
		if newV != vString {
			s.Attributes[k] = &AnyValue{
				Value: &AnyValue_StringValueRef{
					StringValueRef: s.Strings.Add(newV),
				},
			}
		}
	}
}

// InternalSpanLink is a span link structure that is optimized for trace-agent usage
// Namely it stores Attributes as a map for fast key lookups
type InternalSpanLink struct {
	// Strings is a pointer to the strings slice (Shared across a tracer payload)
	Strings       *StringTable
	TraceID       []byte
	SpanID        uint64
	Attributes    map[uint32]*AnyValue
	TracestateRef uint32
	Flags         uint32
}

func (sl *InternalSpanLink) GetAttributeAsString(key string) (string, bool) {
	if attr, ok := sl.Attributes[sl.Strings.Lookup(key)]; ok {
		return attr.AsString(sl.Strings), true
	}
	return "", false
}

func (sl *InternalSpanLink) SetStringAttribute(key, value string) {
	// TODO: removing a string
	sl.Attributes[sl.Strings.Add(key)] = &AnyValue{
		Value: &AnyValue_StringValueRef{
			StringValueRef: sl.Strings.Add(value),
		},
	}
}

func (sl *InternalSpanLink) Tracestate() string {
	return sl.Strings.Get(sl.TracestateRef)
}

// InternalSpanEvent is a span event structure that is optimized for trace-agent usage
// Namely it stores Attributes as a map for fast key lookups
type InternalSpanEvent struct {
	// Strings is a pointer to the strings slice (Shared across a tracer payload)
	Strings    *StringTable
	Time       uint64
	NameRef    uint32
	Attributes map[uint32]*AnyValue
}

func (se *InternalSpanEvent) GetAttributeAsString(key string) (string, bool) {
	if attr, ok := se.Attributes[se.Strings.Lookup(key)]; ok {
		return attr.AsString(se.Strings), true
	}
	return "", false
}

// SetAttributeFromString sets the attribute on an InternalSpanEvent from a string, attempting to use the most backwards compatible type possible
// for the attribute value. Meaning we will prefer DoubleValue > IntValue > StringValue to match the previous metrics vs meta behavior
func (se *InternalSpanEvent) SetAttributeFromString(key, value string) {
	se.Attributes[se.Strings.Add(key)] = FromString(se.Strings, value)
}

// AsString returns the attribute in string format, this format is backwards compatible with non-v1 behavior
func (attr *AnyValue) AsString(strTable *StringTable) string {
	switch v := attr.Value.(type) {
	case *AnyValue_StringValueRef:
		return strTable.Get(v.StringValueRef)
	case *AnyValue_BoolValue:
		return strconv.FormatBool(v.BoolValue)
	case *AnyValue_DoubleValue:
		return strconv.FormatFloat(v.DoubleValue, 'f', -1, 64)
	case *AnyValue_IntValue:
		return strconv.FormatInt(v.IntValue, 10)
	case *AnyValue_BytesValue:
		return string(v.BytesValue)
	case *AnyValue_ArrayValue:
		values := v.ArrayValue.Values
		valuesStr := []string{}
		for _, value := range values {
			valuesStr = append(valuesStr, value.AsString(strTable))
		}
		return "[" + strings.Join(valuesStr, ",") + "]"
	case *AnyValue_KeyValueList:
		values := v.KeyValueList.KeyValues
		valuesStr := []string{}
		for _, kv := range values {
			valuesStr = append(valuesStr, strTable.Get(kv.Key)+"="+kv.Value.AsString(strTable))
		}
		return "{" + strings.Join(valuesStr, ",") + "}"
	default:
		return ""
	}
}

// AsDoubleValue returns the attribute in float64 format, returning an error if the attribute is not a float64 or can't be converted to a float64
func (attr *AnyValue) AsDoubleValue(strTable *StringTable) (float64, error) {
	switch v := attr.Value.(type) {
	case *AnyValue_StringValueRef:
		doubleVal, err := strconv.ParseFloat(strTable.Get(v.StringValueRef), 64)
		if err != nil {
			return 0, fmt.Errorf("string value not a float64: %w", err)
		}
		return doubleVal, nil
	case *AnyValue_BoolValue:
		if v.BoolValue {
			return 1, nil
		}
		return 0, nil
	case *AnyValue_DoubleValue:
		return v.DoubleValue, nil
	case *AnyValue_IntValue:
		return float64(v.IntValue), nil
	case *AnyValue_BytesValue:
		return 0, fmt.Errorf("bytes value not a float64")
	case *AnyValue_ArrayValue:
		return 0, fmt.Errorf("array value not a float64")
	case *AnyValue_KeyValueList:
		return 0, fmt.Errorf("key-value list value not a float64")
	default:
		return 0, fmt.Errorf("unknown value type not a float64")
	}
}

// FromString creates an AnyValue from a string, attempting to use the most backwards compatible type possible
// Meaning we will prefer DoubleValue > IntValue > StringValue to match the previous metrics vs meta behavior
func FromString(strTable *StringTable, s string) *AnyValue {
	if intVal, err := strconv.ParseInt(s, 10, 64); err == nil {
		return &AnyValue{
			Value: &AnyValue_IntValue{
				IntValue: intVal,
			},
		}
	}
	if floatVal, err := strconv.ParseFloat(s, 64); err == nil {
		return &AnyValue{
			Value: &AnyValue_DoubleValue{
				DoubleValue: floatVal,
			},
		}
	}
	return &AnyValue{
		Value: &AnyValue_StringValueRef{
			StringValueRef: strTable.Add(s),
		},
	}
}

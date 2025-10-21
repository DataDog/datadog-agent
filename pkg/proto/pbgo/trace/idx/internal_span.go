// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package idx

import (
	"encoding/binary"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/tinylib/msgp/msgp"
)

// StringTable is a table of strings that is used to store the de-duplicated strings in a trace
// Strings are not garbage collected automatically, so it is important to call RemoveUnusedStrings
// on the tracer payload to remove any strings that are no longer referenced.
type StringTable struct {
	strings []string
	lookup  map[string]uint32
}

// NewStringTable creates a new string table, always starts with an empty string at index 0
func NewStringTable() *StringTable {
	return &StringTable{
		strings: []string{""},
		lookup:  map[string]uint32{"": 0},
	}
}

// StringTableFromArray creates a new string table from an array of already de-duplicated strings
func StringTableFromArray(strings []string) *StringTable {
	st := &StringTable{
		strings: make([]string, len(strings)),
		lookup:  make(map[string]uint32, len(strings)),
	}
	for i, str := range strings {
		st.strings[i+1] = str
		st.lookup[str] = uint32(i + 1)
	}
	return st
}

// Msgsize returns the size of the message when serialized.
func (s *StringTable) Msgsize() int {
	size := 0
	size += msgp.ArrayHeaderSize
	size += msgp.StringPrefixSize * len(s.strings)
	for _, str := range s.strings {
		size += len(str)
	}
	return size
}

// Clone creates a deep copy of the string table.
func (s *StringTable) Clone() *StringTable {
	clone := &StringTable{
		strings: append([]string{}, s.strings...),
		lookup:  make(map[string]uint32, len(s.lookup)),
	}
	maps.Copy(clone.lookup, s.lookup)
	return clone
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
// Namely it stores Attributes as a map for fast key lookups.
type InternalTracerPayload struct {
	// Strings referenced in this tracer payload, its chunks and spans
	// This should generally not be accessed directly, but rather through the methods on the InternalTracerPayload
	// It is only exposed here for use in other packages that need to construct tracer payloads for testing.
	Strings *StringTable
	// containerID specifies the ref in the strings table of the ID of the container where the tracer is running on.
	containerIDRef uint32
	// languageName specifies the ref in the strings table of the language of the tracer.
	languageNameRef uint32
	// languageVersion specifies the ref in the strings table of the language version of the tracer.
	languageVersionRef uint32
	// tracerVersion specifies the ref in the strings table of the version of the tracer.
	tracerVersionRef uint32
	// runtimeID specifies the ref in the strings table of the V4 UUID representation of a tracer session.
	runtimeIDRef uint32
	// env specifies the ref in the strings table of the `env` tag that set with the tracer.
	envRef uint32
	// hostname specifies the ref in the strings table of the hostname of where the tracer is running.
	hostnameRef uint32
	// version specifies the ref in the strings table of the `version` tag that set with the tracer.
	appVersionRef uint32
	// a collection of key to value pairs common in all `chunks`
	Attributes map[uint32]*AnyValue
	// chunks specifies list of containing trace chunks.
	Chunks []*InternalTraceChunk
}

// Msgsize returns the size of the message when serialized.
func (tp *InternalTracerPayload) Msgsize() int {
	size := 0
	size += tp.Strings.Msgsize()
	size += msgp.Uint32Size + msgp.Uint32Size // containerIDRef
	size += msgp.Uint32Size + msgp.Uint32Size // languageNameRef
	size += msgp.Uint32Size + msgp.Uint32Size // languageVersionRef
	size += msgp.Uint32Size + msgp.Uint32Size // tracerVersionRef
	size += msgp.Uint32Size + msgp.Uint32Size // runtimeIDRef
	size += msgp.Uint32Size + msgp.Uint32Size // envRef
	size += msgp.Uint32Size + msgp.Uint32Size // hostnameRef
	size += msgp.Uint32Size + msgp.Uint32Size // appVersionRef
	size += msgp.MapHeaderSize                // Attributes
	for _, attr := range tp.Attributes {
		size += msgp.Uint32Size + attr.Msgsize() // Key size + Attribute size
	}
	size += msgp.ArrayHeaderSize // Chunks
	for _, chunk := range tp.Chunks {
		size += chunk.Msgsize()
	}
	return size
}

// RemoveUnusedStrings removes any strings from the string table that are not referenced in the tracer payload
// This should be called before marshalling or otherwise exposing the tracer payload to remove any sensitive
// strings that are no longer referenced
func (tp *InternalTracerPayload) RemoveUnusedStrings() {
	usedStrings := make([]bool, tp.Strings.Len())
	usedStrings[tp.containerIDRef] = true
	usedStrings[tp.languageNameRef] = true
	usedStrings[tp.languageVersionRef] = true
	usedStrings[tp.tracerVersionRef] = true
	usedStrings[tp.runtimeIDRef] = true
	usedStrings[tp.envRef] = true
	usedStrings[tp.hostnameRef] = true
	usedStrings[tp.appVersionRef] = true
	markAttributeMapStringsUsed(usedStrings, tp.Strings, tp.Attributes)
	for _, chunk := range tp.Chunks {
		chunk.markUsedStrings(usedStrings)
	}
	for i, used := range usedStrings {
		if !used {
			// Remove the reverse lookup and set the string to the empty string
			// We don't adjust the table itself to avoid changing the indices of the other strings
			delete(tp.Strings.lookup, tp.Strings.strings[i])
			tp.Strings.strings[i] = ""
		}
	}
}

// ToProto converts an InternalTracerPayload to a proto TracerPayload
// This returns the structure _AS IS_, so even strings that are no longer referenced
// may be included in the resulting proto. To ensure that only used strings are included,
// call RemoveUnusedStrings first.
func (tp *InternalTracerPayload) ToProto() *TracerPayload {
	chunks := make([]*TraceChunk, len(tp.Chunks))
	for i, chunk := range tp.Chunks {
		chunks[i] = chunk.ToProto()
	}

	return &TracerPayload{
		Strings:            tp.Strings.strings,
		ContainerIDRef:     tp.containerIDRef,
		LanguageNameRef:    tp.languageNameRef,
		LanguageVersionRef: tp.languageVersionRef,
		TracerVersionRef:   tp.tracerVersionRef,
		RuntimeIDRef:       tp.runtimeIDRef,
		EnvRef:             tp.envRef,
		HostnameRef:        tp.hostnameRef,
		AppVersionRef:      tp.appVersionRef,
		Attributes:         tp.Attributes,
		Chunks:             chunks,
	}
}

// SetAttributes sets the attributes for the tracer payload.
func (tp *InternalTracerPayload) SetAttributes(attributes map[uint32]*AnyValue) {
	tp.Attributes = attributes
}

// Hostname returns the hostname from the tracer payload.
func (tp *InternalTracerPayload) Hostname() string {
	return tp.Strings.Get(tp.hostnameRef)
}

// SetHostname sets the hostname for the tracer payload.
func (tp *InternalTracerPayload) SetHostname(hostname string) {
	tp.hostnameRef = tp.Strings.Add(hostname)
}

// AppVersion returns the application version from the tracer payload.
func (tp *InternalTracerPayload) AppVersion() string {
	return tp.Strings.Get(tp.appVersionRef)
}

// SetAppVersion sets the application version for the tracer payload.
func (tp *InternalTracerPayload) SetAppVersion(version string) {
	tp.appVersionRef = tp.Strings.Add(version)
}

// LanguageName returns the language name from the tracer payload.
func (tp *InternalTracerPayload) LanguageName() string {
	return tp.Strings.Get(tp.languageNameRef)
}

// SetLanguageName sets the language name in the string table
func (tp *InternalTracerPayload) SetLanguageName(name string) {
	tp.languageNameRef = tp.Strings.Add(name)
}

// LanguageVersion returns the language version from the tracer payload.
func (tp *InternalTracerPayload) LanguageVersion() string {
	return tp.Strings.Get(tp.languageVersionRef)
}

// SetLanguageVersion sets the language version in the string table
func (tp *InternalTracerPayload) SetLanguageVersion(version string) {
	tp.languageVersionRef = tp.Strings.Add(version)
}

// TracerVersion returns the tracer version from the tracer payload.
func (tp *InternalTracerPayload) TracerVersion() string {
	return tp.Strings.Get(tp.tracerVersionRef)
}

// SetTracerVersion sets the tracer version in the string table
func (tp *InternalTracerPayload) SetTracerVersion(version string) {
	tp.tracerVersionRef = tp.Strings.Add(version)
}

// ContainerID returns the container ID from the tracer payload.
func (tp *InternalTracerPayload) ContainerID() string {
	return tp.Strings.Get(tp.containerIDRef)
}

// SetContainerID sets the container ID for the tracer payload.
func (tp *InternalTracerPayload) SetContainerID(containerID string) {
	tp.containerIDRef = tp.Strings.Add(containerID)
}

// Env returns the environment from the tracer payload.
func (tp *InternalTracerPayload) Env() string {
	return tp.Strings.Get(tp.envRef)
}

// SetEnv sets the environment for the tracer payload.
func (tp *InternalTracerPayload) SetEnv(env string) {
	tp.envRef = tp.Strings.Add(env)
}

// RuntimeID returns the runtime ID from the tracer payload.
func (tp *InternalTracerPayload) RuntimeID() string {
	return tp.Strings.Get(tp.runtimeIDRef)
}

// SetRuntimeID sets the runtime ID for the tracer payload.
func (tp *InternalTracerPayload) SetRuntimeID(runtimeID string) {
	tp.runtimeIDRef = tp.Strings.Add(runtimeID)
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

// SetStringAttribute sets a string attribute for the tracer payload.
func (tp *InternalTracerPayload) SetStringAttribute(key, value string) {
	setStringAttribute(key, value, tp.Strings, tp.Attributes)
}

// GetAttributeAsString gets a string attribute from the tracer payload.
func (tp *InternalTracerPayload) GetAttributeAsString(key string) (string, bool) {
	return getAttributeAsString(key, tp.Strings, tp.Attributes)
}

// Cut cuts off a new tracer payload from the `p` with [0, i-1] chunks
// and keeps [i, n-1] chunks in the original payload `p`.
// The new payload will have a new string table, so it can be used concurrently with the original payload.
func (tp *InternalTracerPayload) Cut(i int) *InternalTracerPayload {
	if i < 0 {
		i = 0
	}
	if i > len(tp.Chunks) {
		i = len(tp.Chunks)
	}
	newStrings := tp.Strings.Clone()
	newPayload := InternalTracerPayload{
		Strings:            newStrings, // Clone string table to protect against concurrent access
		containerIDRef:     tp.containerIDRef,
		languageNameRef:    tp.languageNameRef,
		languageVersionRef: tp.languageVersionRef,
		tracerVersionRef:   tp.tracerVersionRef,
		runtimeIDRef:       tp.runtimeIDRef,
		envRef:             tp.envRef,
		hostnameRef:        tp.hostnameRef,
		appVersionRef:      tp.appVersionRef,
		Attributes:         tp.Attributes,
	}
	newPayload.Chunks = tp.Chunks[:i]
	for _, chunk := range newPayload.Chunks {
		chunk.Strings = newStrings
		for _, span := range chunk.Spans {
			span.Strings = newStrings
		}
	}
	tp.Chunks = tp.Chunks[i:]
	return &newPayload
}

// InternalTraceChunk is a trace chunk structure that is optimized for trace-agent usage
// Namely it stores Attributes as a map for fast key lookups and holds a pointer to the strings slice
// so a trace chunk holds all local context necessary to understand all fields
type InternalTraceChunk struct {
	// Strings referenced in this trace chunk. Note this is shared with the tracer payload
	// This should generally not be accessed directly, but rather through the methods on the InternalTracerPayload
	// It is only exposed here for use in other packages that need to construct tracer payloads for testing.
	Strings           *StringTable
	Priority          int32
	originRef         uint32
	Attributes        map[uint32]*AnyValue
	Spans             []*InternalSpan
	DroppedTrace      bool
	TraceID           []byte
	samplingMechanism uint32
}

// NewInternalTraceChunk creates a new internal trace chunk.
func NewInternalTraceChunk(strings *StringTable, priority int32, origin string, attributes map[uint32]*AnyValue, spans []*InternalSpan, droppedTrace bool, traceID []byte, samplingMechanism uint32) *InternalTraceChunk {
	return &InternalTraceChunk{
		Strings:           strings,
		Priority:          priority,
		originRef:         strings.Add(origin),
		Attributes:        attributes,
		Spans:             spans,
		DroppedTrace:      droppedTrace,
		TraceID:           traceID,
		samplingMechanism: samplingMechanism,
	}
}

// ShallowCopy creates a shallow copy of the internal trace chunk.
// TODO: add a test to verify we have all fields
func (c *InternalTraceChunk) ShallowCopy() *InternalTraceChunk {
	return &InternalTraceChunk{
		Strings:           c.Strings,
		Priority:          c.Priority,
		originRef:         c.originRef,
		Attributes:        c.Attributes,
		Spans:             c.Spans,
		DroppedTrace:      c.DroppedTrace,
		TraceID:           c.TraceID,
		samplingMechanism: c.samplingMechanism,
	}
}

// Msgsize returns the size of the message when serialized.
func (c *InternalTraceChunk) Msgsize() int {
	size := 0
	size += c.Strings.Msgsize()
	size += msgp.Int32Size     // Priority
	size += msgp.Uint32Size    // OriginRef
	size += msgp.MapHeaderSize // Attributes
	for _, attr := range c.Attributes {
		size += msgp.Uint32Size + attr.Msgsize() // Key size + Attribute size
	}
	size += msgp.ArrayHeaderSize // Spans
	for _, span := range c.Spans {
		size += span.Msgsize()
	}
	size += msgp.BoolSize             // DroppedTrace
	size += msgp.BytesPrefixSize + 16 // TraceID (128 bits)
	size += msgp.Uint32Size           // DecisionMakerRef
	return size
}

// LegacyTraceID returns the trace ID of the trace chunk as a uint64, the lowest order 8 bytes of the trace ID are the legacy trace ID
func (c *InternalTraceChunk) LegacyTraceID() uint64 {
	return binary.BigEndian.Uint64(c.TraceID[8:])
}

// Origin returns the origin from the trace chunk.
func (c *InternalTraceChunk) Origin() string {
	return c.Strings.Get(c.originRef)
}

// SetOrigin sets the origin for the trace chunk.
func (c *InternalTraceChunk) SetOrigin(origin string) {
	c.originRef = c.Strings.Add(origin)
}

// SamplingMechanism returns the sampling mechanism from the trace chunk.
func (c *InternalTraceChunk) SamplingMechanism() uint32 {
	return c.samplingMechanism
}

// SetSamplingMechanism sets the sampling mechanism for the trace chunk.
func (c *InternalTraceChunk) SetSamplingMechanism(samplingMechanism uint32) {
	c.samplingMechanism = samplingMechanism
}

// GetAttributeAsString returns the attribute as a string, or an empty string if the attribute is not found
func (c *InternalTraceChunk) GetAttributeAsString(key string) (string, bool) {
	return getAttributeAsString(key, c.Strings, c.Attributes)
}

// SetStringAttribute sets a string attribute for the trace chunk.
func (c *InternalTraceChunk) SetStringAttribute(key, value string) {
	setStringAttribute(key, value, c.Strings, c.Attributes)
}

func (c *InternalTraceChunk) markUsedStrings(usedStrings []bool) {
	usedStrings[c.originRef] = true
	markAttributeMapStringsUsed(usedStrings, c.Strings, c.Attributes)
	for _, span := range c.Spans {
		span.markUsedStrings(usedStrings)
	}
}

// ToProto converts an InternalTraceChunk to a proto TraceChunk
func (c *InternalTraceChunk) ToProto() *TraceChunk {
	spans := make([]*Span, len(c.Spans))
	for i, span := range c.Spans {
		spans[i] = span.ToProto()
	}
	return &TraceChunk{
		Priority:          c.Priority,
		OriginRef:         c.originRef,
		Attributes:        c.Attributes,
		Spans:             spans,
		DroppedTrace:      c.DroppedTrace,
		TraceID:           c.TraceID,
		SamplingMechanism: c.samplingMechanism,
	}
}

// InternalSpan is a span structure that is optimized for trace-agent usage
// Namely it stores Attributes as a map for fast key lookups and holds a pointer to the strings slice
// so a span holds all local context necessary to understand all fields
type InternalSpan struct {
	// Strings referenced in this span. Note this is shared with the tracer payload
	// This should generally not be accessed directly, but rather through the methods on the InternalTracerPayload
	// It is only exposed here for use in other packages that need to construct tracer payloads for testing.
	Strings *StringTable
	span    *Span // We reference the proto span directly to avoid the allocation overhead when converting this to a proto span
}

// NewInternalSpan creates a new internal span.
func NewInternalSpan(strings *StringTable, span *Span) *InternalSpan {
	return &InternalSpan{
		Strings: strings,
		span:    span,
	}
}

// ShallowCopy creates a shallow copy of the internal span.
func (s *InternalSpan) ShallowCopy() *InternalSpan {
	return &InternalSpan{
		Strings: s.Strings,
		span:    s.span.ShallowCopy(),
	}
}

func (s *InternalSpan) markUsedStrings(usedStrings []bool) {
	usedStrings[s.span.ServiceRef] = true
	usedStrings[s.span.NameRef] = true
	usedStrings[s.span.ResourceRef] = true
	usedStrings[s.span.TypeRef] = true
	usedStrings[s.span.EnvRef] = true
	usedStrings[s.span.VersionRef] = true
	usedStrings[s.span.ComponentRef] = true
	markAttributeMapStringsUsed(usedStrings, s.Strings, s.span.Attributes)
	for _, link := range s.span.Links {
		markSpanLinkUsedStrings(usedStrings, s.Strings, link)
	}
	for _, event := range s.span.Events {
		markSpanEventUsedStrings(usedStrings, s.Strings, event)
	}
}

// ToProto converts the internal span to a protobuf span.
func (s *InternalSpan) ToProto() *Span {
	return s.span
}

func markSpanLinkUsedStrings(usedStrings []bool, strTable *StringTable, link *SpanLink) {
	usedStrings[link.TracestateRef] = true
	markAttributeMapStringsUsed(usedStrings, strTable, link.Attributes)
}

func markSpanEventUsedStrings(usedStrings []bool, strTable *StringTable, event *SpanEvent) {
	usedStrings[event.NameRef] = true
	markAttributeMapStringsUsed(usedStrings, strTable, event.Attributes)
}

// ShallowCopy returns a shallow copy of the span
func (s *Span) ShallowCopy() *Span {
	return &Span{
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
		Links:        s.Links,
		Events:       s.Events,
		EnvRef:       s.EnvRef,
		VersionRef:   s.VersionRef,
		ComponentRef: s.ComponentRef,
		Kind:         s.Kind,
	}
}

// DebugString returns a human readable string representation of the span
func (s *InternalSpan) DebugString() string {
	str := "Span {"
	str += fmt.Sprintf("Service: (%s, at %d), ", s.Service(), s.span.ServiceRef)
	str += fmt.Sprintf("Name: (%s, at %d), ", s.Name(), s.span.NameRef)
	str += fmt.Sprintf("Resource: (%s, at %d), ", s.Resource(), s.span.ResourceRef)
	str += fmt.Sprintf("SpanID: %d, ", s.span.SpanID)
	str += fmt.Sprintf("ParentID: %d, ", s.span.ParentID)
	str += fmt.Sprintf("Start: %d, ", s.span.Start)
	str += fmt.Sprintf("Duration: %d, ", s.span.Duration)
	str += fmt.Sprintf("Error: %t, ", s.span.Error)
	str += fmt.Sprintf("Attributes: %v, ", s.span.Attributes)
	str += fmt.Sprintf("Type: (%s, at %d), ", s.Type(), s.span.TypeRef)
	str += fmt.Sprintf("Links: %v, ", s.Links())
	str += fmt.Sprintf("Events: %v, ", s.Events())
	str += fmt.Sprintf("Env: (%s, at %d), ", s.Env(), s.span.EnvRef)
	str += fmt.Sprintf("Version: (%s, at %d), ", s.Version(), s.span.VersionRef)
	str += fmt.Sprintf("Component: (%s, at %d), ", s.Component(), s.span.ComponentRef)
	str += fmt.Sprintf("Kind: %s, ", s.SpanKind())
	str += "}"
	return str
}

// Events returns the spans events in the InternalSpanEvent format
func (s *InternalSpan) Events() []*InternalSpanEvent {
	events := make([]*InternalSpanEvent, len(s.span.Events))
	for i, event := range s.span.Events {
		events[i] = &InternalSpanEvent{
			Strings: s.Strings,
			event:   event,
		}
	}
	return events
}

// Links returns the spans links in the InternalSpanLink format
func (s *InternalSpan) Links() []*InternalSpanLink {
	links := make([]*InternalSpanLink, len(s.span.Links))
	for i, link := range s.span.Links {
		links[i] = &InternalSpanLink{
			Strings: s.Strings,
			link:    link,
		}
	}
	return links
}

// LenLinks returns the number of links in the span.
func (s *InternalSpan) LenLinks() int {
	return len(s.span.Links)
}

// Msgsize returns the size of the message when serialized.
func (s *InternalSpan) Msgsize() int {
	size := 0
	size += msgp.MapHeaderSize                   // Header (All fields are key-value pairs, uint32 for keys)
	size += msgp.Uint32Size + msgp.Uint32Size    // ServiceRef
	size += msgp.Uint32Size + msgp.Uint32Size    // NameRef
	size += msgp.Uint32Size + msgp.Uint32Size    // ResourceRef
	size += msgp.Uint32Size + msgp.Uint64Size    // SpanID
	size += msgp.Uint32Size + msgp.Uint64Size    // ParentID
	size += msgp.Uint32Size + msgp.Uint64Size    // Start
	size += msgp.Uint32Size + msgp.Uint64Size    // Duration
	size += msgp.Uint32Size + msgp.BoolSize      // Error
	size += msgp.Uint32Size + msgp.MapHeaderSize // Attributes
	for _, attr := range s.span.Attributes {
		size += msgp.Uint32Size + attr.Msgsize() // Key size + Attribute size
	}
	size += msgp.Uint32Size + msgp.Uint32Size      // TypeRef
	size += msgp.Uint32Size + msgp.ArrayHeaderSize // SpanLinks
	for _, link := range s.span.Links {
		size += link.Msgsize()
	}
	size += msgp.Uint32Size + msgp.ArrayHeaderSize // SpanEvents
	for _, event := range s.span.Events {
		size += event.Msgsize()
	}
	size += msgp.Uint32Size + msgp.Uint32Size // EnvRef
	size += msgp.Uint32Size + msgp.Uint32Size // VersionRef
	size += msgp.Uint32Size + msgp.Uint32Size // ComponentRef
	size += msgp.Uint32Size + msgp.Uint32Size // Kind
	return size
}

// SpanKind returns the string representation of the span kind
func (s *InternalSpan) SpanKind() string {
	switch s.span.Kind {
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

// Service returns the service name from the span.
func (s *InternalSpan) Service() string {
	return s.Strings.Get(s.span.ServiceRef)
}

// SetService sets the service name for the span.
func (s *InternalSpan) SetService(svc string) {
	s.span.ServiceRef = s.Strings.Add(svc)
}

// Name returns the span name.
func (s *InternalSpan) Name() string {
	return s.Strings.Get(s.span.NameRef)
}

// SetName sets the span name.
func (s *InternalSpan) SetName(name string) {
	s.span.NameRef = s.Strings.Add(name)
}

// Resource returns the resource from the span.
func (s *InternalSpan) Resource() string {
	return s.Strings.Get(s.span.ResourceRef)
}

// SetResource sets the resource for the span.
func (s *InternalSpan) SetResource(resource string) {
	s.span.ResourceRef = s.Strings.Add(resource)
}

// Type returns the span type.
func (s *InternalSpan) Type() string {
	return s.Strings.Get(s.span.TypeRef)
}

// SetType sets the span type.
func (s *InternalSpan) SetType(t string) {
	s.span.TypeRef = s.Strings.Add(t)
}

// Env returns the environment from the span.
func (s *InternalSpan) Env() string {
	return s.Strings.Get(s.span.EnvRef)
}

// SetEnv sets the environment for the span.
func (s *InternalSpan) SetEnv(e string) {
	s.span.EnvRef = s.Strings.Add(e)
}

// ParentID returns the parent span ID.
func (s *InternalSpan) ParentID() uint64 {
	return s.span.ParentID
}

// SetParentID sets the parent span ID.
func (s *InternalSpan) SetParentID(parentID uint64) {
	s.span.ParentID = parentID
}

// SpanID returns the span ID.
func (s *InternalSpan) SpanID() uint64 {
	return s.span.SpanID
}

// SetSpanID sets the span ID.
func (s *InternalSpan) SetSpanID(spanID uint64) {
	s.span.SpanID = spanID
}

// Start returns the start time of the span.
func (s *InternalSpan) Start() uint64 {
	return s.span.Start
}

// SetStart sets the start time of the span.
func (s *InternalSpan) SetStart(start uint64) {
	s.span.Start = start
}

func (s *InternalSpan) Error() bool {
	return s.span.Error
}

// SetError sets the error flag for the span.
func (s *InternalSpan) SetError(error bool) {
	s.span.Error = error
}

// Attributes returns the attributes of the span.
func (s *InternalSpan) Attributes() map[uint32]*AnyValue {
	return s.span.Attributes
}

// Duration returns the duration of the span.
func (s *InternalSpan) Duration() uint64 {
	return s.span.Duration
}

// SetDuration sets the duration of the span.
func (s *InternalSpan) SetDuration(duration uint64) {
	s.span.Duration = duration
}

// Kind returns the span kind.
func (s *InternalSpan) Kind() SpanKind {
	return s.span.Kind
}

// SetSpanKind sets the span kind.
func (s *InternalSpan) SetSpanKind(kind SpanKind) {
	s.span.Kind = kind
}

// Component returns the component from the span.
func (s *InternalSpan) Component() string {
	return s.Strings.Get(s.span.ComponentRef)
}

// SetComponent sets the component for the span.
func (s *InternalSpan) SetComponent(component string) {
	s.span.ComponentRef = s.Strings.Add(component)
}

// Version returns the version from the span.
func (s *InternalSpan) Version() string {
	return s.Strings.Get(s.span.VersionRef)
}

// SetVersion sets the version for the span.
func (s *InternalSpan) SetVersion(version string) {
	s.span.VersionRef = s.Strings.Add(version)
}

// GetAttributeAsString returns the attribute as a string, or an empty string if the attribute is not found
func (s *InternalSpan) GetAttributeAsString(key string) (string, bool) {
	// To maintain backwards compatibility, we need to handle some special cases where these keys used to be set as tags
	if key == "env" {
		return s.Env(), true
	}
	if key == "version" {
		return s.Version(), true
	}
	if key == "component" {
		return s.Component(), true
	}
	if key == "span.kind" {
		return s.SpanKind(), true
	}
	return getAttributeAsString(key, s.Strings, s.span.Attributes)
}

// GetAttributeAsFloat64 returns the attribute as a float64 and a boolean indicating if the attribute was found AND it was able to be converted to a float64
func (s *InternalSpan) GetAttributeAsFloat64(key string) (float64, bool) {
	keyIdx := s.Strings.Lookup(key)
	if keyIdx == 0 {
		return 0, false
	}
	if attr, ok := s.span.Attributes[keyIdx]; ok {
		doubleVal, err := attr.AsDoubleValue(s.Strings)
		if err != nil {
			return 0, false
		}
		return doubleVal, true
	}
	return 0, false
}

// SetStringAttribute sets the attribute with key and value
// For backwards compatibility, env, version, component, and span.kind will be set as fields instead of attributes
func (s *InternalSpan) SetStringAttribute(key, value string) {
	if s.span.Attributes == nil {
		s.span.Attributes = make(map[uint32]*AnyValue)
	}
	if s.setCompatibleTags(key, value) {
		return
	}
	setStringAttribute(key, value, s.Strings, s.span.Attributes)
}

// SetFloat64Attribute sets a float64 attribute for the span.
func (s *InternalSpan) SetFloat64Attribute(key string, value float64) {
	if s.span.Attributes == nil {
		s.span.Attributes = make(map[uint32]*AnyValue)
	}
	setFloat64Attribute(key, value, s.Strings, s.span.Attributes)
}

// setCompatibleTags checks if the key is a special case that was previously a tag
// if it is, then we set the new field and return true, otherwise we return false
func (s *InternalSpan) setCompatibleTags(key, value string) bool {
	if s.span.Attributes == nil {
		s.span.Attributes = make(map[uint32]*AnyValue)
	}
	if key == "env" {
		s.SetEnv(value)
		return true
	}
	if key == "version" {
		s.SetVersion(value)
		return true
	}
	if key == "component" {
		s.SetComponent(value)
		return true
	}
	if key == "span.kind" {
		newKind := SpanKind_SPAN_KIND_UNSPECIFIED
		switch value {
		case "internal":
			newKind = SpanKind_SPAN_KIND_INTERNAL
		case "server":
			newKind = SpanKind_SPAN_KIND_SERVER
		case "client":
			newKind = SpanKind_SPAN_KIND_CLIENT
		case "producer":
			newKind = SpanKind_SPAN_KIND_PRODUCER
		case "consumer":
			newKind = SpanKind_SPAN_KIND_CONSUMER
		}
		if newKind == SpanKind_SPAN_KIND_UNSPECIFIED {
			// On an unknown span kind, we just won't set it
			return true
		}
		s.SetSpanKind(newKind)
		return true
	}
	return false
}

// SetAttributeFromString sets the attribute from a string, attempting to use the most backwards compatible type possible
// for the attribute value. Meaning we will prefer DoubleValue > IntValue > StringValue to match the previous metrics vs meta behavior
// For backwards compatibility, env, version, component, and span.kind will be set as fields instead of attributes
func (s *InternalSpan) SetAttributeFromString(key, value string) {
	if s.span.Attributes == nil {
		s.span.Attributes = make(map[uint32]*AnyValue)
	}
	if s.setCompatibleTags(key, value) {
		return
	}
	setAttribute(key, FromString(s.Strings, value), s.Strings, s.span.Attributes)
}

// DeleteAttribute deletes an attribute from the span.
func (s *InternalSpan) DeleteAttribute(key string) {
	deleteAttribute(key, s.Strings, s.span.Attributes)
}

// MapAttributesAsStrings maps over all string attributes and applies the given function to each attribute
// Note that this will only act on true attributes, fields like env, version, component, etc are not considered
// The provided function will receive all attributes as strings, and should return the new value for the attribute
func (s *InternalSpan) MapAttributesAsStrings(f func(k, v string) string) {
	for k, v := range s.span.Attributes {
		// TODO: we could cache the results of these transformations
		// TODO: This is only used for CC obfuscation today, we could optimize this to reduce the overhead here
		vString := v.AsString(s.Strings)
		newV := f(s.Strings.Get(k), vString)
		if newV != vString {
			s.span.Attributes[k] = &AnyValue{
				Value: &AnyValue_StringValueRef{
					StringValueRef: s.Strings.Add(newV),
				},
			}
		}
	}
}

// MapStringAttributesFunc is a function that maps over all string attributes and applies the given function to each attribute
type MapStringAttributesFunc func(k, v string) (newK string, newV string, shouldReplace bool)

// MapStringAttributes maps over all string attributes and applies the given function to each attribute
// Note that this will only act on true attributes, fields like env, version, component, etc are not considered
// The provided function will only act on attributes that are string types
func (s *InternalSpan) MapStringAttributes(f MapStringAttributesFunc) {
	for k, v := range s.span.Attributes {
		if vStrAttr, ok := v.Value.(*AnyValue_StringValueRef); ok {
			oldK := s.Strings.Get(k)
			oldV := s.Strings.Get(vStrAttr.StringValueRef)
			newK, newV, shouldReplace := f(oldK, oldV)
			if shouldReplace {
				newVAttr := v
				if newV != oldV {
					newVAttr.Value.(*AnyValue_StringValueRef).StringValueRef = s.Strings.Add(newV)
				}
				kIdx := k
				if newK != oldK {
					// Key has changed we must introduce a new attribute
					delete(s.span.Attributes, k)
					kIdx = s.Strings.Add(newK)
				}
				s.span.Attributes[kIdx] = newVAttr
			}
		}
	}
}

// InternalSpanLink is a span link structure that is optimized for trace-agent usage
// Namely it stores Attributes as a map for fast key lookups
type InternalSpanLink struct {
	// Strings is a pointer to the strings slice (Shared across a tracer payload)
	Strings *StringTable
	link    *SpanLink
}

// Msgsize returns the size of the message when serialized.
func (sl *SpanLink) Msgsize() int {
	size := 0
	size += msgp.MapHeaderSize                          // Map
	size += msgp.Uint32Size + msgp.BytesPrefixSize + 16 // TraceID (128 bits)
	size += msgp.Uint32Size + msgp.Uint64Size           // SpanID
	size += msgp.Uint32Size + msgp.MapHeaderSize        // Attributes
	for _, attr := range sl.Attributes {
		size += msgp.Uint32Size + attr.Msgsize() // Key size + Attribute size
	}
	size += msgp.Uint32Size + msgp.Uint32Size // TracestateRef
	size += msgp.Uint32Size + msgp.Uint32Size // Flags
	return size
}

// TraceID returns the trace ID from the span link.
func (sl *InternalSpanLink) TraceID() []byte {
	return sl.link.TraceID
}

// SpanID returns the span ID from the span link.
func (sl *InternalSpanLink) SpanID() uint64 {
	return sl.link.SpanID
}

// Flags returns the flags from the span link.
func (sl *InternalSpanLink) Flags() uint32 {
	return sl.link.Flags
}

// GetAttributeAsString gets a string attribute from the span link.
func (sl *InternalSpanLink) GetAttributeAsString(key string) (string, bool) {
	return getAttributeAsString(key, sl.Strings, sl.link.Attributes)
}

// SetStringAttribute sets a string attribute for the span link.
func (sl *InternalSpanLink) SetStringAttribute(key, value string) {
	setStringAttribute(key, value, sl.Strings, sl.link.Attributes)
}

// Tracestate returns the tracestate from the span link.
func (sl *InternalSpanLink) Tracestate() string {
	return sl.Strings.Get(sl.link.TracestateRef)
}

// Attributes returns the attributes of the span link.
func (sl *InternalSpanLink) Attributes() map[uint32]*AnyValue {
	return sl.link.Attributes
}

// InternalSpanEvent is the canonical internal span event structure
type InternalSpanEvent struct {
	// Strings is a pointer to the strings slice (Shared across a tracer payload)
	Strings *StringTable
	event   *SpanEvent
}

// Name returns the name from the span event.
func (se *InternalSpanEvent) Name() string {
	return se.Strings.Get(se.event.NameRef)
}

// Attributes returns the attributes of the span event.
func (se *InternalSpanEvent) Attributes() map[uint32]*AnyValue {
	return se.event.Attributes
}

// Msgsize returns the size of the message when serialized.
func (se *SpanEvent) Msgsize() int {
	size := 0
	size += msgp.MapHeaderSize                   // Map
	size += msgp.Uint32Size + msgp.Uint64Size    // Time
	size += msgp.Uint32Size + msgp.Uint32Size    // NameRef
	size += msgp.Uint32Size + msgp.MapHeaderSize // Attributes
	for _, attr := range se.Attributes {
		size += msgp.Uint32Size + attr.Msgsize() // Key size + Attribute size
	}
	return size
}

// GetAttributeAsString gets a string attribute from the span event.
func (se *InternalSpanEvent) GetAttributeAsString(key string) (string, bool) {
	return getAttributeAsString(key, se.Strings, se.event.Attributes)
}

// SetAttributeFromString sets the attribute on an InternalSpanEvent from a string, attempting to use the most backwards compatible type possible
// for the attribute value. Meaning we will prefer DoubleValue > IntValue > StringValue to match the previous metrics vs meta behavior
func (se *InternalSpanEvent) SetAttributeFromString(key, value string) {
	setAttribute(key, FromString(se.Strings, value), se.Strings, se.event.Attributes)
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

func getAttributeAsString(key string, strTable *StringTable, attributes map[uint32]*AnyValue) (string, bool) {
	keyIdx := strTable.Lookup(key)
	if keyIdx == 0 {
		return "", false
	}
	if attr, ok := attributes[keyIdx]; ok {
		return attr.AsString(strTable), true
	}
	return "", false
}

func setStringAttribute(key, value string, strTable *StringTable, attributes map[uint32]*AnyValue) {
	setAttribute(key, &AnyValue{
		Value: &AnyValue_StringValueRef{
			StringValueRef: strTable.Add(value),
		},
	}, strTable, attributes)
}

func setFloat64Attribute(key string, value float64, strTable *StringTable, attributes map[uint32]*AnyValue) {
	setAttribute(key, &AnyValue{
		Value: &AnyValue_DoubleValue{
			DoubleValue: value,
		},
	}, strTable, attributes)
}

func setAttribute(key string, value *AnyValue, strTable *StringTable, attributes map[uint32]*AnyValue) {
	newKeyIdx := strTable.Add(key)
	attributes[newKeyIdx] = value
}

func deleteAttribute(key string, strTable *StringTable, attributes map[uint32]*AnyValue) {
	keyIdx := strTable.Lookup(key)
	if keyIdx != 0 {
		delete(attributes, keyIdx)
	}
}

func markAttributeMapStringsUsed(usedStrings []bool, strTable *StringTable, attributes map[uint32]*AnyValue) {
	for keyIdx, attr := range attributes {
		usedStrings[keyIdx] = true
		markAttributeStringUsed(usedStrings, strTable, attr)
	}
}

// markAttributeStringUsed marks the string referenced by the value as used
// This is used to track which strings are used in the span and can be removed from the string table
func markAttributeStringUsed(usedStrings []bool, strTable *StringTable, value *AnyValue) {
	switch v := value.Value.(type) {
	case *AnyValue_StringValueRef:
		usedStrings[v.StringValueRef] = true
	case *AnyValue_ArrayValue:
		for _, value := range v.ArrayValue.Values {
			markAttributeStringUsed(usedStrings, strTable, value)
		}
	case *AnyValue_KeyValueList:
		for _, kv := range v.KeyValueList.KeyValues {
			usedStrings[kv.Key] = true
			markAttributeStringUsed(usedStrings, strTable, kv.Value)
		}
	}
}

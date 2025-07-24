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

	"github.com/tinylib/msgp/msgp"
)

// StringTable is a table of strings that is used to store the de-duplicated strings in a trace
// Strings are reference counted, when a string has no references it is removed from the table.
type StringTable struct {
	strings []string
	refs    []uint32 // ref count for each string at string[i]
	lookup  map[string]uint32
}

// NewStringTable creates a new string table, always starts with an empty string at index 0
func NewStringTable() *StringTable {
	return &StringTable{
		strings: []string{""},
		refs:    []uint32{0},
		lookup:  map[string]uint32{"": 0},
	}
}

// StringTableFromArray creates a new string table from an array of already de-duplicated strings
func StringTableFromArray(strings []string) *StringTable {
	st := &StringTable{
		strings: make([]string, len(strings)),
		refs:    make([]uint32, len(strings)),
		lookup:  make(map[string]uint32, len(strings)),
	}
	for i, str := range strings {
		st.strings[i+1] = str
		st.refs[i+1] = 1
		st.lookup[str] = uint32(i + 1)
	}
	return st
}

func (s *StringTable) Msgsize() int {
	size := 0
	size += msgp.ArrayHeaderSize
	size += msgp.StringPrefixSize * len(s.strings)
	for _, str := range s.strings {
		size += len(str)
	}
	return size
}

// addUnchecked adds a string to the string table without checking for duplicates
func (s *StringTable) addUnchecked(str string) uint32 {
	s.strings = append(s.strings, str)
	s.refs = append(s.refs, 1)
	s.lookup[str] = uint32(len(s.strings) - 1)
	return uint32(len(s.strings) - 1)
}

// Add adds a string to the string table if it doesn't already exist and returns the index of the string
// This is counted as a new reference to the string.
func (s *StringTable) Add(str string) uint32 {
	if idx, ok := s.lookup[str]; ok {
		s.refs[idx]++
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
// This does not add a new reference to that string.
func (s *StringTable) Lookup(str string) uint32 {
	if idx, ok := s.lookup[str]; ok {
		return idx
	}
	return 0
}

// DecrementReference decrements the ref count for the string at the given index
// If the ref count reaches 0, the string is set to the empty string and the lookup is removed
func (s *StringTable) DecrementReference(idx uint32) {
	s.refs[idx]--
	if s.refs[idx] == 0 {
		// Remove string from lookup table as well, as it is no longer referenced
		delete(s.lookup, s.strings[idx])
		s.strings[idx] = ""
	}
}

// InternalTracerPayload is a tracer payload structure that is optimized for trace-agent usage
// Namely it stores Attributes as a map for fast key lookups
type InternalTracerPayload struct {
	// array of strings referenced in this tracer payload, its chunks and spans
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

// ToProto converts an InternalTracerPayload to a proto TracerPayload
// It builds a new string table with only the strings that are used in the tracer payload
// This ensures that any chunks or spans that were removed from this payload will not have any strings in the string table
// that are no longer referenced. (As tracking these using the ref count is too expensive / error prone)
func (tp *InternalTracerPayload) ToProto() *TracerPayload {
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
	chunks := make([]*TraceChunk, len(tp.Chunks))
	for i, chunk := range tp.Chunks {
		chunks[i] = chunk.ToProto(usedStrings)
	}
	// We do not adjust the existing string table in case another goroutine is using it (e.g. the trace writer and span concentrator concurrently)
	sanitizedStrings := make([]string, len(tp.Strings.strings))
	for i, used := range usedStrings {
		if used {
			sanitizedStrings[i] = tp.Strings.strings[i]
		}
	}

	return &TracerPayload{
		Strings:            sanitizedStrings,
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

func (tp *InternalTracerPayload) Hostname() string {
	return tp.Strings.Get(tp.hostnameRef)
}

func (tp *InternalTracerPayload) AppVersion() string {
	return tp.Strings.Get(tp.appVersionRef)
}

func (tp *InternalTracerPayload) LanguageName() string {
	return tp.Strings.Get(tp.languageNameRef)
}

// SetLanguageName sets the language name in the string table
func (tp *InternalTracerPayload) SetLanguageName(name string) {
	tp.Strings.DecrementReference(tp.languageNameRef)
	tp.languageNameRef = tp.Strings.Add(name)
}

func (tp *InternalTracerPayload) LanguageVersion() string {
	return tp.Strings.Get(tp.languageVersionRef)
}

// SetLanguageVersion sets the language version in the string table
func (tp *InternalTracerPayload) SetLanguageVersion(version string) {
	tp.Strings.DecrementReference(tp.languageVersionRef)
	tp.languageVersionRef = tp.Strings.Add(version)
}

func (tp *InternalTracerPayload) TracerVersion() string {
	return tp.Strings.Get(tp.tracerVersionRef)
}

// SetTracerVersion sets the tracer version in the string table
func (tp *InternalTracerPayload) SetTracerVersion(version string) {
	tp.Strings.DecrementReference(tp.tracerVersionRef)
	tp.tracerVersionRef = tp.Strings.Add(version)
}

func (tp *InternalTracerPayload) ContainerID() string {
	return tp.Strings.Get(tp.containerIDRef)
}

func (tp *InternalTracerPayload) SetContainerID(containerID string) {
	tp.Strings.DecrementReference(tp.containerIDRef)
	tp.containerIDRef = tp.Strings.Add(containerID)
}

func (tp *InternalTracerPayload) Env() string {
	return tp.Strings.Get(tp.envRef)
}

func (tp *InternalTracerPayload) SetEnv(env string) {
	tp.Strings.DecrementReference(tp.envRef)
	tp.envRef = tp.Strings.Add(env)
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
	setStringAttribute(key, value, tp.Strings, tp.Attributes)
}

// Cut cuts off a new tracer payload from the `p` with [0, i-1] chunks
// and keeps [i, n-1] chunks in the original payload `p`.
func (tp *InternalTracerPayload) Cut(i int) *InternalTracerPayload {
	if i < 0 {
		i = 0
	}
	if i > len(tp.Chunks) {
		i = len(tp.Chunks)
	}
	newPayload := InternalTracerPayload{
		Strings:            tp.Strings,
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
	tp.Chunks = tp.Chunks[i:]
	return &newPayload
}

// InternalTraceChunk is a trace chunk structure that is optimized for trace-agent usage
// Namely it stores Attributes as a map for fast key lookups and holds a pointer to the strings slice
// so a trace chunk holds all local context necessary to understand all fields
type InternalTraceChunk struct {
	Strings          *StringTable
	Priority         int32
	originRef        uint32
	Attributes       map[uint32]*AnyValue
	Spans            []*InternalSpan
	DroppedTrace     bool
	TraceID          []byte
	decisionMakerRef uint32
}

func NewInternalTraceChunk(strings *StringTable, priority int32, origin string, attributes map[uint32]*AnyValue, spans []*InternalSpan, droppedTrace bool, traceID []byte, decisionMaker string) *InternalTraceChunk {
	return &InternalTraceChunk{
		Strings:          strings,
		Priority:         priority,
		originRef:        strings.Add(origin),
		Attributes:       attributes,
		Spans:            spans,
		DroppedTrace:     droppedTrace,
		TraceID:          traceID,
		decisionMakerRef: strings.Add(decisionMaker),
	}
}

// TODO: add a test to verify we have all fields
func (c *InternalTraceChunk) ShallowCopy() *InternalTraceChunk {
	return &InternalTraceChunk{
		Strings:          c.Strings,
		Priority:         c.Priority,
		originRef:        c.originRef,
		Attributes:       c.Attributes,
		Spans:            c.Spans,
		DroppedTrace:     c.DroppedTrace,
		TraceID:          c.TraceID,
		decisionMakerRef: c.decisionMakerRef,
	}
}

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

func (c *InternalTraceChunk) Origin() string {
	return c.Strings.Get(c.originRef)
}

func (c *InternalTraceChunk) SetOrigin(origin string) {
	c.Strings.DecrementReference(c.originRef)
	c.originRef = c.Strings.Add(origin)
}

func (c *InternalTraceChunk) DecisionMaker() string {
	return c.Strings.Get(c.decisionMakerRef)
}

func (c *InternalTraceChunk) SetDecisionMaker(decisionMaker string) {
	c.Strings.DecrementReference(c.decisionMakerRef)
	c.decisionMakerRef = c.Strings.Add(decisionMaker)
}

// GetAttributeAsString returns the attribute as a string, or an empty string if the attribute is not found
func (c *InternalTraceChunk) GetAttributeAsString(key string) (string, bool) {
	return getAttributeAsString(key, c.Strings, c.Attributes)
}

func (c *InternalTraceChunk) SetStringAttribute(key, value string) {
	setStringAttribute(key, value, c.Strings, c.Attributes)
}

// ToProto converts an InternalTraceChunk to a proto TraceChunk and marks any strings referenced in this chunk in usedStrings
func (c *InternalTraceChunk) ToProto(usedStrings []bool) *TraceChunk {
	usedStrings[c.originRef] = true
	usedStrings[c.decisionMakerRef] = true
	markAttributeMapStringsUsed(usedStrings, c.Strings, c.Attributes)
	spans := make([]*Span, len(c.Spans))
	for i, span := range c.Spans {
		spans[i] = span.ToProto(usedStrings)
	}
	return &TraceChunk{
		Priority:         c.Priority,
		OriginRef:        c.originRef,
		Attributes:       c.Attributes,
		Spans:            spans,
		DroppedTrace:     c.DroppedTrace,
		TraceID:          c.TraceID,
		DecisionMakerRef: c.decisionMakerRef,
	}
}

// InternalSpan is a span structure that is optimized for trace-agent usage
// Namely it stores Attributes as a map for fast key lookups and holds a pointer to the strings slice
// so a span holds all local context necessary to understand all fields
type InternalSpan struct {
	// Strings is a pointer to the strings slice (Shared across a tracer payload)
	Strings *StringTable
	span    *Span
}

func NewInternalSpan(strings *StringTable, span *Span) *InternalSpan {
	return &InternalSpan{
		Strings: strings,
		span:    span,
	}
}

func (s *InternalSpan) ShallowCopy() *InternalSpan {
	return &InternalSpan{
		Strings: s.Strings,
		span:    s.span.ShallowCopy(),
	}
}

func (s *InternalSpan) ToProto(usedStrings []bool) *Span {
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
		// TODO: add a test to verify we have all fields
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
	str += fmt.Sprintf("Service: (%s, at %d, #refs %d), ", s.Service(), s.span.ServiceRef, s.Strings.refs[s.span.ServiceRef])
	str += fmt.Sprintf("Name: (%s, at %d, #refs %d), ", s.Name(), s.span.NameRef, s.Strings.refs[s.span.NameRef])
	str += fmt.Sprintf("Resource: (%s, at %d, #refs %d), ", s.Resource(), s.span.ResourceRef, s.Strings.refs[s.span.ResourceRef])
	str += fmt.Sprintf("SpanID: %d, ", s.span.SpanID)
	str += fmt.Sprintf("ParentID: %d, ", s.span.ParentID)
	str += fmt.Sprintf("Start: %d, ", s.span.Start)
	str += fmt.Sprintf("Duration: %d, ", s.span.Duration)
	str += fmt.Sprintf("Error: %t, ", s.span.Error)
	str += fmt.Sprintf("Attributes: %v, ", s.span.Attributes)
	str += fmt.Sprintf("Type: (%s, at %d, #refs %d), ", s.Type(), s.span.TypeRef, s.Strings.refs[s.span.TypeRef])
	str += fmt.Sprintf("Links: %v, ", s.Links())
	str += fmt.Sprintf("Events: %v, ", s.Events())
	str += fmt.Sprintf("Env: (%s, at %d, #refs %d), ", s.Env(), s.span.EnvRef, s.Strings.refs[s.span.EnvRef])
	str += fmt.Sprintf("Version: (%s, at %d, #refs %d), ", s.Version(), s.span.VersionRef, s.Strings.refs[s.span.VersionRef])
	str += fmt.Sprintf("Component: (%s, at %d, #refs %d), ", s.Component(), s.span.ComponentRef, s.Strings.refs[s.span.ComponentRef])
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

func (s *InternalSpan) LenLinks() int {
	return len(s.span.Links)
}

// TODO: how can we maintain this as we add more fields?
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

func (s *InternalSpan) Service() string {
	return s.Strings.Get(s.span.ServiceRef)
}

func (s *InternalSpan) SetService(svc string) {
	s.Strings.DecrementReference(s.span.ServiceRef)
	s.span.ServiceRef = s.Strings.Add(svc)
}

func (s *InternalSpan) Name() string {
	return s.Strings.Get(s.span.NameRef)
}

func (s *InternalSpan) SetName(name string) {
	s.Strings.DecrementReference(s.span.NameRef)
	s.span.NameRef = s.Strings.Add(name)
}

func (s *InternalSpan) Resource() string {
	return s.Strings.Get(s.span.ResourceRef)
}

func (s *InternalSpan) SetResource(resource string) {
	s.Strings.DecrementReference(s.span.ResourceRef)
	s.span.ResourceRef = s.Strings.Add(resource)
}

func (s *InternalSpan) Type() string {
	return s.Strings.Get(s.span.TypeRef)
}

func (s *InternalSpan) SetType(t string) {
	s.Strings.DecrementReference(s.span.TypeRef)
	s.span.TypeRef = s.Strings.Add(t)
}

func (s *InternalSpan) Env() string {
	return s.Strings.Get(s.span.EnvRef)
}

func (s *InternalSpan) SetEnv(e string) {
	s.Strings.DecrementReference(s.span.EnvRef)
	s.span.EnvRef = s.Strings.Add(e)
}

func (s *InternalSpan) ParentID() uint64 {
	return s.span.ParentID
}

func (s *InternalSpan) SetParentID(parentID uint64) {
	s.span.ParentID = parentID
}

func (s *InternalSpan) SpanID() uint64 {
	return s.span.SpanID
}

func (s *InternalSpan) SetSpanID(spanID uint64) {
	s.span.SpanID = spanID
}

func (s *InternalSpan) Start() uint64 {
	return s.span.Start
}

func (s *InternalSpan) SetStart(start uint64) {
	s.span.Start = start
}

func (s *InternalSpan) Error() bool {
	return s.span.Error
}

func (s *InternalSpan) SetError(error bool) {
	s.span.Error = error
}

func (s *InternalSpan) Attributes() map[uint32]*AnyValue {
	return s.span.Attributes
}

func (s *InternalSpan) Duration() uint64 {
	return s.span.Duration
}

func (s *InternalSpan) SetDuration(duration uint64) {
	s.span.Duration = duration
}

func (s *InternalSpan) Kind() SpanKind {
	return s.span.Kind
}

func (s *InternalSpan) Component() string {
	return s.Strings.Get(s.span.ComponentRef)
}

func (s *InternalSpan) Version() string {
	return s.Strings.Get(s.span.VersionRef)
}

// GetAttributeAsString returns the attribute as a string, or an empty string if the attribute is not found
func (s *InternalSpan) GetAttributeAsString(key string) (string, bool) {
	return getAttributeAsString(key, s.Strings, s.span.Attributes)
}

// GetAttributeAsFloat64 returns the attribute as a float64 and a boolean indicating if the attribute was found AND it was able to be converted to a float64
func (s *InternalSpan) GetAttributeAsFloat64(key string) (float64, bool) {
	if attr, ok := s.span.Attributes[s.Strings.Lookup(key)]; ok {
		doubleVal, err := attr.AsDoubleValue(s.Strings)
		if err != nil {
			return 0, false
		}
		return doubleVal, true
	}
	return 0, false
}

func (s *InternalSpan) SetStringAttribute(key, value string) {
	if s.span.Attributes == nil {
		s.span.Attributes = make(map[uint32]*AnyValue)
	}
	setStringAttribute(key, value, s.Strings, s.span.Attributes)
}

func (s *InternalSpan) SetFloat64Attribute(key string, value float64) {
	if s.span.Attributes == nil {
		s.span.Attributes = make(map[uint32]*AnyValue)
	}
	setFloat64Attribute(key, value, s.Strings, s.span.Attributes)
}

// SetAttributeFromString sets the attribute from a string, attempting to use the most backwards compatible type possible
// for the attribute value. Meaning we will prefer DoubleValue > IntValue > StringValue to match the previous metrics vs meta behavior
func (s *InternalSpan) SetAttributeFromString(key, value string) {
	setAttribute(key, FromString(s.Strings, value), s.Strings, s.span.Attributes)
}

func (s *InternalSpan) DeleteAttribute(key string) {
	deleteAttribute(key, s.Strings, s.span.Attributes)
}

func (s *InternalSpan) MapStringAttributes(f func(k, v string) string) {
	for k, v := range s.span.Attributes {
		// TODO: we could cache the results of these transformations
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

// InternalSpanLink is a span link structure that is optimized for trace-agent usage
// Namely it stores Attributes as a map for fast key lookups
type InternalSpanLink struct {
	// Strings is a pointer to the strings slice (Shared across a tracer payload)
	Strings *StringTable
	link    *SpanLink
}

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

func (sl *InternalSpanLink) TraceID() []byte {
	return sl.link.TraceID
}

func (sl *InternalSpanLink) SpanID() uint64 {
	return sl.link.SpanID
}

func (sl *InternalSpanLink) Flags() uint32 {
	return sl.link.Flags
}

func (sl *InternalSpanLink) GetAttributeAsString(key string) (string, bool) {
	return getAttributeAsString(key, sl.Strings, sl.link.Attributes)
}

func (sl *InternalSpanLink) SetStringAttribute(key, value string) {
	setStringAttribute(key, value, sl.Strings, sl.link.Attributes)
}

func (sl *InternalSpanLink) Tracestate() string {
	return sl.Strings.Get(sl.link.TracestateRef)
}

// InternalSpanEvent is a span event structure that is optimized for trace-agent usage
// Namely it stores Attributes as a map for fast key lookups
type InternalSpanEvent struct {
	// Strings is a pointer to the strings slice (Shared across a tracer payload)
	Strings *StringTable
	event   *SpanEvent
}

func (se *InternalSpanEvent) Attributes() map[uint32]*AnyValue {
	return se.event.Attributes
}

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

func (se *InternalSpanEvent) GetAttributeAsString(key string) (string, bool) {
	return getAttributeAsString(key, se.Strings, se.event.Attributes)
}

// SetAttributeFromString sets the attribute on an InternalSpanEvent from a string, attempting to use the most backwards compatible type possible
// for the attribute value. Meaning we will prefer DoubleValue > IntValue > StringValue to match the previous metrics vs meta behavior
func (se *InternalSpanEvent) SetAttributeFromString(key, value string) {
	se.event.Attributes[se.Strings.Add(key)] = FromString(se.Strings, value)
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

// DecStringRefs decrements the ref count for all strings (including nested values) in this AnyValue as this value is being removed / replaced
// Noop for non-string values
func (attr *AnyValue) RemoveStringRefs(strTable *StringTable) {
	switch v := attr.Value.(type) {
	case *AnyValue_StringValueRef:
		strTable.DecrementReference(v.StringValueRef)
	case *AnyValue_ArrayValue:
		for _, value := range v.ArrayValue.Values {
			value.RemoveStringRefs(strTable)
		}
	case *AnyValue_KeyValueList:
		for _, kv := range v.KeyValueList.KeyValues {
			strTable.DecrementReference(kv.Key)
			kv.Value.RemoveStringRefs(strTable)
		}
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
	if attr, ok := attributes[strTable.Lookup(key)]; ok {
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
	if oldVal, ok := attributes[newKeyIdx]; ok {
		// Key already exists, remove the old value's string references
		oldVal.RemoveStringRefs(strTable)
	}
	attributes[newKeyIdx] = value
}

func deleteAttribute(key string, strTable *StringTable, attributes map[uint32]*AnyValue) {
	keyIdx := strTable.Lookup(key)
	if keyIdx != 0 {
		// Remove key ref and any value ref
		strTable.DecrementReference(keyIdx)
		attributes[keyIdx].RemoveStringRefs(strTable)
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

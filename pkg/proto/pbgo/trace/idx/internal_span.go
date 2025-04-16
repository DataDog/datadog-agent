// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package idx

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

// InternalSpanLink is a span link structure that is optimized for trace-agent usage
// Namely it stores Attributes as a map for fast key lookups
type InternalSpanLink struct {
	// Strings is a pointer to the strings slice (Shared across a tracer payload)
	Strings       *StringTable
	TraceID       []byte
	SpanID        uint64
	Attributes    map[uint32]*AnyValue
	TracestateRef uint32
	FlagsRef      uint32
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

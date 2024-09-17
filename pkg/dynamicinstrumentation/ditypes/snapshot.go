// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ditypes

import (
	"github.com/google/uuid"
)

// SnapshotUpload is a single message sent to the datadog back containing the
// snapshot and metadata
type SnapshotUpload struct {
	Service  string `json:"service"`
	Message  string `json:"message"`
	DDSource string `json:"ddsource"`
	DDTags   string `json:"ddtags"`
	Logger   struct {
		Name       string `json:"name"`
		Method     string `json:"method"`
		Version    int    `json:"version,omitempty"`
		ThreadID   int    `json:"thread_id,omitempty"`
		ThreadName string `json:"thread_name,omitempty"`
	} `json:"logger"`

	Debugger struct {
		Snapshot `json:"snapshot"`
	} `json:"debugger"`

	// TODO: check precision (ms, ns etc)
	Duration int64 `json:"duration"`

	DD *TraceCorrelation `json:"dd,omitempty"`
}

// Snapshot is a single instance of a function invocation and all
// captured data
type Snapshot struct {
	ID        *uuid.UUID `json:"id"`
	Timestamp int64      `json:"timestamp"`

	Language        string `json:"language"`
	ProbeInSnapshot `json:"probe"`

	Captures `json:"captures"`

	Errors []EvaluationError `json:"evaluationErrors,omitempty"`

	Stack []StackFrame `json:"stack"`
}

// Captures contains captured data at various points during a function invocation
type Captures struct {
	Entry  *Capture `json:"entry,omitempty"`
	Return *Capture `json:"return,omitempty"`

	Lines map[string]Capture `json:"lines,omitempty"`
}

// ProbeInSnapshot contains information about the probe that produced a snapshot
type ProbeInSnapshot struct {
	ID         string `json:"id"`
	EvaluateAt string `json:"evaluateAt,omitempty"`
	Tags       string `json:"tags,omitempty"`
	Version    int    `json:"version,omitempty"`

	ProbeLocation `json:"location"`
}

// ProbeLocation represents where a snapshot was originally captured
type ProbeLocation struct {
	Type   string   `json:"type,omitempty"`
	Method string   `json:"method,omitempty"`
	Lines  []string `json:"lines,omitempty"`
	File   string   `json:"file,omitempty"`
}

// CapturedValueMap maps type names to their values
type CapturedValueMap = map[string]*CapturedValue

// Capture represents all the captured values in a snapshot
type Capture struct {
	Arguments CapturedValueMap `json:"arguments,omitempty"`
	Locals    CapturedValueMap `json:"locals,omitempty"`
}

// CapturedValue represents the value of a captured type
type CapturedValue struct {
	Type string `json:"type"`

	// we use a string pointer so the empty string is marshalled
	Value *string `json:"value,omitempty"`

	Fields   map[string]*CapturedValue `json:"fields,omitempty"`
	Entries  [][]CapturedValue         `json:"entries,omitempty"`
	Elements []CapturedValue           `json:"elements,omitempty"`

	NotCapturedReason string `json:"notCapturedReason,omitempty"`
	IsNull            bool   `json:"isNull,omitempty"`

	Size      string `json:"size,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// EvaluationError expresses why a value could not be evaluated
type EvaluationError struct {
	Expr    string `json:"expr"`
	Message string `json:"message"`
}

// TraceCorrelation contains fields that correlate a snapshot with traces
type TraceCorrelation struct {
	TraceID string `json:"trace_id,omitempty"`
	SpanID  string `json:"span_id,omitempty"`
}

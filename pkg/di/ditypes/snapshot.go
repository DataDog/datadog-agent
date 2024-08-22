// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package ditypes

import (
	"github.com/google/uuid"
)

type SnapshotUpload struct {
	Service  string `json:"service"`
	Message  string `json:"message"`
	DDSource string `json:"ddsource"`
	DDTags   string `json:"ddtags"`
	Logger   struct {
		Name       string `json:"name"`
		Method     string `json:"method"`
		Version    int    `json:"version,omitempty"`
		ThreadId   int    `json:"thread_id,omitempty"`
		ThreadName string `json:"thread_name,omitempty"`
	} `json:"logger"`

	Debugger struct {
		Snapshot `json:"snapshot"`
	} `json:"debugger"`

	// TODO: check precision (ms, ns etc)
	Duration int64 `json:"duration"`

	DD *TraceCorrelation `json:"dd,omitempty"`
}

type Snapshot struct {
	ID        *uuid.UUID `json:"id"`
	Timestamp int64      `json:"timestamp"`

	Language        string `json:"language"`
	ProbeInSnapshot `json:"probe"`

	Captures `json:"captures"`

	Errors []EvaluationError `json:"evaluationErrors,omitempty"`

	Stack []StackFrame `json:"stack"`
}

type Captures struct {
	Entry  *Capture `json:"entry,omitempty"`
	Return *Capture `json:"return,omitempty"`

	Lines map[string]Capture `json:"lines,omitempty"`
}

type ProbeInSnapshot struct {
	ID         string `json:"id"`
	EvaluateAt string `json:"evaluateAt,omitempty"`
	Tags       string `json:"tags,omitempty"`
	Version    int    `json:"version,omitempty"`

	ProbeLocation `json:"location"`
}

type ProbeLocation struct {
	Type   string   `json:"type,omitempty"`
	Method string   `json:"method,omitempty"`
	Lines  []string `json:"lines,omitempty"`
	File   string   `json:"file,omitempty"`
}

type CapturedValueMap = map[string]*CapturedValue

type Capture struct {
	Arguments CapturedValueMap `json:"arguments,omitempty"`
	Locals    CapturedValueMap `json:"locals,omitempty"`
}

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

type EvaluationError struct {
	Expr    string `json:"expr"`
	Message string `json:"message"`
}

type TraceCorrelation struct {
	TraceID string `json:"trace_id,omitempty"`
	SpanID  string `json:"span_id,omitempty"`
}

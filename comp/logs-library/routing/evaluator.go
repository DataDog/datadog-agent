// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package routing

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// ErrOTTLNotImplemented is returned by StubCompiler when a non-empty endpoint
// selector expression is supplied before upstream OTTL integration (PR #59688).
var ErrOTTLNotImplemented = errors.New("OTTL endpoint filter evaluation is not yet implemented")

// RecordView provides read-only access to a processed log record before rendering
// and encoding. LocalTags returns unfiltered local Agent tags, including tags that
// export tag_filters may later remove from outgoing payloads.
type RecordView struct {
	msg *message.Message
}

// NewRecordView wraps a processed log message for read-only selector evaluation.
func NewRecordView(msg *message.Message) RecordView {
	return RecordView{msg: msg}
}

// Content returns the processed message bytes. Callers must not mutate the slice.
func (v RecordView) Content() []byte {
	if v.msg == nil {
		return nil
	}
	return v.msg.GetContent()
}

// StructuredAttribute retrieves a dot-delimited string attribute from structured
// content before rendering destroys structured access.
func (v RecordView) StructuredAttribute(path string) (string, bool) {
	if v.msg == nil {
		return "", false
	}
	return v.msg.GetStructuredAttribute(path)
}

// LocalTags returns the record's unfiltered local tags (origin tags, sourcecategory,
// and source config tags), unaffected by export tag_filters.
func (v RecordView) LocalTags() []string {
	if v.msg == nil || v.msg.Origin == nil {
		return nil
	}
	return v.msg.Origin.Tags()
}

// Source returns the record's effective source (including remap_source overrides).
func (v RecordView) Source() string {
	if v.msg == nil || v.msg.Origin == nil {
		return ""
	}
	return v.msg.Origin.Source()
}

// Service returns the record's service metadata.
func (v RecordView) Service() string {
	if v.msg == nil || v.msg.Origin == nil {
		return ""
	}
	return v.msg.Origin.Service()
}

// SourceCategory returns the configured sourcecategory metadata, if any.
func (v RecordView) SourceCategory() string {
	if v.msg == nil || v.msg.Origin == nil || v.msg.Origin.LogSource == nil || v.msg.Origin.LogSource.Config == nil {
		return ""
	}
	return v.msg.Origin.LogSource.Config.SourceCategory
}

// Status returns the record's normalized log status (defaults to "info").
func (v RecordView) Status() string {
	if v.msg == nil {
		return ""
	}
	return v.msg.GetStatus()
}

// Hostname returns the record's hostname metadata, if set on the message.
func (v RecordView) Hostname() string {
	if v.msg == nil {
		return ""
	}
	return v.msg.Hostname
}

// Evaluator evaluates an endpoint selection predicate against a processed record.
// Implementations must be local, bounded, concurrency-safe, and read-only.
type Evaluator interface {
	Match(view RecordView) (bool, error)
}

// EvaluatorFunc adapts a function to Evaluator.
type EvaluatorFunc func(view RecordView) (bool, error)

// Match invokes the underlying predicate function.
func (f EvaluatorFunc) Match(view RecordView) (bool, error) {
	return f(view)
}

// Compiler compiles an endpoint selector expression into a reusable Evaluator.
type Compiler interface {
	Compile(expression string) (Evaluator, error)
}

// MatchAllEvaluator matches every record with zero allocations.
type MatchAllEvaluator struct{}

// Match always returns true, nil.
func (MatchAllEvaluator) Match(RecordView) (bool, error) { return true, nil }

// StubCompiler is the production OTTL compiler stub pending PR #59688.
// Empty expressions compile to MatchAllEvaluator; non-empty expressions fail fast.
type StubCompiler struct{}

// Compile returns MatchAllEvaluator for empty expressions and ErrOTTLNotImplemented otherwise.
func (StubCompiler) Compile(expression string) (Evaluator, error) {
	if strings.TrimSpace(expression) == "" {
		return MatchAllEvaluator{}, nil
	}
	return nil, fmt.Errorf("%w: non-empty endpoint selector expression rejected by stub compiler", ErrOTTLNotImplemented)
}

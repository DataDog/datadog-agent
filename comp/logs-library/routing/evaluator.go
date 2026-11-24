// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package routing

import "github.com/DataDog/datadog-agent/pkg/logs/message"

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

// LocalTags returns the record's unfiltered local tags (origin tags, sourcecategory,
// and source config tags), unaffected by export tag_filters.
func (v RecordView) LocalTags() []string {
	if v.msg == nil || v.msg.Origin == nil {
		return nil
	}
	return v.msg.Origin.Tags()
}

// Status returns the record's normalized log status (defaults to "info").
func (v RecordView) Status() string {
	if v.msg == nil {
		return ""
	}
	return v.msg.GetStatus()
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

// MatchAllEvaluator matches every record with zero allocations.
type MatchAllEvaluator struct{}

// Match always returns true, nil.
func (MatchAllEvaluator) Match(RecordView) (bool, error) { return true, nil }

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

// Label is a label for a log message.
type Label uint32

const (
	startGroup Label = iota
	noAggregate
	aggregate

	defaultLabelSource = "default"
)

type messageContext struct {
	rawMessage []byte
	// tokens is a view over the tokenizer's scratch buffers, valid only for the
	// duration of Label. Heuristics read it via tokens.Borrow() and must clone
	// (tokens.Clone()) before retaining it. It may be empty (check tokens.Empty()).
	tokens          BorrowedTokens
	label           Label
	labelAssignedBy string
}

// Heuristic is an interface representing a strategy to label log messages.
type Heuristic interface {
	// ProcessAndContinue processes a log message and annotates the context with a label. It returns false if the message should be done processing.
	// Heuristic implementations may mutate the message context but must do so synchronously.
	ProcessAndContinue(*messageContext) bool
}

// Labeler classifies a log line as startGroup, noAggregate, or aggregate.
// tokens are pre-computed by the Preprocessor's Tokenizer step and passed here
// so heuristics can inspect them without re-tokenizing. They are borrowed for
// the duration of Label; a heuristic that retains them must Clone first.
type Labeler interface {
	Label(content []byte, tokens BorrowedTokens) Label
}

// labeler is the real implementation: it chains a set of heuristics and returns
// the label chosen by the first heuristic that signals it is done.
type labeler struct {
	lablerHeuristics    []Heuristic
	analyticsHeuristics []Heuristic
}

// NewLabeler creates a new Labeler with the given heuristics.
// lablerHeuristics are used to mutate the label of a log message.
// analyticsHeuristics are used to analyze the log message and labeling process
// for the status page and telemetry.
func NewLabeler(lablerHeuristics []Heuristic, analyticsHeuristics []Heuristic) Labeler {
	return &labeler{
		lablerHeuristics:    lablerHeuristics,
		analyticsHeuristics: analyticsHeuristics,
	}
}

// Label labels a log message using the provided content and pre-computed tokens.
func (l *labeler) Label(content []byte, tokens BorrowedTokens) Label {
	context := &messageContext{
		rawMessage:      content,
		tokens:          tokens,
		label:           aggregate,
		labelAssignedBy: defaultLabelSource,
	}

	for _, h := range l.lablerHeuristics {
		if !h.ProcessAndContinue(context) {
			break
		}
	}
	// analyticsHeuristics are always run and don't change the final label
	for _, h := range l.analyticsHeuristics {
		h.ProcessAndContinue(context)
	}
	return context.label
}

// NoopLabeler is a Labeler that always returns noAggregate without any processing.
// Use this for pipeline paths that don't need auto-multiline detection
// (e.g. pass-through, regex multiline).
type NoopLabeler struct{}

// NewNoopLabeler returns a new NoopLabeler.
func NewNoopLabeler() *NoopLabeler {
	return &NoopLabeler{}
}

// Label always returns noAggregate without inspecting content or tokens.
func (l *NoopLabeler) Label(_ []byte, _ BorrowedTokens) Label {
	return noAggregate
}

func labelToString(label Label) string {
	switch label {
	case startGroup:
		return "start_group"
	case noAggregate:
		return "no_aggregate"
	case aggregate:
		return "aggregate"
	default:
		return ""
	}
}

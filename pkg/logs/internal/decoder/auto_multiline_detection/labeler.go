// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import "github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"

// Label is a label for a log message.
type Label uint32

const (
	startGroup Label = iota
	noAggregate
	aggregate
)

type messageContext struct {
	rawMessage []byte
	// NOTE: tokens can be nil if the heuristic runs before the tokenizer.
	// Heuristic implementations must check if tokens is nil before using it.
	tokens        []tokens.Token
	tokenIndicies []int
	label         Label
}

// Heuristic is an interface representing a strategy to label log messages.
type Heuristic interface {
	// ProcessAndContinue processes a log message and annotates the context with a label. It returns false if the message should be done processing.
	// Heuristic implementations may mutate the message context but must do so synchronously.
	ProcessAndContinue(*messageContext) bool
}

// Labeler labels log messages based on a set of heuristics.
// Each Heuristic operates on the output of the previous heuristic - mutating the message context.
// A label is chosen when a herusitc signals the labeler to stop or when all herustics have been processed.
type Labeler struct {
	heuristics []Heuristic
}

// NewLabeler creates a new labeler with the given heuristics.
func NewLabeler(heuristics []Heuristic) *Labeler {
	return &Labeler{
		heuristics: heuristics,
	}
}

// Label labels a log message.
func (l *Labeler) Label(rawMessage []byte) Label {
	context := &messageContext{
		rawMessage: rawMessage,
		tokens:     nil,
		label:      aggregate,
	}
	for _, h := range l.heuristics {
		if !h.ProcessAndContinue(context) {
			return context.label
		}
	}
	return context.label
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

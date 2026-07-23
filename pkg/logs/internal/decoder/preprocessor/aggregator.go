// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"slices"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// cloneTokens retains a borrowed token slice beyond the current processing
// call. Keeping this operation explicit prevents every log line from paying
// for ownership when most consumers are synchronous.
func cloneTokens(tokens []Token) []Token {
	return slices.Clone(tokens)
}

// AggregatedMessageWithTokens pairs a completed log message with the tokens from its first line.
// Tokens are used by the sampler for pattern-based rate limiting.
type AggregatedMessageWithTokens struct {
	Msg    *message.Message
	Tokens []Token
}

// Aggregator is the interface for log line combining strategies.
// An Aggregator receives individual log lines and may buffer them, returning zero
// or more completed log messages per input. Each call to Process may return nil
// (line was buffered) or one or more messages (a group was completed).
// The returned slice is only valid until the next call to Process or Flush.
type Aggregator interface {
	// Process handles a log line and returns zero or more completed messages.
	// label is the result of labeling this message; aggregators that don't use it may ignore it.
	// tokens are borrowed until the next Process call. Aggregators must clone them before
	// retaining them; slices returned for immediate sampling may remain borrowed.
	Process(msg *message.Message, label Label, tokens []Token) []AggregatedMessageWithTokens

	// Flush returns any buffered messages and clears internal state.
	Flush() []AggregatedMessageWithTokens

	// IsEmpty returns true if the aggregator has no buffered data.
	IsEmpty() bool
}

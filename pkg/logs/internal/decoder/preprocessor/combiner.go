// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

import "github.com/DataDog/datadog-agent/pkg/logs/message"

// Combiner is the interface for log line combining strategies.
// A Combiner receives individual log lines and may buffer them, returning zero
// or more completed log messages per input. Each call to Process may return nil
// (line was buffered) or one or more messages (a group was completed).
// The returned slice is only valid until the next call to Process or Flush.
type Combiner interface {
	// Process handles a log line and returns zero or more completed messages.
	// label is the result of labeling this message; combiners that don't use it may ignore it.
	Process(msg *message.Message, label Label) []*message.Message

	// Flush returns any buffered messages and clears internal state.
	Flush() []*message.Message

	// IsEmpty returns true if the combiner has no buffered data.
	IsEmpty() bool
}

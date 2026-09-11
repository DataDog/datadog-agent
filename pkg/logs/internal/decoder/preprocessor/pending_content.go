// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// PendingContent is content a decoder pipeline stage has buffered but not
// emitted yet, captured so it can be handed over to the equivalent stage of a
// replacement decoder when a file rotates.
//
// Both buffering stages of the pipeline share this shape: the LineParser stage
// buffers CRI/Docker "partial" chunks, and the LineHandler stage buffers an
// in-progress multiline group.
type PendingContent struct {
	// Msg is the last message that contributed to the buffer; it carries the
	// origin/timestamp/tag metadata used when the group is eventually emitted.
	Msg *message.Message

	// Content is the buffered payload exactly as the stage held it (LineHandler
	// stages have already joined lines with message.EscapedLineFeed).
	Content []byte

	// RawDataLen is the number of source bytes already folded into Content. The
	// checkpoint length is deliberately not carried over so the replacement
	// tailer's auditor offsets stay relative to the new file.
	RawDataLen int

	// LinesCombined is how many source lines are already in Content.
	LinesCombined int

	// Truncated reports whether the stage had already marked the buffer as
	// truncated.
	Truncated bool

	// Stream identifies the sub-stream the content belongs to for stages that
	// buffer per stream (CRI partial lines are keyed by stdout / stderr). Empty
	// for stages with a single buffer.
	Stream string

	// Stage records which buffering sub-stage the content came from, so it is
	// restored into the same one. The Preprocessor has three independent
	// buffers and any of them can be the one holding a partial message.
	Stage string
}

// Names of the Preprocessor's buffering sub-stages, in pipeline order.
const (
	// StageJSONAggregation is the JSONAggregator's buffer of an incomplete
	// pretty-printed JSON object.
	StageJSONAggregation = "json_aggregation"
	// StageStackTraceAggregation is the StackTraceAggregator's buffer of an
	// in-progress stack trace.
	StageStackTraceAggregation = "stack_trace_aggregation"
	// StageLineAggregation is the combining/regex Aggregator's buffer of an
	// in-progress multiline group.
	StageLineAggregation = "line_aggregation"
)

// PendingContentCarrier is implemented by pipeline stages that can hand their
// in-progress buffer to the equivalent stage of another decoder instead of
// force-flushing it as a broken standalone message.
//
// Neither method emits anything downstream. Seeding re-arms the stage's
// existing aggregation-timeout timer rather than introducing a separate wait,
// so a handed-over buffer whose continuation never arrives is still flushed
// after the usual `logs_config.aggregation_timeout` window.
type PendingContentCarrier interface {
	// TakePendingContent removes and returns the stage's buffered content, or
	// nil when nothing is buffered.
	TakePendingContent() []PendingContent

	// SeedPendingContent restores content previously taken from the equivalent
	// stage of the decoder being replaced. It must be called before the decoder
	// processes any new input.
	SeedPendingContent([]PendingContent)
}

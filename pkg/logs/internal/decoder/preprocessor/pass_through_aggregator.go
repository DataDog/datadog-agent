// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"bytes"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

// PassThroughAggregator is a stateless Aggregator that emits each message immediately
// after applying truncation handling. It is the equivalent of the decoder's SingleLineHandler.
type PassThroughAggregator struct {
	collected      []CompletedMessage
	shouldTruncate bool
	lineLimit      int
}

// NewPassThroughAggregator returns a new PassThroughAggregator with the given line size limit.
func NewPassThroughAggregator(lineLimit int) *PassThroughAggregator {
	return &PassThroughAggregator{lineLimit: lineLimit}
}

// Process handles a log line, applying truncation flags if the content exceeds
// lineLimit, and returns it as a single-element slice. label is unused.
func (a *PassThroughAggregator) Process(msg *message.Message, _ Label, tokens []Token) []CompletedMessage {
	lastWasTruncated := a.shouldTruncate
	content := msg.GetContent()
	a.shouldTruncate = len(content) > a.lineLimit || msg.ParsingExtra.IsTruncated

	content = bytes.TrimSpace(content)

	if lastWasTruncated {
		content = append(message.TruncatedFlag, content...)
	}

	if a.shouldTruncate {
		content = append(content, message.TruncatedFlag...)
		metrics.LogsTruncated.Add(1)
	}

	if lastWasTruncated || a.shouldTruncate {
		msg.ParsingExtra.IsTruncated = true
		if pkgconfigsetup.Datadog().GetBool("logs_config.tag_truncated_logs") {
			msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.TruncatedReasonTag("single_line"))
		}
	}

	msg.SetContent(content)
	a.collected = append(a.collected[:0], CompletedMessage{Msg: msg, Tokens: tokens})
	return a.collected
}

// Flush returns nil since PassThroughAggregator has no buffered state.
func (a *PassThroughAggregator) Flush() []CompletedMessage {
	return nil
}

// IsEmpty always returns true since PassThroughAggregator is stateless.
func (a *PassThroughAggregator) IsEmpty() bool {
	return true
}

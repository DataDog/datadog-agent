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

// PassThroughCombiner is a stateless Combiner that emits each message immediately
// after applying truncation handling. It is the pull-model equivalent of the
// decoder's SingleLineHandler.
type PassThroughCombiner struct {
	shouldTruncate bool
	lineLimit      int
}

// NewPassThroughCombiner returns a new PassThroughCombiner with the given line size limit.
func NewPassThroughCombiner(lineLimit int) *PassThroughCombiner {
	return &PassThroughCombiner{lineLimit: lineLimit}
}

// Process handles a log line, applying truncation flags if the content exceeds
// lineLimit, and returns it as a single-element slice.
func (c *PassThroughCombiner) Process(msg *message.Message) []*message.Message {
	lastWasTruncated := c.shouldTruncate
	content := msg.GetContent()
	c.shouldTruncate = len(content) > c.lineLimit || msg.ParsingExtra.IsTruncated

	content = bytes.TrimSpace(content)

	if lastWasTruncated {
		content = append(message.TruncatedFlag, content...)
	}

	if c.shouldTruncate {
		content = append(content, message.TruncatedFlag...)
		metrics.LogsTruncated.Add(1)
	}

	if lastWasTruncated || c.shouldTruncate {
		msg.ParsingExtra.IsTruncated = true
		if pkgconfigsetup.Datadog().GetBool("logs_config.tag_truncated_logs") {
			msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.TruncatedReasonTag("single_line"))
		}
	}

	msg.SetContent(content)
	return []*message.Message{msg}
}

// Flush returns nil since PassThroughCombiner has no buffered state.
func (c *PassThroughCombiner) Flush() []*message.Message {
	return nil
}

// IsEmpty always returns true since PassThroughCombiner is stateless.
func (c *PassThroughCombiner) IsEmpty() bool {
	return true
}

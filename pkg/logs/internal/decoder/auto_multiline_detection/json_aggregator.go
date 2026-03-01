// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package automultilinedetection

import (
	"bytes"
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

// JSONAggregator aggregates pretty printed JSON messages into single line JSON messages.
type JSONAggregator struct {
	decoder         *IncrementalJSONValidator
	messageBuf      []*message.Message
	currentSize     int
	tagCompleteJSON bool
	maxContentSize  int
	prefixLen       int
	inBuf           *bytes.Buffer
	outBuf          *bytes.Buffer
}

// NewJSONAggregator creates a new JSONAggregator.
func NewJSONAggregator(tagCompleteJSON bool, maxContentSize int) *JSONAggregator {
	return &JSONAggregator{
		decoder:         NewIncrementalJSONValidator(),
		messageBuf:      make([]*message.Message, 0),
		tagCompleteJSON: tagCompleteJSON,
		maxContentSize:  maxContentSize,
		inBuf:           &bytes.Buffer{},
		outBuf:          &bytes.Buffer{},
	}
}

// findEmbeddedJSONStart returns the byte index of a trailing '{' or '['
// in content (ignoring trailing whitespace), or -1 if none is found.
// This is used to detect log lines like "error message: [" where JSON is
// embedded after a text prefix.
func findEmbeddedJSONStart(content []byte) int {
	trimmed := bytes.TrimRight(content, " \t\r\n")
	if len(trimmed) == 0 {
		return -1
	}
	last := trimmed[len(trimmed)-1]
	if last == '{' || last == '[' {
		return len(trimmed) - 1
	}
	return -1
}

// Process processes a message. If the message is a complete JSON message, it will be aggregated into a single line JSON message.
// If the message is an incomplete JSON message, it will be added to the buffer and processed later.
// If the message is not a JSON message, it will be returned as is, and any buffered messages will be flushed (unmodified).
func (r *JSONAggregator) Process(msg *message.Message) []*message.Message {
	content := msg.GetContent()

	// If buffer is empty and content is likely complete single-line JSON,
	// validate and return without parsing
	if len(r.messageBuf) == 0 && json.Valid(content) {
		return []*message.Message{msg}
	}

	r.messageBuf = append(r.messageBuf, msg)
	r.currentSize += msg.RawDataLen

	// Flush if we've exceeded the max size
	if r.currentSize > r.maxContentSize {
		return r.Flush()
	}

	// On the first message, check for a text prefix before an embedded JSON start.
	// For example: "2024-01-01 error: [" — feed only "[" to the validator.
	writeContent := content
	if len(r.messageBuf) == 1 {
		if idx := findEmbeddedJSONStart(content); idx > 0 {
			r.prefixLen = idx
			writeContent = content[idx:]
		}
	}

	switch r.decoder.Write(writeContent) {
	case Incomplete:
		break
	case Complete:
		// Read suffix info before reset — only relevant when a prefix was detected
		suffixLen := 0
		if r.prefixLen > 0 {
			suffixLen = r.decoder.SuffixLen()
		}
		r.decoder.Reset()

		// If only one message and no prefix, no need to compact
		if len(r.messageBuf) == 1 && r.prefixLen == 0 {
			r.messageBuf = r.messageBuf[:0]
			r.currentSize = 0
			return []*message.Message{msg}
		}

		// If the suffix is whitespace-only, treat it as no suffix
		// so json.Compact handles it naturally
		var suffix []byte
		if suffixLen > 0 {
			lastContent := r.messageBuf[len(r.messageBuf)-1].GetContent()
			suffix = lastContent[len(lastContent)-suffixLen:]
			if len(bytes.TrimSpace(suffix)) == 0 {
				suffix = nil
				suffixLen = 0
			}
		}

		// Build the JSON portion (skipping prefix from first message, suffix from last)
		r.inBuf.Reset()
		lastIdx := len(r.messageBuf) - 1
		for i, m := range r.messageBuf {
			c := m.GetContent()
			if i == 0 && r.prefixLen > 0 {
				c = c[r.prefixLen:]
			}
			if i == lastIdx && suffixLen > 0 {
				c = c[:len(c)-suffixLen]
			}
			r.inBuf.Write(c)
		}

		r.outBuf.Reset()
		err := json.Compact(r.outBuf, r.inBuf.Bytes())
		if err != nil {
			return r.Flush()
		}

		// Only tag the message if it's a complete JSON message that has been aggregated from more than one message
		if r.tagCompleteJSON && len(r.messageBuf) > 1 {
			msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.AggregatedJSONTag)
			metrics.TlmAutoMultilineJSONAggregatorFlush.Inc("true")
		}

		// Assemble output: prefix + compacted JSON + suffix
		if r.prefixLen > 0 {
			prefix := r.messageBuf[0].GetContent()[:r.prefixLen]
			combined := make([]byte, len(prefix)+r.outBuf.Len()+len(suffix))
			n := copy(combined, prefix)
			n += copy(combined[n:], r.outBuf.Bytes())
			copy(combined[n:], suffix)
			msg.SetContent(combined)
		} else {
			msg.SetContent(r.outBuf.Bytes())
		}

		r.messageBuf = r.messageBuf[:0]
		msg.RawDataLen = r.currentSize
		r.currentSize = 0
		r.prefixLen = 0

		return []*message.Message{msg}
	case Invalid:
		return r.Flush()
	}
	return []*message.Message{}
}

// Flush flushes the buffer and returns the messages.
func (r *JSONAggregator) Flush() []*message.Message {
	if len(r.messageBuf) > 1 {
		metrics.TlmAutoMultilineJSONAggregatorFlush.Inc("false")
	}

	r.decoder.Reset()
	msgs := r.messageBuf
	r.messageBuf = r.messageBuf[:0]
	r.currentSize = 0
	r.prefixLen = 0
	return msgs
}

// IsEmpty returns true if the buffer is empty.
func (r *JSONAggregator) IsEmpty() bool {
	return len(r.messageBuf) == 0
}

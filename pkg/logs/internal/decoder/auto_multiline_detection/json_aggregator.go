// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package automultilinedetection

import (
	"bytes"
	"encoding/json"
	"unicode"
	"unicode/utf8"

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

	switch r.decoder.Write(content) {
	case Incomplete:
		break
	case Complete:
		r.decoder.Reset()

		// If only one message, no need to compact
		if len(r.messageBuf) == 1 {
			r.messageBuf = r.messageBuf[:0]
			r.currentSize = 0
			return []*message.Message{msg}
		}

		r.outBuf.Reset()
		r.inBuf.Reset()
		for _, m := range r.messageBuf {
			r.inBuf.Write(m.GetContent())
		}
		err := json.Compact(r.outBuf, r.inBuf.Bytes())
		if err != nil {
			return r.Flush()
		}

		// Only tag the message if it's a complete JSON message that has been aggregated from more than one message
		if r.tagCompleteJSON && len(r.messageBuf) > 1 {
			msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.AggregatedJSONTag)
			metrics.TlmAutoMultilineJSONAggregatorFlush.Inc("true")
		}

		r.messageBuf = r.messageBuf[:0]
		msg.SetContent(r.outBuf.Bytes())
		msg.RawDataLen = r.currentSize
		r.currentSize = 0

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
	return msgs
}

// IsEmpty returns true if the buffer is empty.
func (r *JSONAggregator) IsEmpty() bool {
	return len(r.messageBuf) == 0
}

// isSingleLineJSON performs a scan (without full parsing) to determine if the message has
// balanced braces, which indicates that the message is likely to be complete single-line JSON.
func isSingleLineJSON(content []byte) bool {
	if len(content) == 0 {
		return false
	}

	// Must start with '{' to be a JSON object
	if content[0] != '{' {
		return false
	}

	braceCount := 0
	inString := false
	escaped := false

	for i, b := range content {
		if escaped {
			escaped = false
			continue
		}

		if b == '\\' && inString {
			escaped = true
			continue
		}

		if b == '"' {
			inString = !inString
			continue
		}

		if !inString {
			switch b {
			case '{':
				braceCount++
			case '}':
				braceCount--
				// If we hit balanced braces before the end, check if rest is whitespace
				if braceCount == 0 && i < len(content)-1 {
					return isOnlyWhitespace(content[i+1:])
				}
			}
		}
	}

	// Balanced braces = likely complete single-line JSON
	return braceCount == 0
}

// isOnlyWhitespace returns true if the data contains only whitespace characters.
func isOnlyWhitespace(data []byte) bool {
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		if !unicode.IsSpace(r) {
			return false
		}
		data = data[size:]
	}
	return true
}

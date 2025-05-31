// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"bytes"
	"encoding/json"
	logConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ChannelWriter is a buffered writer that sends log messages to a channel
// to be sent to our intake.
type ChannelWriter struct {
	Buffer  bytes.Buffer
	Channel chan *logConfig.ChannelMessage
	IsError bool
}

// NewChannelWriter returns a new channel writer.
// Implements io.Writer, used for redirecting stdout/stderr
// logs to Datadog.
func NewChannelWriter(ch chan *logConfig.ChannelMessage, isError bool) *ChannelWriter {
	return &ChannelWriter{
		Channel: ch,
		IsError: isError,
	}
}

// Write processes writes from our stdout/stderr fd and sends complete
// log messages to the channel. There are two edge cases:
// 1. May receive multiple logs in one payload, so we should split JSON logs to avoid dropped logs.
// 2. May receive part of a long log, so we should wait for a `\n` character before flushing.
func (cw *ChannelWriter) Write(p []byte) (n int, err error) {
	if bytes.IndexByte(p, '\n') < 0 {
		return cw.Buffer.Write(p)
	}

	cw.Buffer.Write(p)
	// Copy to prevent race condition
	payload := make([]byte, cw.Buffer.Len())
	copy(payload, cw.Buffer.Bytes())
	cw.Buffer.Reset()

	parts := splitJSONBytes(payload)

	// Send each message as a separate log entry
	for _, msg := range parts {
		channelMessage := &logConfig.ChannelMessage{
			Content: msg,
			IsError: cw.IsError,
		}

		select {
		case cw.Channel <- channelMessage:
			// Success case -- the channel isn't full, and can accommodate our message
		default:
			// Channel is full (i.e, we aren't flushing data to Datadog as our backend is down).
			// message will be dropped.
			log.Debug("Log dropped due to full buffer")
		}
	}
	return len(p), nil
}

// splitJSONBytes takes input bytes which may contain one or more JSON objects, or
// plain text, or a combination of both. This handles an edge case where multiple JSON
// logs are sent back-to-back with no delay, which occasionally causes them to be combined.
// E.g. `{"msg": "A"}\n{"msg": "B"}\n` would cause "B" to be dropped by the backend.
// Runs in O(n) time complexity.
func splitJSONBytes(input []byte) [][]byte {
	trimmed := bytes.TrimSpace(input)
	if len(trimmed) <= 0 {
		return nil
	}

	var out [][]byte
	n := len(trimmed)
	offset := 0

	for offset < n {
		// find the next “{” which might start JSON
		idx := bytes.IndexByte(trimmed[offset:], '{')
		if idx < 0 {
			// no more JSON: emit the rest as one plain-text chunk
			tail := bytes.TrimSpace(trimmed[offset:])
			if len(tail) > 0 {
				out = append(out, tail)
			}
			break
		}

		start := offset + idx
		// emit any plain-text before that “{”
		if start > offset {
			prefix := bytes.TrimSpace(trimmed[offset:start])
			if len(prefix) > 0 {
				out = append(out, prefix)
			}
		}

		// try to decode a complete JSON value
		dec := json.NewDecoder(bytes.NewReader(trimmed[start:]))
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			// incomplete JSON: emit the rest as plain-text
			tail := bytes.TrimSpace(trimmed[start:])
			if len(tail) > 0 {
				out = append(out, tail)
			}
			break
		}

		// got one JSON object
		out = append(out, raw)
		// advance past it
		offset = start + int(dec.InputOffset())
	}

	return out
}

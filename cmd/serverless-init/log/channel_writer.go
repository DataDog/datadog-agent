// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"bytes"
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
	payload := cw.Buffer.Bytes()
	cw.Buffer.Reset()

	parts := splitJSONBytes(payload)

	// Send each message as a separate log entry
	for _, msg := range parts {
		trimmed := bytes.TrimSpace(msg)
		if len(trimmed) == 0 {
			continue
		}

		channelMessage := &logConfig.ChannelMessage{
			Content: trimmed,
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
// plain text. This handles an edge case where multiple JSON logs are sent back-to-back
// with no delay, which occasionally causes them to be combined into one message.
// E.g. `{"msg": "A"}\n{"msg": "B"}\n` would cause "B" to be dropped by the backend.
// O(n) time complexity for JSON, O(1) for plaintext.
func splitJSONBytes(input []byte) [][]byte {
	trimmed := bytes.TrimSpace(input)
	if len(trimmed) == 0 {
		return nil
	}

	// if it doesn’t even start like JSON, just return the whole thing
	if trimmed[0] != '{' {
		return [][]byte{trimmed}
	}

	var (
		out      [][]byte
		depth    int
		inString bool
		escape   bool
		startIdx int
		lastEnd  int
	)

	for i, c := range trimmed {
		if escape {
			// the char after '\' is ignored for structure
			escape = false
			continue
		}
		switch c {
		case '\\':
			escape = true
		case '"':
			inString = !inString
		default:
			if !inString {
				if c == '{' {
					if depth == 0 {
						startIdx = i
					}
					depth++
				} else if c == '}' {
					depth--
					if depth == 0 {
						// finished a full object
						out = append(out, trimmed[startIdx:i+1])
						lastEnd = i + 1
					}
				}
			}
		}
	}

	// anything left after the last JSON object
	if lastEnd < len(trimmed) {
		tail := bytes.TrimSpace(trimmed[lastEnd:])
		if len(tail) > 0 {
			out = append(out, tail)
		}
	}

	// if we never saw any complete JSON, fall back
	if len(out) == 0 {
		return [][]byte{trimmed}
	}
	return out
}

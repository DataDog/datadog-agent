// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"strings"

	logConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ChannelWriter is a buffered writer that sends log messages to a channel
// to be sent to our intake.
type ChannelWriter struct {
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
// log messages to the channel.
func (cw *ChannelWriter) Write(p []byte) (n int, err error) {
	// Split any combined JSON messages
	messages := splitJsonMessages(string(p))

	// Send each message as a separate log entry
	for _, msg := range messages {
		channelMessage := &logConfig.ChannelMessage{
			Content: []byte(strings.TrimSpace(msg)),
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

// splitJsonMessages takes an input string which may contain one or more JSON objects,
// or plain text. This handles an edge case where multiple JSON logs are sent back-to-back
// with no delay, which occasionally causes them to be combined into one message.
// E.g. `{"msg": "A"}\n{"msg": "B"}\n` would cause "B" to be dropped by the backend.
// O(n) time complexity for JSON, O(1) for plaintext.
func splitJsonMessages(input string) []string {
	trimmed := strings.TrimSpace(input)
	if len(trimmed) <= 0 {
		return []string{}
	}

	if trimmed[0] != '{' {
		return []string{input}
	}

	var out []string
	var buf strings.Builder
	depth := 0
	inString := false
	escape := false

	for i := 0; i < len(input); i++ {
		c := input[i]
		buf.WriteByte(c)

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
				switch c {
				case '{':
					depth++
				case '}':
					depth--
					if depth == 0 {
						// finished a full object
						out = append(out, strings.TrimSpace(buf.String()))
						buf.Reset()
					}
				}
			}
		}
	}

	// anything left (e.g. trailing whitespace or a non-JSON tail)
	if rest := strings.TrimSpace(buf.String()); rest != "" {
		out = append(out, rest)
	}

	if len(out) == 0 {
		return []string{input}
	}
	return out
}

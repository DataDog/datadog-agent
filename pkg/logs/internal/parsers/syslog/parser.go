// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// parser implements parsers.Parser for syslog-formatted input.
// It converts each newline-framed line into a StateStructured message,
// preserving all syslog metadata in a SyslogStructuredContent.
//
// PRI detection is automatic: if a line starts with '<', it is parsed as a
// network-format syslog message (RFC 5424 or BSD with PRI). Otherwise it is
// parsed as a plain BSD line without PRI (e.g., traditional /var/log/syslog).
type parser struct {
	debugRender bool
}

// NewParser returns a parsers.Parser for syslog-formatted input.
// PRI headers are auto-detected per line. CEF/LEEF headers in the message
// body are always detected and extracted into structured SIEM fields.
//
// When debugRender is true, Render() on the resulting structured content
// produces a JSON envelope with syslog/siem keys instead of the raw log
// line. This is controlled by the debug_attr_parsing config option.
func NewParser(debugRender bool) parsers.Parser {
	return &parser{debugRender: debugRender}
}

// Parse implements parsers.Parser. It parses the unstructured line content
// and returns a new StateStructured message with syslog metadata.
//
// Unlike most parsers, Parse always returns a valid *message.Message even when
// err != nil. On error, the structured message contains the raw content as its
// "message" field and best-effort syslog metadata. Callers MUST NOT discard
// the result on error — the message is intentionally usable.
func (p *parser) Parse(msg *message.Message) (*message.Message, error) {
	var parsed SyslogMessage
	var err error

	content := msg.GetContent()
	if len(content) > 0 && content[0] == '<' {
		parsed, err = Parse(content)
	} else {
		parsed, err = ParseBSDLine(content)
	}

	if err != nil {
		parsed.Msg = content
	}

	sc := NewSyslogStructuredContent(parsed)
	// The full original log line is always the transmitted content; parsed
	// syslog/CEF fields are exposed alongside it (via the structured envelope
	// when debug rendering is enabled), never as a replacement for it.
	sc.msg = string(content)
	sc.debugRender = p.debugRender
	if err != nil {
		// Syslog parsing failed: any CEF/LEEF "detected" inside a malformed
		// fragment must not be trusted.
		sc.siem = nil
	}

	structured := message.NewStructuredMessage(
		sc,
		msg.Origin,
		SeverityToStatus(parsed.Pri),
		msg.IngestionTimestamp,
	)
	structured.RawDataLen = msg.RawDataLen
	structured.ParsingExtra = msg.ParsingExtra
	structured.ParsingExtra.Timestamp = parsed.Timestamp

	return structured, err
}

// SupportsPartialLine implements parsers.Parser. Syslog lines are always
// complete (one message per line), so partial line support is not needed.
func (p *parser) SupportsPartialLine() bool {
	return false
}

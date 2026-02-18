// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// syslogFileParser implements parsers.Parser for syslog-formatted log files.
// It converts each newline-framed line into a StateStructured message,
// preserving all syslog metadata in a BasicStructuredContent.
//
// PRI detection is automatic: if a line starts with '<', it is parsed as a
// network-format syslog message (RFC 5424 or BSD with PRI). Otherwise it is
// parsed as a plain BSD line without PRI (e.g., traditional /var/log/syslog).
type syslogFileParser struct{}

// NewFileParser returns a parsers.Parser for syslog-formatted log files.
// PRI headers are auto-detected per line; no configuration is needed.
func NewFileParser() parsers.Parser {
	return &syslogFileParser{}
}

// Parse implements parsers.Parser. It parses the unstructured line content
// and returns a new StateStructured message with syslog metadata.
func (p *syslogFileParser) Parse(msg *message.Message) (*message.Message, error) {
	var parsed SyslogMessage
	var err error

	content := msg.GetContent()
	if len(content) > 0 && content[0] == '<' {
		parsed, err = Parse(content)
	} else {
		parsed, err = ParseBSDLine(content)
	}

	// Build structured content identical to the TCP tailer's output
	sc := &message.BasicStructuredContent{
		Data: map[string]interface{}{
			"message": string(parsed.Msg),
			"syslog":  BuildSyslogFields(parsed),
		},
	}

	// Create a new StateStructured message, preserving the origin from
	// the file tailer (which carries source, service, tags, file offset).
	structured := message.NewStructuredMessage(
		sc,
		msg.Origin,
		SeverityToStatus(parsed.Pri),
		time.Now().UnixNano(),
	)
	structured.RawDataLen = msg.RawDataLen
	structured.ParsingExtra = msg.ParsingExtra
	structured.ParsingExtra.Timestamp = parsed.Timestamp

	if parsed.AppName != "" && parsed.AppName != "-" {
		structured.ParsingExtra.SourceOverride = parsed.AppName
		structured.ParsingExtra.ServiceOverride = parsed.AppName
	}

	return structured, err
}

// SupportsPartialLine implements parsers.Parser. Syslog lines are always
// complete (one message per line), so partial line support is not needed.
func (p *syslogFileParser) SupportsPartialLine() bool {
	return false
}

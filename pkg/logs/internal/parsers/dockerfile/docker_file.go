// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dockerfile implements a Parser for the JSON-per-line format found in
// Docker logfiles.
package dockerfile

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// New returns a new parser which will parse raw JSON lines as found in docker log files.
//
// The parser handles Docker's JSON log format where each line represents output
// from a container.  A trailing newline (\n) indicates a complete line and is
// stripped from the content.  The absence of a trailing newline indicates a
// partial line (e.g., a prompt waiting for input).
//
// Examples:
//
//	`{"log":"a message\n","stream":"stderr","time":"2019-06-06T16:35:55.930852911Z"}`
//	returns:
//	    Content: []byte("a message"),  // newline stripped
//	    Status: "error",
//	    IsPartial: false,              // complete line
//
//	`{"log":"a prompt: ","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}`
//	returns:
//	    Content: []byte("a prompt: "), // no newline to strip
//	    Status: "info",
//	    IsPartial: true,               // partial line
//
// Note: Only the final newline is stripped. Multiple newlines (e.g., "\n\n")
// represent content with empty lines, so "\n\n" becomes "\n" after parsing.
func New() parsers.Parser {
	return &dockerFileFormat{}
}

type logLine struct {
	Log    string
	Stream string
	Time   string
}

type dockerFileFormat struct{}

// Parse implements Parser#Parse
func (p *dockerFileFormat) Parse(msg *message.Message) (*message.Message, error) {
	var log *logLine
	err := json.Unmarshal(msg.GetContent(), &log)
	if err != nil {
		msg.Status = message.StatusInfo
		return msg, fmt.Errorf("cannot parse docker message, invalid JSON: %v", err)
	}

	// Check if log is nil (e.g., when input is the JSON literal null)
	if log == nil {
		msg.Status = message.StatusInfo
		return msg, fmt.Errorf("cannot parse docker message, invalid format: got null")
	}

	var status string
	switch log.Stream {
	case "stderr":
		status = message.StatusError
	case "stdout":
		status = message.StatusInfo
	default:
		status = ""
	}

	content := []byte(log.Log)
	length := len(content)
	partial := false
	if length > 0 {
		if log.Log[length-1] == '\n' {
			content = content[:length-1]
		} else {
			partial = true
		}
	}
	msg.SetContent(content)
	msg.Status = status
	msg.ParsingExtra.IsPartial = partial
	msg.ParsingExtra.Timestamp = log.Time
	return msg, nil
}

// SupportsPartialLine implements Parser#SupportsPartialLine
func (p *dockerFileFormat) SupportsPartialLine() bool {
	return true
}

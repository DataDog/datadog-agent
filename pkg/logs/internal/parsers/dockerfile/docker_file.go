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
// For example:
//
//	`{"log":"a message","stream":"stderr","time":"2019-06-06T16:35:55.930852911Z"}`
//
// returns:
//
//	parsers.Message {
//	    Content: []byte("a message"),
//	    Status: "error",
//	    Timestamp: "2019-06-06T16:35:55.930852911Z",
//	    IsPartial: false,
//	}
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

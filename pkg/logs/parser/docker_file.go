// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parser

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// DockerFileFormat parses a raw JSON lines as found in docker log files, or
// returns an error if it failed.
// For example:
// `{"log":"a message","stream":"stderr","time":"2019-06-06T16:35:55.930852911Z"}`
// returns:
// `"a message", "error", "2019-06-06T16:35:55.930852911Z", false, nil`
var DockerFileFormat Parser = &dockerFileFormat{}

type logLine struct {
	Log    string
	Stream string
	Time   string
}

type dockerFileFormat struct{}

// Parse implements Parser#Parse
func (p *dockerFileFormat) Parse(data []byte) ([]byte, string, string, bool, error) {
	var log *logLine
	err := json.Unmarshal(data, &log)
	if err != nil {
		return data, message.StatusInfo, "", false, fmt.Errorf("cannot parse docker message, invalid JSON: %v", err)
	}

	var status string
	switch log.Stream {
	case stderr:
		status = message.StatusError
	case stdout:
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
	return content, status, log.Time, partial, nil
}

// SupportsPartialLine implements Parser#SupportsPartialLine
func (p *dockerFileFormat) SupportsPartialLine() bool {
	return true
}

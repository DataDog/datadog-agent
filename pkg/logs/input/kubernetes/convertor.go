// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package kubernetes

import (
	"bytes"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	iParser "github.com/DataDog/datadog-agent/pkg/logs/parser"
	"regexp"
)

type Convertor struct {
	iParser.Convertor
}

func (c *Convertor) Convert(msg []byte, defaultPrefix iParser.Prefix) *iParser.Line {
	components := bytes.SplitN(msg, delimiter, numberOfComponents)

	if len(components) < numberOfComponents-1 {
		// take this msg as partial log splitted by upstream (line generator).
		return &iParser.Line{
			Prefix:  defaultPrefix,
			Content: msg,
			Size:    len(msg),
		}
	}
	timestamp := string(components[0])
	status := string(components[1])
	flag := string(components[2])
	if !c.validate(timestamp, status, flag) {
		// take this msg as partial log splitted by upstream (line generator).
		return &iParser.Line{
			Prefix:  defaultPrefix,
			Content: msg,
			Size:    len(msg),
		}
	}

	if len(components) > numberOfComponents-1 {
		return &iParser.Line{
			Prefix: iParser.Prefix{
				Status:    standardStatus(status),
				Timestamp: timestamp,
			},
			Content: components[3],
			Size:    len(components[3]),
		}
	}
	return nil
}

// 2019-05-29T13:27:27.482052544Z
var timestampMatcher, _ = regexp.Compile("\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}\\.\\d{9}Z")
var statusMatcher, _ = regexp.Compile("std(out|err)")
var flagMatcher, _ = regexp.Compile("F|P")

func (c *Convertor) validate(timestamp string, status string, flag string) bool {
	return timestampMatcher.MatchString(timestamp) &&
		statusMatcher.MatchString(status) &&
		flagMatcher.MatchString(flag)
}

// getStatus returns the status of the message based on
// the value of the STREAM_TYPE field in the header,
// returns the status INFO by default
func standardStatus(streamType string) string {
	switch streamType {
	case stdout:
		return message.StatusInfo
	case stderr:
		return message.StatusError
	default:
		return message.StatusInfo
	}
}

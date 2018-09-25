// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package parser

import (
	"bytes"
	"errors"
)

const (
	// Containerd log partial flags
	logTagPartial = 'P'
	logTagFull    = 'F'
	// Containerd log stream type
	stdout = "stdout"
	stderr = "stderr"
)

// ContainerdFileParser parses containerd file logs
type ContainerdFileParser struct{}

// NewContainerdFileParser returns a new ContainerdFileParser
func NewContainerdFileParser() *ContainerdFileParser {
	return &ContainerdFileParser{}
}

// Parse parse log lines of containerd
// These line have the following format
// Timestamp ouputchannel partial_flag msg
// Example:
// 2018-09-20T11:54:11.753589172Z stdout F This is my message
func (p *ContainerdFileParser) Parse(msg []byte) (ParsedLine, error) {
	// timestamp goes till first space
	endOfTimestampIdx := bytes.Index(msg, []byte{' '})
	if endOfTimestampIdx == -1 {
		// Nothing after the timestamp: ERROR
		return ParsedLine{}, errors.New("can't parse containerd message")
	}

	endOfLogStreamTypeIdx := bytes.Index(msg[endOfTimestampIdx+1:], []byte{' '})
	if endOfLogStreamTypeIdx == -1 {
		// Nothing after the output: ERROR
		return ParsedLine{}, errors.New("can't parse containerd message")
	}
	endOfLogStreamTypeIdx += endOfTimestampIdx + 1
	severity := getContainerdSeverity(msg[endOfTimestampIdx+1 : endOfLogStreamTypeIdx])

	endOfPartialFlagIdx := bytes.Index(msg[endOfLogStreamTypeIdx+1:], []byte{' '})
	if endOfPartialFlagIdx == -1 {
		// Nothing after the PartialFlag: empty message
		return ParsedLine{Severity: severity}, nil
	}
	endOfPartialFlagIdx += endOfLogStreamTypeIdx + 1
	if msg[endOfPartialFlagIdx-1] != byte(logTagFull) && msg[endOfPartialFlagIdx-1] != byte(logTagPartial) {
		return ParsedLine{Severity: severity}, errors.New("can't parse containerd message")
	}

	return ParsedLine{
		Content:  msg[endOfPartialFlagIdx+1:],
		Severity: severity,
	}, nil
}

// Unwrap remove the header of log lines of containerd
func (p *ContainerdFileParser) Unwrap(line []byte) ([]byte, error) {
	// timestamp goes till first space
	endOfTimestampIdx := bytes.Index(line, []byte{' '})
	if endOfTimestampIdx == -1 {
		// Nothing after the timestamp: ERROR
		return nil, errors.New("can't parse containerd message")
	}

	endOfLogStreamTypeIdx := bytes.Index(line[endOfTimestampIdx+1:], []byte{' '})
	if endOfLogStreamTypeIdx == -1 {
		// Nothing after the output: ERROR
		return nil, errors.New("can't parse containerd message")
	}
	endOfLogStreamTypeIdx += endOfTimestampIdx + 1

	endOfPartialFlagIdx := bytes.Index(line[endOfLogStreamTypeIdx+1:], []byte{' '})
	if endOfPartialFlagIdx == -1 {
		// Nothing after the PartialFlag: empty message
		return []byte(nil), nil
	}
	endOfPartialFlagIdx += endOfLogStreamTypeIdx + 1
	if line[endOfPartialFlagIdx-1] != byte(logTagFull) && line[endOfPartialFlagIdx-1] != byte(logTagPartial) {
		return nil, errors.New("can't parse containerd message")
	}

	return line[endOfPartialFlagIdx+1:], nil
}

// getContainerdSeverity returns the severity of the message based on the value of the
// STREAM_TYPE field in the header
func getContainerdSeverity(severity []byte) string {
	switch string(severity) {
	case stdout:
		return StatusInfo
	case stderr:
		return StatusError
	default:
		return ""
	}
}

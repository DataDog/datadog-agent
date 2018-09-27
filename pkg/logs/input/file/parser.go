// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package file

import (
	"bytes"
	"errors"

	logParser "github.com/DataDog/datadog-agent/pkg/logs/parser"
	"github.com/DataDog/datadog-agent/pkg/logs/severity"
)

const (
	// Containerd log partial flags
	logTagPartial = 'P'
	logTagFull    = 'F'
	// Containerd log stream type
	stdout = "stdout"
	stderr = "stderr"
)

// containerdFileParser parses containerd file logs
var containerdFileParser *parser

type parser struct {
	logParser.Parser
}

// Parse parse log lines of containerd
// These line have the following format
// Timestamp ouputchannel partial_flag msg
// Example:
// 2018-09-20T11:54:11.753589172Z stdout F This is my message
func (p *parser) Parse(msg []byte) (logParser.ParsedLine, error) {
	// timestamp goes till first space
	timestamp := bytes.Index(msg, []byte{' '})
	if timestamp == -1 {
		// Nothing after the timestamp: ERROR
		return logParser.ParsedLine{}, errors.New("can't parse containerd message, no whitespace found after timestamp")
	}

	streamType := bytes.Index(msg[timestamp+1:], []byte{' '})
	if streamType == -1 {
		// Nothing after the output: ERROR
		return logParser.ParsedLine{}, errors.New("can't parse containerd message, no whitespace found after stream type")
	}
	streamType += timestamp + 1
	severity := getContainerdSeverity(msg[timestamp+1 : streamType])

	partial := bytes.Index(msg[streamType+1:], []byte{' '})
	if partial == -1 {
		// Nothing after the PartialFlag: empty message
		return logParser.ParsedLine{Severity: severity}, nil
	}
	partial += streamType + 1
	if msg[partial-1] != byte(logTagFull) && msg[partial-1] != byte(logTagPartial) {
		return logParser.ParsedLine{Severity: severity}, errors.New("can't parse containerd message, no whitespace found after partial flag")
	}

	return logParser.ParsedLine{
		Content:  msg[partial+1:],
		Severity: severity,
	}, nil
}

// Unwrap remove the header of log lines of containerd
func (p *parser) Unwrap(line []byte) ([]byte, error) {
	// timestamp goes till first space
	timestamp := bytes.Index(line, []byte{' '})
	if timestamp == -1 {
		// Nothing after the timestamp: ERROR
		return nil, errors.New("can't parse containerd message")
	}

	streamType := bytes.Index(line[timestamp+1:], []byte{' '})
	if streamType == -1 {
		// Nothing after the output: ERROR
		return nil, errors.New("can't parse containerd message")
	}
	streamType += timestamp + 1

	partial := bytes.Index(line[streamType+1:], []byte{' '})
	if partial == -1 {
		// Nothing after the PartialFlag: empty message
		return []byte(nil), nil
	}
	partial += streamType + 1
	if line[partial-1] != byte(logTagFull) && line[partial-1] != byte(logTagPartial) {
		return nil, errors.New("can't parse containerd message")
	}

	return line[partial+1:], nil
}

// getContainerdSeverity returns the severity of the message based on the value of the
// STREAM_TYPE field in the header
func getContainerdSeverity(logStream []byte) string {
	switch string(logStream) {
	case stdout:
		return severity.StatusInfo
	case stderr:
		return severity.StatusError
	default:
		return ""
	}
}

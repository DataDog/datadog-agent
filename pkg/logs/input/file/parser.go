// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package file

import (
	"bytes"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	logParser "github.com/DataDog/datadog-agent/pkg/logs/parser"
)

const (
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
func (p *parser) Parse(msg []byte) (*message.Message, error) {
	parsedMsg, err := parseMsg(msg)
	if err != nil {
		return nil, err
	}
	severity := getContainerdStatus(parsedMsg[1])
	return &message.Message{
		Content: parsedMsg[3],
		Status:  severity,
	}, nil
}

// Unwrap remove the header of log lines of containerd
func (p *parser) Unwrap(line []byte) ([]byte, error) {
	parsedMsg, err := parseMsg(line)
	if err != nil {
		return nil, err
	}
	return parsedMsg[3], nil
}

// getContainerdStatus returns the severity of the message based on the value of the
// STREAM_TYPE field in the header
func getContainerdStatus(logStream []byte) string {
	switch string(logStream) {
	case stdout:
		return message.StatusInfo
	case stderr:
		return message.StatusError
	default:
		return ""
	}
}

func parseMsg(msg []byte) ([][]byte, error) {
	parsedMsg := bytes.SplitN(msg, []byte{' '}, 4)

	if len(parsedMsg) < 3 {
		return nil, errors.New("can't parse containerd message")
	}

	// Empty message
	if len(parsedMsg) == 3 {
		parsedMsg = append(parsedMsg, []byte(nil))
	}

	return parsedMsg, nil
}

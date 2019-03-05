// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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
	components, err := parse(msg)
	if err != nil {
		return message.NewMessage(msg, nil, message.StatusInfo), err
	}
	status := getContainerdStatus(components[1])

	parsedMsg := message.NewMessage(components[3], nil, status)
	parsedMsg.Timestamp = string(components[0])
	return parsedMsg, nil
}

// Unwrap remove the header of log lines of containerd
func (p *parser) Unwrap(line []byte) ([]byte, error) {
	components, err := parse(line)
	if err != nil {
		return line, err
	}
	return components[3], nil
}

// getContainerdStatus returns the status of the message based on the value of the
// STREAM_TYPE field in the header. It returns the status INFO by default
func getContainerdStatus(streamType []byte) string {
	switch string(streamType) {
	case stdout:
		return message.StatusInfo
	case stderr:
		return message.StatusError
	default:
		return message.StatusInfo
	}
}

func parse(msg []byte) ([][]byte, error) {
	components := bytes.SplitN(msg, []byte{' '}, 4)

	if len(components) < 3 {
		return nil, errors.New("can't parse containerd message")
	}

	// Empty message
	if len(components) == 3 {
		components = append(components, []byte(nil))
	}

	return components, nil
}

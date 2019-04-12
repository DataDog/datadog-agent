// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package kubernetes

import (
	"bytes"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	lineParser "github.com/DataDog/datadog-agent/pkg/logs/parser"
)

const (
	// Kubernetes log stream types
	stdout = "stdout"
	stderr = "stderr"
)

var (
	// log line timestamp/stream/flag/content delimiter
	delimiter = []byte{' '}
)

// Parser parses Kubernetes log lines
var Parser *parser

type parser struct {
	lineParser.Parser
}

// Parse parses a Kubernetes log line.
// Kubernetes log lines follow this pattern '<timestamp> <stream> <flag> <content>',
// see https://github.com/kubernetes/kubernetes/blob/master/pkg/kubelet/kuberuntime/logs/logs.go
// Example:
// 2018-09-20T11:54:11.753589172Z stdout F This is my message
func (p *parser) Parse(msg []byte) (*message.Message, error) {
	components, err := splitLine(msg)
	if err != nil {
		return message.NewMessage(msg, nil, message.StatusInfo), err
	}
	status := getStatus(components[1])

	parsedMsg := message.NewMessage(components[3], nil, status)
	parsedMsg.Timestamp = string(components[0])
	return parsedMsg, nil
}

// Unwrap removes the header of the log line.
func (p *parser) Unwrap(line []byte) ([]byte, error) {
	components, err := splitLine(line)
	if err != nil {
		return line, err
	}
	return components[3], nil
}

// getStatus returns the status of the message based on
// the value of the STREAM_TYPE field in the header,
// returns the status INFO by default
func getStatus(streamType []byte) string {
	switch string(streamType) {
	case stdout:
		return message.StatusInfo
	case stderr:
		return message.StatusError
	default:
		return message.StatusInfo
	}
}

// splitLine splits the log line into the four components using ,
// returns an error if it failed.
func splitLine(msg []byte) ([][]byte, error) {
	components := bytes.SplitN(msg, delimiter, 4)

	if len(components) < 3 {
		return nil, errors.New("can't parse the log line")
	}

	// Empty message
	if len(components) == 3 {
		components = append(components, []byte(nil))
	}

	return components, nil
}

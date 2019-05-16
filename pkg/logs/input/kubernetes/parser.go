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
	content, status, timestamp, _, err := parse(msg)
	return message.NewPartialMessage(content, status, timestamp), err
}

// Unwrap removes the header of the log line
// and return the log and timestamp
func (p *parser) Unwrap(line []byte) ([]byte, string, error) {
	content, _, timestamp, _, err := parse(line)
	return content, timestamp, err
}

func parse(msg []byte) ([]byte, string, string, string, error) {
	var status = message.StatusInfo
	var flag string
	var timestamp string
	components := bytes.SplitN(msg, delimiter, 4)
	if len(components) < 3 {
		return msg, status, timestamp, flag, errors.New("cannot parse the log line")
	}
	var content []byte
	if len(components) > 3 {
		content = components[3]
	}
	status = getStatus(components[1])
	timestamp = string(components[0])
	flag = string(components[2])
	return content, status, timestamp, flag, nil
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

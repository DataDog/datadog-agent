// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package kubernetes

import (
	"bytes"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	lineParser "github.com/DataDog/datadog-agent/pkg/logs/parser"
)

const (
	// Kubernetes log stream types
	stdout             = "stdout"
	stderr             = "stderr"
	numberOfComponents = 4 // timestamp stream flag message
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
func (p *parser) Parse(msg []byte) ([]byte, string, string, bool, error) {
	content, status, timestamp, flag, err := parse(msg)
	return content, status, timestamp, isPartial(flag), err
}

func (p *parser) SupportsPartialLine() bool {
	return true
}

func parse(msg []byte) ([]byte, string, string, string, error) {
	var status = message.StatusInfo
	var flag string
	var timestamp string
	components := bytes.SplitN(msg, delimiter, numberOfComponents)
	if len(components) < numberOfComponents-1 {
		return msg, status, timestamp, flag, errors.New("cannot parse the log line")
	}
	var content []byte
	if len(components) > numberOfComponents-1 {
		content = components[3]
	}
	status = getStatus(components[1])
	timestamp = string(components[0])
	flag = string(components[2])
	return content, status, timestamp, flag, nil
}

func isPartial(flag string) bool {
	if flag == "P" {
		return true
	}
	return false
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

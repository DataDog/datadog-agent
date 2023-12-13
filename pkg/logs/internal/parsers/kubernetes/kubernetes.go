// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"bytes"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

var (
	// one-space delimiter, created outside of a hot loop
	spaceByte = []byte{' '}
)

// New creates a new parser that parses Kubernetes-formatted log lines.
//
// Kubernetes log lines follow the pattern '<timestamp> <stream> <flag> <content>'; see
// https://github.com/kubernetes/kubernetes/blob/master/pkg/kubelet/kuberuntime/logs/logs.go.
//
// For example: `2018-09-20T11:54:11.753589172Z stdout F This is my message`
func New() parsers.Parser {
	return &kubernetesFormat{}
}

type kubernetesFormat struct{}

// Parse implements Parser#Parse
func (p *kubernetesFormat) Parse(msg *message.Message) (*message.Message, error) {
	return parseKubernetes(msg)
}

// SupportsPartialLine implements Parser#SupportsPartialLine
func (p *kubernetesFormat) SupportsPartialLine() bool {
	return true
}

func parseKubernetes(msg *message.Message) (*message.Message, error) {
	var status = message.StatusInfo
	var flag string
	var timestamp string
	// split '<timestamp> <stream> <flag> <content>' into its components
	components := bytes.SplitN(msg.GetContent(), spaceByte, 4)
	if len(components) < 3 {
		return message.NewMessage(msg.GetContent(), nil, status, 0), errors.New("cannot parse the log line")
	}
	var content []byte
	if len(components) > 3 {
		content = components[3]
	}
	timestamp = string(components[0])
	status = getStatus(components[1])
	flag = string(components[2])

	msg.SetContent(content)
	msg.Status = status
	msg.ParsingExtra = message.ParsingExtra{
		IsPartial: isPartial(flag),
		Timestamp: timestamp,
	}

	return msg, nil
}

func isPartial(flag string) bool {
	return flag == "P"
}

// getStatus returns the status of the message based on
// the value of the STREAM_TYPE field in the header,
// returns the status INFO by default
func getStatus(streamType []byte) string {
	switch string(streamType) {
	case "stdout":
		return message.StatusInfo
	case "stderr":
		return message.StatusError
	default:
		return message.StatusInfo
	}
}

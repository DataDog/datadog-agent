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

var (
	// 2019-05-29T13:27:27.482052544Z
	timestampMatcher = regexp.MustCompile("\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}\\.\\d{9}Z")
	statusMatcher    = regexp.MustCompile("std(out|err)")
	flagMatcher      = regexp.MustCompile("F|P")
)

const (
	numOfComponents  = 4
	indexOfTimestamp = 0
	indexOfStatus    = 1
	indexOfFlag      = 2
	indexOfContent   = 3
)

// Convertor for converting kubernetes log line to struct Line.
type Convertor struct {
	iParser.Convertor
}

// Convert validates and converts kubernetes log from byte array to struct Line.
// Kubernetes log lines follow this pattern '<timestamp> <stream> <flag> <content>',
// see https://github.com/kubernetes/kubernetes/blob/master/pkg/kubelet/kuberuntime/logs/logs.go
// Example:
// 2018-09-20T11:54:11.753589172Z stdout F This is my message
func (c *Convertor) Convert(msg []byte, defaultPrefix iParser.Prefix) *iParser.Line {
	components := bytes.SplitN(msg, delimiter, numOfComponents)
	if !c.validate(components) {
		return &iParser.Line{
			Prefix:  defaultPrefix,
			Content: msg,
			Size:    len(msg),
		}
	}
	// empty message:
	// 2018-09-20T11:54:11.753589143Z stdout F\n
	// 2018-09-20T11:54:11.753589143Z stdout F \n
	if len(components) < numOfComponents || len(components[numOfComponents-1]) <= 0 {
		return nil
	}

	timestamp := string(components[indexOfTimestamp])
	status := string(components[indexOfStatus])
	flag := string(components[indexOfFlag])
	return &iParser.Line{
		Prefix: iParser.Prefix{
			Status:    standardStatus(status),
			Timestamp: timestamp,
			Flag:      flag,
		},
		Content: components[indexOfContent],
		Size:    len(components[indexOfContent]),
	}
}

func (c *Convertor) validate(components [][]byte) bool {
	return len(components) >= numOfComponents-1 &&
		timestampMatcher.MatchString(string(components[indexOfTimestamp])) &&
		statusMatcher.MatchString(string(components[indexOfStatus])) &&
		flagMatcher.MatchString(string(components[indexOfFlag]))
}

// standardStatus returns the standard status of the message based on
// the value of the STREAM_TYPE field in the prefix,
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

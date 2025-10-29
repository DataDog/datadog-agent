// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config provides log configuration structures and utilities
package config

import "time"

// ChannelMessage represents a log line sent to datadog, with its metadata
type ChannelMessage struct {
	Content []byte
	// Optional. Must be UTC. If not provided, time.Now().UTC() will be used
	// Used in the Serverless Agent
	Timestamp time.Time
	// Optional.
	// Used in the Serverless Agent
	Lambda  *Lambda
	IsError bool
}

// Lambda is a struct storing information about the Lambda function and function execution.
type Lambda struct {
	ARN          string
	RequestID    string
	FunctionName string
}

// NewChannelMessageFromLambda construts a message with content and with the given timestamp and Lambda metadata
func NewChannelMessageFromLambda(content []byte, utcTime time.Time, ARN, reqID string, isError bool) *ChannelMessage {
	return &ChannelMessage{
		Content:   content,
		Timestamp: utcTime,
		Lambda: &Lambda{
			ARN:       ARN,
			RequestID: reqID,
		},
		IsError: isError,
	}
}

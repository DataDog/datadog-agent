// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// Message represents a log line sent to datadog, with its metadata
type Message struct {
	Content            []byte
	Origin             *Origin
	status             string
	IngestionTimestamp int64
	// Optional. Must be UTC. If not provided, time.Now().UTC() will be used
	// Used in the Serverless Agent
	Timestamp time.Time
	// Optional.
	// Used in the Serverless Agent
	Lambda *Lambda
}

// Lambda is a struct storing information about the Lambda function and function execution.
type Lambda struct {
	ARN       string
	RequestID string
}

// NewMessageWithSource constructs message with content, status and log source.
func NewMessageWithSource(content []byte, status string, source *config.LogSource, ingestionTimestamp int64) *Message {
	return NewMessage(content, NewOrigin(source), status, ingestionTimestamp)
}

// NewMessage constructs message with content, status, origin and the ingestion timestamp.
func NewMessage(content []byte, origin *Origin, status string, ingestionTimestamp int64) *Message {
	return &Message{
		Content:            content,
		Origin:             origin,
		status:             status,
		IngestionTimestamp: ingestionTimestamp,
	}
}

// NewMessageFromLambda construts a message with content, status, origin and with the given timestamp and Lambda metadata
func NewMessageFromLambda(content []byte, origin *Origin, status string, utcTime time.Time, ARN, reqID string, ingestionTimestamp int64) *Message {
	return &Message{
		Content:            content,
		Origin:             origin,
		status:             status,
		IngestionTimestamp: ingestionTimestamp,
		Timestamp:          utcTime,
		Lambda: &Lambda{
			ARN:       ARN,
			RequestID: reqID,
		},
	}
}

// GetStatus gets the status of the message.
// if status is not set, StatusInfo will be returned.
func (m *Message) GetStatus() string {
	if m.status == "" {
		m.status = StatusInfo
	}
	return m.status
}

// GetLatency returns the latency delta from ingestion time until now
func (m *Message) GetLatency() int64 {
	return time.Now().UnixNano() - m.IngestionTimestamp
}

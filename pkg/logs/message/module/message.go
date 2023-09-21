// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"context"
	"time"
)

// Payload represents an encoded collection of messages ready to be sent to the intake
type Payload struct {
	// The slice of sources messages encoded in the payload
	Messages []*Message
	// The encoded bytes to be sent to the intake (sometimes compressed)
	Encoded []byte
	// The content encoding. A header for HTTP, empty for TCP
	Encoding string
	// The size of the unencoded payload
	UnencodedSize int
}

// Message represents a log line sent to datadog, with its metadata
type Message struct {
	Content            []byte
	Origin             *Origin
	Status             string
	IngestionTimestamp int64
	RawDataLen         int
	// Extra information from the parsers
	ParsingExtra
	// Extra information for Serverless Logs messages
	ServerlessExtra
	// Function to retrieve host name
	GetHostnameFunc GetHostnameFunc
}

// ParsingExtra ships extra information parsers want to make available
// to the rest of the pipeline.
// E.g. Timestamp is used by the docker parsers to transmit a tailing offset.
type ParsingExtra struct {
	// Used by docker parsers to transmit an offset.
	Timestamp string
	IsPartial bool
}

// ServerlessExtra ships extra information from logs processing in serverless envs.
type ServerlessExtra struct {
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

// TODO: remove module here if this approach works for util/hostname
// GetHostnameFunc defines the signature for a function to retrieve the hostname for the agent.
type GetHostnameFunc func(ctx context.Context) (string, error)

// GetStatus gets the status of the message.
// if status is not set, StatusInfo will be returned.
func (m *Message) GetStatus() string {
	if m.Status == "" {
		m.Status = StatusInfo
	}
	return m.Status
}

// GetLatency returns the latency delta from ingestion time until now
func (m *Message) GetLatency() int64 {
	return time.Now().UnixNano() - m.IngestionTimestamp
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message/module"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// Payload represents an encoded collection of messages ready to be sent to the intake
type Payload = module.Payload

// Message represents a log line sent to datadog, with its metadata
type Message = module.Message

// ParsingExtra ships extra information parsers want to make available
// to the rest of the pipeline.
// E.g. Timestamp is used by the docker parsers to transmit a tailing offset.
type ParsingExtra = module.ParsingExtra

// ServerlessExtra ships extra information from logs processing in serverless envs.
type ServerlessExtra = module.ServerlessExtra

// Lambda is a struct storing information about the Lambda function and function execution.
type Lambda = module.Lambda

// NewMessageWithSource constructs message with content, status and log source.
func NewMessageWithSource(content []byte, status string, source *sources.LogSource, ingestionTimestamp int64) *Message {
	return NewMessage(content, NewOrigin(source), status, ingestionTimestamp)
}

// TODO: could do hostname getter if hostname migration is too hard
// NewMessage constructs message with content, status, origin and the ingestion timestamp.
func NewMessage(content []byte, origin *module.Origin, status string, ingestionTimestamp int64) *Message {
	return &Message{
		Content:            content,
		Origin:             origin,
		Status:             status,
		IngestionTimestamp: ingestionTimestamp,
	}
}

// NewMessageFromLambda construts a message with content, status, origin and with the given timestamp and Lambda metadata
func NewMessageFromLambda(content []byte, origin *module.Origin, status string, utcTime time.Time, ARN, reqID string, ingestionTimestamp int64) *Message {
	return &Message{
		Content:            content,
		Origin:             origin,
		Status:             status,
		IngestionTimestamp: ingestionTimestamp,
		ServerlessExtra: ServerlessExtra{
			Timestamp: utcTime,
			Lambda: &Lambda{
				ARN:       ARN,
				RequestID: reqID,
			},
		},
	}
}

// GetStatus gets the status of the message.
// if status is not set, StatusInfo will be returned.
var GetStatus = (*module.Message).GetStatus

// GetLatency returns the latency delta from ingestion time until now
var GetLatency = (*module.Message).GetLatency

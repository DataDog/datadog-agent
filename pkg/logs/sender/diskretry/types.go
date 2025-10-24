// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package diskretry provides disk-based persistence for failed log payloads
package diskretry

import (
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// PersistedPayload represents a payload that has been persisted to disk
// It wraps the original message.Payload with retry metadata
type PersistedPayload struct {
	// Original payload data
	MessageMetas  []*message.MessageMetadata `json:"message_metas"`
	Encoded       []byte                     `json:"encoded"`
	Encoding      string                     `json:"encoding"`
	UnencodedSize int                        `json:"unencoded_size"`

	// Retry metadata
	CreatedAt   int64  `json:"created_at"`    // Unix timestamp (seconds)
	RetryCount  int    `json:"retry_count"`   // Number of replay attempts
	LastRetryAt int64  `json:"last_retry_at"` // Unix timestamp (seconds)
	WorkerID    string `json:"worker_id"`     // Worker that wrote this
	FilePath    string `json:"-"`             // Path to file on disk (not serialized)
}

// ToPayload converts a PersistedPayload back to a message.Payload
func (p *PersistedPayload) ToPayload() *message.Payload {
	return &message.Payload{
		MessageMetas:  p.MessageMetas,
		Encoded:       p.Encoded,
		Encoding:      p.Encoding,
		UnencodedSize: p.UnencodedSize,
	}
}

// FromPayload creates a PersistedPayload from a message.Payload
func FromPayload(payload *message.Payload, workerID string) *PersistedPayload {
	now := time.Now().Unix()
	return &PersistedPayload{
		MessageMetas:  payload.MessageMetas,
		Encoded:       payload.Encoded,
		Encoding:      payload.Encoding,
		UnencodedSize: payload.UnencodedSize,
		CreatedAt:     now,
		RetryCount:    0,
		LastRetryAt:   0,
		WorkerID:      workerID,
	}
}

// Marshal serializes the PersistedPayload to JSON
func (p *PersistedPayload) Marshal() ([]byte, error) {
	return json.Marshal(p)
}

// Unmarshal deserializes JSON into a PersistedPayload
func Unmarshal(data []byte) (*PersistedPayload, error) {
	var p PersistedPayload
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Age returns how long ago the payload was created
func (p *PersistedPayload) Age() time.Duration {
	return time.Since(time.Unix(p.CreatedAt, 0))
}

// ShouldRetry checks if the payload should be retried based on age and retry count
func (p *PersistedPayload) ShouldRetry(maxAge time.Duration, maxRetries int) bool {
	if p.Age() > maxAge {
		return false // Too old
	}
	if maxRetries > 0 && p.RetryCount >= maxRetries {
		return false // Too many retries
	}
	return true
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build observer

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConnectionErrorExtractor_Name(t *testing.T) {
	e := &ConnectionErrorExtractor{}
	assert.Equal(t, "connection_error_extractor", e.Name())
}

func TestConnectionErrorExtractor_Process_ConnectionRefused(t *testing.T) {
	e := &ConnectionErrorExtractor{}
	log := &mockLogView{
		content: []byte("Failed to connect: connection refused"),
		tags:    []string{"env:prod", "service:api"},
	}

	result := e.Process(log)

	assert.Len(t, result.Metrics, 1)
	assert.Equal(t, "connection.errors", result.Metrics[0].Name)
	assert.Equal(t, 1.0, result.Metrics[0].Value)
	assert.Equal(t, []string{"env:prod", "service:api"}, result.Metrics[0].Tags)
}

func TestConnectionErrorExtractor_Process_ECONNRESET(t *testing.T) {
	e := &ConnectionErrorExtractor{}
	log := &mockLogView{
		content: []byte("Socket error: ECONNRESET"),
		tags:    []string{"env:staging"},
	}

	result := e.Process(log)

	assert.Len(t, result.Metrics, 1)
	assert.Equal(t, "connection.errors", result.Metrics[0].Name)
	assert.Equal(t, 1.0, result.Metrics[0].Value)
	assert.Equal(t, []string{"env:staging"}, result.Metrics[0].Tags)
}

func TestConnectionErrorExtractor_Process_NoMatch(t *testing.T) {
	e := &ConnectionErrorExtractor{}
	log := &mockLogView{
		content: []byte("Request completed successfully"),
		tags:    []string{"env:test"},
	}

	result := e.Process(log)

	assert.Empty(t, result.Metrics)
}

func TestConnectionErrorExtractor_Process_CaseInsensitive(t *testing.T) {
	e := &ConnectionErrorExtractor{}
	log := &mockLogView{
		content: []byte("Error: Connection Refused by server"),
		tags:    []string{"env:prod"},
	}

	result := e.Process(log)

	assert.Len(t, result.Metrics, 1)
	assert.Equal(t, "connection.errors", result.Metrics[0].Name)
	assert.Equal(t, 1.0, result.Metrics[0].Value)
	assert.Equal(t, []string{"env:prod"}, result.Metrics[0].Tags)
}

func TestConnectionErrorExtractor_Process_TagsCopied(t *testing.T) {
	e := &ConnectionErrorExtractor{}
	inputTags := []string{"env:prod", "service:api", "host:web-1"}
	log := &mockLogView{
		content: []byte("connection timed out after 30s"),
		tags:    inputTags,
	}

	result := e.Process(log)

	assert.Len(t, result.Metrics, 1)
	assert.Equal(t, inputTags, result.Metrics[0].Tags)
}

func TestConnectionErrorExtractor_Process_AllPatterns(t *testing.T) {
	e := &ConnectionErrorExtractor{}

	testCases := []struct {
		name    string
		content string
	}{
		{"connection refused", "connection refused"},
		{"connection reset", "connection reset by peer"},
		{"connection timed out", "connection timed out"},
		{"ECONNREFUSED", "Error: ECONNREFUSED"},
		{"ECONNRESET", "ECONNRESET occurred"},
		{"ETIMEDOUT", "syscall error: ETIMEDOUT"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			log := &mockLogView{
				content: []byte(tc.content),
				tags:    []string{"test:pattern"},
			}

			result := e.Process(log)

			assert.Len(t, result.Metrics, 1, "Expected metric for pattern: %s", tc.name)
			assert.Equal(t, "connection.errors", result.Metrics[0].Name)
			assert.Equal(t, 1.0, result.Metrics[0].Value)
		})
	}
}

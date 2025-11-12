// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package streamlogs_test

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/serverless/streamlogs"
	"github.com/stretchr/testify/require"
)

func newTestMessage(msg string, status string, timestamp time.Time) *message.Message {
	origin := message.NewOrigin(&sources.LogSource{
		Name: "Test Logs",
		Config: &config.LogsConfig{
			Type:    "unit-test",
			Service: "test-service",
			Source:  "test-source",
			Tags:    []string{"tag1:value1", "tag2:value2"},
		},
	})
	m := message.NewMessageFromLambda(
		[]byte(msg),
		origin,
		status,
		timestamp,
		"arn:aws:lambda:us-east-1:123456789012:function:test",
		"request-id",
		0,
	)
	return m
}

func TestFormatterFormat(t *testing.T) {
	tests := []struct {
		name     string
		message  *message.Message
		expected string
	}{
		{
			name:     "nil message",
			message:  nil,
			expected: "",
		},
		{
			name: "valid message",
			message: newTestMessage(
				"test log message",
				"INFO",
				time.Date(2024, time.May, 26, 12, 14, 39, 0, time.UTC),
			),
			expected: "Integration Name: Test Logs | Type: unit-test | Status: INFO | Timestamp: 2024-05-26 12:14:39 +0000 UTC | Service: test-service | Source: test-source | Tags: tag1:value1,tag2:value2 | Message: test log message\n",
		},
		{
			name: "message with unicode",
			message: newTestMessage(
				"ログメッセージ",
				"DEBUG",
				time.Date(2024, time.May, 26, 12, 14, 39, 0, time.UTC),
			),
			expected: "Integration Name: Test Logs | Type: unit-test | Status: DEBUG | Timestamp: 2024-05-26 12:14:39 +0000 UTC | Service: test-service | Source: test-source | Tags: tag1:value1,tag2:value2 | Message: ログメッセージ\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := streamlogs.Formatter{}
			result := formatter.Format(tt.message, "", []byte(tt.message.GetContent()))
			require.Equal(t, tt.expected, result)
		})
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package streamlogs_test package is for testing streamlogs package.
package streamlogs_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/serverless/streamlogs"
	"github.com/stretchr/testify/require"
)

type messageReceiverGetter struct {
	bufferedMessageReceiver *diagnostic.BufferedMessageReceiver
}

func (mrg *messageReceiverGetter) GetMessageReceiver() *diagnostic.BufferedMessageReceiver {
	return mrg.bufferedMessageReceiver
}

func newTestMessage(msg string) *message.Message {
	origin := message.NewOrigin(&sources.LogSource{
		Name: "AWS Logs",
		Config: &config.LogsConfig{
			Type:    "unit-test",
			Service: "test-lambda",
			Source:  "lambda",
			Tags:    []string{"tag1:value1", "tag2:value2"},
		},
	})
	m := message.NewMessageFromLambda(
		[]byte(msg),
		origin,
		"INFO",
		time.Date(
			2024,
			time.May,
			26,
			12,
			14,
			39,
			0,
			time.UTC,
		),
		"arn:aws:lambda:ap-northeast-1:111111111111:function:test-lambda",
		"8b185116-a6d3-41d5-baba-1f430b1f849e",
		0,
	)
	return m
}

func TestRun(t *testing.T) {
	tests := []struct {
		name             string
		logMessages      []string
		diableStreamLogs bool
		wantW            string
	}{
		{
			name:             "disable stream-logs",
			diableStreamLogs: true,
			wantW:            "",
		},
		{
			name:             "stream-logs",
			logMessages:      []string{"log-1", "ログ2"},
			diableStreamLogs: false,
			wantW: `DD_EXTENSION | stream-logs | Integration Name: AWS Logs | Type: unit-test | Status: INFO | Timestamp: 2024-05-26 12:14:39 +0000 UTC | Service: test-lambda | Source: lambda | Tags: tag1:value1,tag2:value2 | Message: log-1
DD_EXTENSION | stream-logs | Integration Name: AWS Logs | Type: unit-test | Status: INFO | Timestamp: 2024-05-26 12:14:39 +0000 UTC | Service: test-lambda | Source: lambda | Tags: tag1:value1,tag2:value2 | Message: ログ2
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DD_SERVERLESS_STREAM_LOGS", "true")
			if tt.diableStreamLogs {
				t.Setenv("DD_SERVERLESS_STREAM_LOGS", "false")
			}
			var buf bytes.Buffer
			ctx, cancel := context.WithCancel(context.Background())
			bmr := diagnostic.NewBufferedMessageReceiver(streamlogs.Formatter{}, nil)
			getter := &messageReceiverGetter{bufferedMessageReceiver: bmr}
			go streamlogs.Run(ctx, getter, &buf)
			time.Sleep(10 * time.Millisecond) // wait for the bmr to be enabled
			for _, msg := range tt.logMessages {
				bmr.HandleMessage(newTestMessage(msg), []byte(msg), "")
			}
			time.Sleep(10 * time.Millisecond) // wait for the stream-logs to process the messages
			cancel()                          // stop stream-logs
			require.Equal(t, tt.wantW, buf.String())
		})
	}
}

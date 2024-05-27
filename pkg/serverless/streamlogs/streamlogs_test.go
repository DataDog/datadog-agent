package streamlogs_test

import (
	"bytes"
	"context"
	"sync"
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
	var bufferMutex sync.Mutex // To pass tests with `-race` flag

	type handleMessageArgs struct {
		message  *message.Message
		rendered []byte
	}
	tests := []struct {
		name                  string
		handleMessageArgsList []handleMessageArgs
		diableStreamLogs      bool
		wantW                 string
	}{
		{
			name:             "disable stream-logs",
			diableStreamLogs: true,
			wantW:            "",
		},
		{
			name: "enable stream-logs",
			handleMessageArgsList: []handleMessageArgs{
				{
					message:  newTestMessage("log-1"),
					rendered: []byte("log-1"),
				},
				{
					message:  newTestMessage("ログ2"),
					rendered: []byte("ログ2"),
				},
				{
					// When logs-agent stops, nil message is sent to stream-logs
					message:  nil,
					rendered: nil,
				},
			},
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

			go func() {
				bufferMutex.Lock()
				defer bufferMutex.Unlock()
				streamlogs.Run(ctx, getter, &buf)
			}()

			time.Sleep(10 * time.Millisecond) // wait for the bmr to be enabled
			for _, args := range tt.handleMessageArgsList {
				bmr.HandleMessage(args.message, args.rendered, "")
			}
			time.Sleep(10 * time.Millisecond) // wait for the stream-logs to process the messages
			cancel()                          // stop stream-logs

			bufferMutex.Lock()
			require.Equal(t, tt.wantW, buf.String())
			bufferMutex.Unlock()
		})
	}
}

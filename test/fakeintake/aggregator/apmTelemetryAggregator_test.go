// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed fixtures/apm_telemetry_bytes
var apmTelemetryData []byte

func TestParseAgentTelemetryLogs(t *testing.T) {
	t.Run("empty JSON object is silently skipped", func(t *testing.T) {
		logs, err := ParseAgentTelemetryLogs(api.Payload{
			Data:     []byte("{}"),
			Encoding: encodingJSON,
		})
		require.NoError(t, err)
		assert.Empty(t, logs)
	})

	t.Run("non-agent-logs request_type is silently skipped", func(t *testing.T) {
		logs, err := ParseAgentTelemetryLogs(api.Payload{
			Data:     []byte(`{"request_type":"agent-metrics","payload":{}}`),
			Encoding: encodingJSON,
		})
		require.NoError(t, err)
		assert.Empty(t, logs)
	})

	t.Run("non-JSON payload (e.g. protobuf) is silently skipped", func(t *testing.T) {
		logs, err := ParseAgentTelemetryLogs(api.Payload{
			Data:     []byte("not valid json"),
			Encoding: encodingJSON,
		})
		require.NoError(t, err)
		assert.Empty(t, logs)
	})

	t.Run("valid agent-logs payload parses all fields and stamps collected time", func(t *testing.T) {
		ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
		logs, err := ParseAgentTelemetryLogs(api.Payload{
			Data:      apmTelemetryData,
			Encoding:  encodingJSON,
			Timestamp: ts,
		})
		require.NoError(t, err)
		require.Len(t, logs, 1)

		l := logs[0]
		assert.Equal(t, "ERROR", l.Level)
		assert.Contains(t, l.StackTrace, "main.main()")
		assert.Equal(t, int64(1234567890), l.TracerTime)
		assert.Equal(t, 3, l.Count)
		assert.False(t, l.IsCrash)
		assert.Empty(t, l.Message)
		assert.Equal(t, ts, l.GetCollectedTime())
	})

	t.Run("multiple logs in one payload are all returned", func(t *testing.T) {
		data := []byte(`{"request_type":"agent-logs","payload":{"logs":[` +
			`{"level":"ERROR","stack_trace":"foo","tracer_time":1,"count":1,"is_crash":false},` +
			`{"level":"ERROR","stack_trace":"bar","tracer_time":2,"count":2,"is_crash":true}` +
			`]}}`)
		logs, err := ParseAgentTelemetryLogs(api.Payload{
			Data:     data,
			Encoding: encodingJSON,
		})
		require.NoError(t, err)
		require.Len(t, logs, 2)
		assert.Equal(t, int64(1), logs[0].TracerTime)
		assert.Equal(t, int64(2), logs[1].TracerTime)
		assert.True(t, logs[1].IsCrash)
	})

	t.Run("GetTags returns empty slice", func(t *testing.T) {
		l := &AgentTelemetryLog{}
		assert.Empty(t, l.GetTags())
	})

	t.Run("name returns agent-errortracking", func(t *testing.T) {
		l := &AgentTelemetryLog{}
		assert.Equal(t, "agent-errortracking", l.name())
	})
}

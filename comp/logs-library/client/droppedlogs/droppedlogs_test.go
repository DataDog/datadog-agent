// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package droppedlogs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

var testTarget = metrics.DropTarget{Host: "intake.example.com", Kind: metrics.DropTargetPrimary}

func meta(cfg *config.LogsConfig, size int) *message.MessageMetadata {
	return &message.MessageMetadata{Origin: message.NewOrigin(sources.NewLogSource("test", cfg)), RawDataLen: size}
}

func TestRecordDroppedPayloadAttributesPerOrigin(t *testing.T) {
	metrics.ResetDroppedLogsForTest()
	t.Cleanup(metrics.ResetDroppedLogsForTest)

	RecordPayload(&message.Payload{MessageMetas: []*message.MessageMetadata{
		meta(&config.LogsConfig{Source: "nginx", Service: "web"}, 100),
		meta(&config.LogsConfig{Source: "nginx", Service: "web"}, 20),
		meta(&config.LogsConfig{Source: "nginx", Service: "api"}, 5),
	}}, metrics.DropReasonAuthRejected, testTarget)

	got := metrics.DroppedLogsSnapshot()
	require.Len(t, got, 2)
	assert.Equal(t, "api", got[0].Service)
	assert.Equal(t, int64(1), got[0].Logs)
	assert.Equal(t, "web", got[1].Service)
	assert.Equal(t, int64(2), got[1].Logs)
	assert.Equal(t, int64(120), got[1].Bytes)
}

// DBM, NDM, network path and the other event-platform producers build messages
// with no Origin. They must never surface as lost logs.
func TestRecordDroppedPayloadSkipsEventPlatform(t *testing.T) {
	metrics.ResetDroppedLogsForTest()
	t.Cleanup(metrics.ResetDroppedLogsForTest)

	RecordPayload(&message.Payload{MessageMetas: []*message.MessageMetadata{
		{RawDataLen: 4096},
		{Origin: &message.Origin{}, RawDataLen: 4096},
		nil,
	}}, metrics.DropReasonAuthRejected, testTarget)
	RecordPayload(nil, metrics.DropReasonAuthRejected, testTarget)

	assert.Empty(t, metrics.DroppedLogsSnapshot())
}

func TestDropOriginFallbacks(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.LogsConfig
		want metrics.DropOrigin
	}{
		{"source and service", &config.LogsConfig{Source: "nginx", Service: "web"}, metrics.DropOrigin{Source: "nginx", Service: "web"}},
		{"service defaults to source", &config.LogsConfig{Source: "nginx"}, metrics.DropOrigin{Source: "nginx", Service: "nginx"}},
		{"integration name", &config.LogsConfig{IntegrationName: "postgres"}, metrics.DropOrigin{Source: "postgres", Service: "postgres"}},
		{"nothing set", &config.LogsConfig{}, metrics.DropOrigin{Source: "unknown", Service: "unknown"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, dropOrigin(message.NewOrigin(sources.NewLogSource("test", tc.cfg))))
		})
	}
}

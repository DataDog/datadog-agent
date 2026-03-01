// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"

	"github.com/DataDog/datadog-go/v5/statsd"
	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestTelemetryMetric builds a flatbuffer TelemetryMetric for testing.
// This constructs the same binary format that the C injector would produce.
func buildTestTelemetryMetric(name string, metricType int8, value float64, tags []string, runtimeID, lang, langVer, injectorVer, externalEnv string) []byte {
	builder := flatbuffers.NewBuilder(512)

	// Create strings (must be created before the table starts).
	nameOffset := builder.CreateString(name)
	runtimeIDOffset := builder.CreateString(runtimeID)
	langOffset := builder.CreateString(lang)
	langVerOffset := builder.CreateString(langVer)
	injectorVerOffset := builder.CreateString(injectorVer)

	var externalEnvOffset flatbuffers.UOffsetT
	if externalEnv != "" {
		externalEnvOffset = builder.CreateString(externalEnv)
	}

	// Create tags vector.
	tagOffsets := make([]flatbuffers.UOffsetT, len(tags))
	for i, tag := range tags {
		tagOffsets[i] = builder.CreateString(tag)
	}
	// Vectors must be created in reverse order.
	builder.StartVector(4, len(tags), 4)
	for i := len(tags) - 1; i >= 0; i-- {
		builder.PrependUOffsetT(tagOffsets[i])
	}
	tagsVector := builder.EndVector(len(tags))

	// Build the TelemetryMetric table.
	// Field order must match the schema vtable offsets:
	//   name=4, type=6, value=8, tags=10, runtime_id=12,
	//   language_name=14, language_version=16, injector_version=18, external_env=20
	builder.StartObject(9) // 9 fields

	builder.PrependUOffsetTSlot(0, flatbuffers.UOffsetT(nameOffset), 0)          // name (offset 4)
	builder.PrependInt8Slot(1, metricType, 0)                                     // type (offset 6)
	builder.PrependFloat64Slot(2, value, 0.0)                                     // value (offset 8)
	builder.PrependUOffsetTSlot(3, flatbuffers.UOffsetT(tagsVector), 0)           // tags (offset 10)
	builder.PrependUOffsetTSlot(4, flatbuffers.UOffsetT(runtimeIDOffset), 0)      // runtime_id (offset 12)
	builder.PrependUOffsetTSlot(5, flatbuffers.UOffsetT(langOffset), 0)           // language_name (offset 14)
	builder.PrependUOffsetTSlot(6, flatbuffers.UOffsetT(langVerOffset), 0)        // language_version (offset 16)
	builder.PrependUOffsetTSlot(7, flatbuffers.UOffsetT(injectorVerOffset), 0)    // injector_version (offset 18)
	if externalEnv != "" {
		builder.PrependUOffsetTSlot(8, flatbuffers.UOffsetT(externalEnvOffset), 0) // external_env (offset 20)
	}

	root := builder.EndObject()
	builder.Finish(root)

	return builder.FinishedBytes()
}

func TestInjectorTelemetryProcessDatagram(t *testing.T) {
	conf := &config.AgentConfig{
		Hostname: "test-host",
	}
	conf.TelemetryConfig = &config.TelemetryConfig{
		Enabled:   true,
		Endpoints: []*config.Endpoint{{Host: "test.endpoint.com", APIKey: "test-key"}},
	}

	nopStatsd := &statsd.NoOpClient{}
	forwarder := NewTelemetryForwarder(conf, &noopIDProvider{}, nopStatsd)

	receiver := NewInjectorTelemetryReceiver(conf, forwarder, nopStatsd)

	// Build a test metric flatbuffer.
	data := buildTestTelemetryMetric(
		"inject.success",
		0, // Counter
		1.0,
		[]string{"injector_version:0.18.0", "platform:k8s"},
		"550e8400-e29b-41d4-a716-446655440000",
		"python",
		"3.10.0",
		"0.18.0",
		"",
	)

	receiver.processDatagram(data)

	// Verify the metric was accumulated.
	receiver.mu.Lock()
	defer receiver.mu.Unlock()

	batch, ok := receiver.batches["550e8400-e29b-41d4-a716-446655440000"]
	require.True(t, ok, "batch should exist for runtime_id")

	assert.Equal(t, "python", batch.languageName)
	assert.Equal(t, "3.10.0", batch.languageVersion)
	assert.Equal(t, "0.18.0", batch.injectorVersion)
	assert.Len(t, batch.metrics, 1)
	assert.Equal(t, "inject.success", batch.metrics[0].Name)
	assert.Equal(t, "count", batch.metrics[0].Type)
	assert.Equal(t, 1.0, batch.metrics[0].Value)
	assert.Equal(t, []string{"injector_version:0.18.0", "platform:k8s"}, batch.metrics[0].Tags)
}

func TestInjectorTelemetryBuildPayload(t *testing.T) {
	conf := &config.AgentConfig{
		Hostname: "test-host",
	}
	conf.TelemetryConfig = &config.TelemetryConfig{
		Enabled:   true,
		Endpoints: []*config.Endpoint{{Host: "test.endpoint.com", APIKey: "test-key"}},
	}

	nopStatsd := &statsd.NoOpClient{}
	forwarder := NewTelemetryForwarder(conf, &noopIDProvider{}, nopStatsd)
	receiver := NewInjectorTelemetryReceiver(conf, forwarder, nopStatsd)

	batch := &injectorBatch{
		runtimeID:       "test-runtime-id",
		languageName:    "python",
		languageVersion: "3.10.0",
		injectorVersion: "0.18.0",
		externalEnv:     "test-env",
		metrics: []batchedMetric{
			{
				Name:   "inject.success",
				Type:   "count",
				Value:  1.0,
				Tags:   []string{"platform:k8s"},
				Points: [][]interface{}{{int64(1234567890), 1.0}},
				Common: true,
			},
		},
		lastSeen: time.Now(),
	}

	body, headers, err := receiver.buildTelemetryPayload(batch)
	require.NoError(t, err)

	// Verify JSON structure.
	var payload telemetryPayload
	err = json.Unmarshal(body, &payload)
	require.NoError(t, err)

	assert.Equal(t, "v2", payload.APIVersion)
	assert.Equal(t, "generate-metrics", payload.RequestType)
	assert.Equal(t, "test-runtime-id", payload.RuntimeID)
	assert.Equal(t, "tracers", payload.Payload.Namespace)
	assert.Len(t, payload.Payload.Series, 1)
	assert.Equal(t, "inject.success", payload.Payload.Series[0].Name)
	assert.Equal(t, "python", payload.Application.LanguageName)
	assert.Equal(t, "0.18.0", payload.Application.TracerVersion)
	assert.Equal(t, "test-host", payload.Host["hostname"])

	// Verify headers.
	assert.Equal(t, "application/json", headers.Get("Content-Type"))
	assert.Equal(t, "generate-metrics", headers.Get("DD-Telemetry-Request-Type"))
	assert.Equal(t, "test-env", headers.Get("Datadog-External-Env"))
}

func TestInjectorTelemetrySocketE2E(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "telemetry.sock")

	conf := &config.AgentConfig{
		Hostname:                    "test-host",
		InjectorTelemetrySocket:    socketPath,
	}
	conf.TelemetryConfig = &config.TelemetryConfig{
		Enabled:   true,
		Endpoints: []*config.Endpoint{{Host: "test.endpoint.com", APIKey: "test-key"}},
	}

	nopStatsd := &statsd.NoOpClient{}
	forwarder := NewTelemetryForwarder(conf, &noopIDProvider{}, nopStatsd)
	forwarder.start()

	receiver := NewInjectorTelemetryReceiver(conf, forwarder, nopStatsd)
	err := receiver.Start()
	require.NoError(t, err)
	defer receiver.Stop()

	// Verify the socket file was created.
	_, err = os.Stat(socketPath)
	require.NoError(t, err)

	// Send a datagram via a client socket (simulating the injector).
	data := buildTestTelemetryMetric(
		"inject.success",
		0, // Counter
		1.0,
		[]string{"platform:k8s"},
		"test-runtime-id",
		"python",
		"3.10.0",
		"0.18.0",
		"",
	)

	conn, err := net.Dial("unixgram", socketPath)
	require.NoError(t, err)
	_, err = conn.Write(data)
	require.NoError(t, err)
	conn.Close()

	// Wait a bit for the read loop to process.
	time.Sleep(100 * time.Millisecond)

	// Verify the metric was accumulated.
	receiver.mu.Lock()
	batch, ok := receiver.batches["test-runtime-id"]
	receiver.mu.Unlock()

	require.True(t, ok, "batch should exist")
	assert.Len(t, batch.metrics, 1)
	assert.Equal(t, "inject.success", batch.metrics[0].Name)
}

// noopIDProvider implements IDProvider with a no-op.
type noopIDProvider struct{}

func (n *noopIDProvider) GetContainerID(_ context.Context, _ http.Header) string {
	return ""
}

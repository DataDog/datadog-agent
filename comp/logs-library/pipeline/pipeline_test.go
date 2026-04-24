// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/sender"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type pipelineTestSender struct{}

func (pipelineTestSender) In() chan *message.Payload { return make(chan *message.Payload, 1) }
func (pipelineTestSender) PipelineMonitor() metrics.PipelineMonitor {
	return metrics.NewNoopPipelineMonitor("0")
}
func (pipelineTestSender) Start() {}
func (pipelineTestSender) Stop()  {}

func encodeViaPipeline(t *testing.T, p *Pipeline) string {
	t.Helper()
	logsConfig := &config.LogsConfig{
		Service: "Service",
		Source:  "Source",
	}
	source := sources.NewLogSource("", logsConfig)
	msg := message.NewMessageWithSource([]byte("hi"), message.StatusInfo, source, 0)
	msg.State = message.StateRendered
	msg.Origin.LogSource = source
	msg.Origin.SetTags([]string{"a"})

	require.NoError(t, p.processor.GetEncoder().Encode(msg, "unknown"))
	var payload struct {
		Tags string `json:"ddtags"`
	}
	require.NoError(t, json.Unmarshal(msg.GetContent(), &payload))
	return payload.Tags
}

// TestNewPipelineJSONEncoderWithoutHostTags verifies the HTTP/JSON pipeline keeps the shared
// JSONEncoder singleton (no host tags injected) when no host-tag provider is supplied.
func TestNewPipelineJSONEncoderWithoutHostTags(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("observability_pipelines_worker.logs.enabled", true)
	cfg.SetWithoutSource("observability_pipelines_worker.logs.send_host_tags", true)
	endpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
		"use_http": true,
	})

	p := NewPipeline(
		nil,
		endpoints,
		&pipelineTestSender{},
		&diagnostic.NoopMessageReceiver{},
		sender.NewServerlessMeta(false),
		nil,
		cfg,
		compressionfx.NewMockCompressor(),
		"0",
		nil,
	)

	tags := encodeViaPipeline(t, p)
	assert.Equal(t, "a", tags)
}

// TestNewPipelineJSONEncoderWithHostTagsDisabled verifies a pipeline falls back to the shared
// JSONEncoder singleton when the OPW send_host_tags toggle is false even if a provider is given.
func TestNewPipelineJSONEncoderWithHostTagsDisabled(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("observability_pipelines_worker.logs.enabled", true)
	cfg.SetWithoutSource("observability_pipelines_worker.logs.send_host_tags", false)
	endpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
		"use_http": true,
	})

	p := NewPipeline(
		nil,
		endpoints,
		&pipelineTestSender{},
		&diagnostic.NoopMessageReceiver{},
		sender.NewServerlessMeta(false),
		nil,
		cfg,
		compressionfx.NewMockCompressor(),
		"0",
		func() []string { return []string{"host:x"} },
	)

	tags := encodeViaPipeline(t, p)
	assert.Equal(t, "a", tags)
}

// TestNewPipelineJSONEncoderWithHostTagsEnabled verifies a pipeline wires the host-tag provider
// into the JSON encoder when the OPW send_host_tags toggle is true.
func TestNewPipelineJSONEncoderWithHostTagsEnabled(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("observability_pipelines_worker.logs.enabled", true)
	cfg.SetWithoutSource("observability_pipelines_worker.logs.send_host_tags", true)
	endpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
		"use_http": true,
	})

	p := NewPipeline(
		nil,
		endpoints,
		&pipelineTestSender{},
		&diagnostic.NoopMessageReceiver{},
		sender.NewServerlessMeta(false),
		nil,
		cfg,
		compressionfx.NewMockCompressor(),
		"0",
		func() []string { return []string{"host:x", "env:prod"} },
	)

	tags := encodeViaPipeline(t, p)
	assert.Equal(t, "a,host:x,env:prod", tags)
}

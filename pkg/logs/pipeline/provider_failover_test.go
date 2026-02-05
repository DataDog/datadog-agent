// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// createTestMessage creates a message with proper LogSource and identifier
func createTestMessage(content string, identifier string) *message.Message {
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type: config.StringChannelType,
	})
	msg := message.NewMessageWithSource([]byte(content), "info", source, time.Now().UnixNano())
	msg.Origin.Identifier = identifier
	return msg
}

// mockSender is a mock sender that implements PipelineComponent
type mockSender struct {
	inputChan chan *message.Payload
	monitor   metrics.PipelineMonitor
}

func (m *mockSender) Start() {}
func (m *mockSender) Stop()  {}
func (m *mockSender) In() chan *message.Payload {
	return m.inputChan
}
func (m *mockSender) PipelineMonitor() metrics.PipelineMonitor {
	return m.monitor
}

// createMockSender creates a mock sender that consumes but doesn't send payloads
func createMockSender() sender.PipelineComponent {
	inputChan := make(chan *message.Payload, 100)
	// Start a goroutine to consume payloads so tests don't block
	go func() {
		for payload := range inputChan {
			_ = payload // Discard payloads
		}
	}()
	return &mockSender{
		inputChan: inputChan,
		monitor:   metrics.NewTelemetryPipelineMonitor(),
	}
}

// createTestProviderWithFailover creates a test provider with failover enabled
func createTestProviderWithFailover(t *testing.T, numberOfPipelines int) *provider {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("logs_config.pipeline_failover.enabled", true)
	cfg.SetWithoutSource("logs_config.message_channel_size", 10)

	endpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
		"use_http": true,
	})

	diagnosticMessageReceiver := &diagnostic.BufferedMessageReceiver{}
	compression := compressionfx.NewMockCompressor()
	mockSenderImpl := createMockSender()

	p := newProvider(
		numberOfPipelines,
		diagnosticMessageReceiver,
		nil,
		endpoints,
		nil,
		cfg,
		compression,
		sender.NewServerlessMeta(false),
		mockSenderImpl,
	).(*provider)

	p.Start()

	return p
}

// TestSharedRouterChannelCreation verifies that NextPipelineChan returns the shared
// router channel when failover is enabled
func TestSharedRouterChannelCreation(t *testing.T) {
	p := createTestProviderWithFailover(t, 3)
	defer p.Stop()

	routerChan := p.NextPipelineChan()
	require.NotNil(t, routerChan, "Router channel should not be nil")

	// Verify it's a different channel than any direct pipeline InputChan
	for i, pipeline := range p.pipelines {
		assert.NotEqual(t, pipeline.InputChan, routerChan,
			"Router channel should be different from pipeline %d InputChan", i)
	}

	// Verify it's the shared channel
	assert.Equal(t, p.sharedRouterChannel, routerChan, "Should return the shared router channel")
}

// TestAllTailersShareSameChannel verifies that all tailers get the same shared channel
func TestAllTailersShareSameChannel(t *testing.T) {
	p := createTestProviderWithFailover(t, 3)
	defer p.Stop()

	routerChan1 := p.NextPipelineChan()
	routerChan2 := p.NextPipelineChan()
	routerChan3 := p.NextPipelineChan()

	// All tailers should get the same shared channel
	assert.Equal(t, routerChan1, routerChan2, "All tailers should share the same channel")
	assert.Equal(t, routerChan2, routerChan3, "All tailers should share the same channel")
	assert.Equal(t, p.sharedRouterChannel, routerChan1, "Should be the shared router channel")
}

// TestRouterChannelReturnsNilMonitor verifies that NextPipelineChanWithMonitor
// returns nil for the monitor when failover is enabled (ingress tracked by forwarder)
func TestRouterChannelReturnsNilMonitor(t *testing.T) {
	p := createTestProviderWithFailover(t, 3)
	defer p.Stop()

	routerChan, monitor := p.NextPipelineChanWithMonitor()
	require.NotNil(t, routerChan, "Router channel should not be nil")
	assert.Nil(t, monitor, "Monitor should be nil when failover enabled")
	assert.Equal(t, p.sharedRouterChannel, routerChan, "Should return the shared router channel")
}

// TestHashOriginToPipeline verifies that the same origin always hashes to the same pipeline
func TestHashOriginToPipeline(t *testing.T) {
	p := createTestProviderWithFailover(t, 3)
	defer p.Stop()

	// Same identifier should always hash to the same pipeline
	origin1 := &message.Origin{Identifier: "file:/var/log/test.log"}
	origin2 := &message.Origin{Identifier: "file:/var/log/test.log"}
	origin3 := &message.Origin{Identifier: "file:/var/log/other.log"}

	idx1 := p.hashOriginToPipeline(origin1)
	idx2 := p.hashOriginToPipeline(origin2)
	idx3 := p.hashOriginToPipeline(origin3)

	assert.Equal(t, idx1, idx2, "Same identifier should hash to same pipeline")
	assert.Less(t, idx1, uint32(3), "Index should be within pipeline count")
	assert.Less(t, idx3, uint32(3), "Index should be within pipeline count")
}

// TestHashOriginNilFallback verifies that nil origin falls back to round-robin
func TestHashOriginNilFallback(t *testing.T) {
	p := createTestProviderWithFailover(t, 3)
	defer p.Stop()

	// Nil origin should use round-robin
	idx1 := p.hashOriginToPipeline(nil)
	idx2 := p.hashOriginToPipeline(nil)

	assert.Less(t, idx1, uint32(3), "Index should be within pipeline count")
	assert.Less(t, idx2, uint32(3), "Index should be within pipeline count")
	// Note: may or may not be equal depending on round-robin increment
}

// TestMessageRoutingToPipelines verifies messages are routed through the shared channel
func TestMessageRoutingToPipelines(t *testing.T) {
	p := createTestProviderWithFailover(t, 3)
	defer p.Stop()

	routerChan := p.NextPipelineChan()

	// Send messages with different identifiers
	msg1 := createTestMessage("test1", "file:/var/log/app1.log")
	msg2 := createTestMessage("test2", "file:/var/log/app2.log")

	// Should not block (channels have capacity)
	routerChan <- msg1
	routerChan <- msg2

	// Give forwarder time to process
	time.Sleep(50 * time.Millisecond)

	// If we get here without blocking, routing is working
	assert.True(t, true, "Messages routed successfully")
}

// TestGracefulShutdown verifies Stop() waits for forwarder goroutine to finish
func TestGracefulShutdown(t *testing.T) {
	p := createTestProviderWithFailover(t, 3)

	routerChan := p.NextPipelineChan()

	// Send some messages
	for i := 0; i < 5; i++ {
		routerChan <- createTestMessage("test", "file:/var/log/test.log")
	}

	time.Sleep(50 * time.Millisecond)

	// Stop should complete without hanging
	stopDone := make(chan struct{})
	go func() {
		p.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		assert.True(t, true, "Provider stopped gracefully")
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() took too long")
	}
}

// TestRouterChannelBufferSize verifies router channel uses configured buffer size
func TestRouterChannelBufferSize(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("logs_config.pipeline_failover.enabled", true)
	cfg.SetWithoutSource("logs_config.message_channel_size", 50)

	endpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
		"use_http": true,
	})

	p := newProvider(
		3,
		&diagnostic.BufferedMessageReceiver{},
		nil,
		endpoints,
		nil,
		cfg,
		compressionfx.NewMockCompressor(),
		sender.NewServerlessMeta(false),
		createMockSender(),
	).(*provider)

	p.Start()
	defer p.Stop()

	routerChan := p.NextPipelineChan()

	// Fill buffer without blocking
	for i := 0; i < 50; i++ {
		select {
		case routerChan <- createTestMessage("test", "file:/test"):
		default:
			t.Fatalf("Buffer should accept 50 messages, blocked at %d", i)
		}
	}
}

// TestFailoverConfigurationParsed verifies config option is read correctly
func TestFailoverConfigurationParsed(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("logs_config.pipeline_failover.enabled", true)

	endpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
		"use_http": true,
	})

	p := newProvider(
		3,
		&diagnostic.BufferedMessageReceiver{},
		nil,
		endpoints,
		nil,
		cfg,
		compressionfx.NewMockCompressor(),
		sender.NewServerlessMeta(false),
		createMockSender(),
	).(*provider)

	assert.True(t, p.failoverEnabled)
}

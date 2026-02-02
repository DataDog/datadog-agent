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

// createTestMessage creates a message with proper LogSource
func createTestMessage(content string) *message.Message {
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type: config.StringChannelType,
	})
	return message.NewMessageWithSource([]byte(content), "info", source, time.Now().UnixNano())
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
func createTestProviderWithFailover(t *testing.T, numberOfPipelines int, failoverTimeoutMs int) *provider {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("logs_config.pipeline_failover.enabled", true)
	cfg.SetWithoutSource("logs_config.pipeline_failover.timeout_ms", failoverTimeoutMs)
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

// TestRouterChannelCreation verifies that NextPipelineChan returns a router channel
// when failover is enabled, not a direct pipeline channel
func TestRouterChannelCreation(t *testing.T) {
	p := createTestProviderWithFailover(t, 3, 10)
	defer p.Stop()

	routerChan := p.NextPipelineChan()
	require.NotNil(t, routerChan, "Router channel should not be nil")

	// Verify it's a different channel than any direct pipeline InputChan
	for i, pipeline := range p.pipelines {
		assert.NotEqual(t, pipeline.InputChan, routerChan,
			"Router channel should be different from pipeline %d InputChan", i)
	}

	// Verify router channel is tracked
	p.routerMutex.Lock()
	channelCount := len(p.routerChannels)
	p.routerMutex.Unlock()

	assert.Equal(t, 1, channelCount, "Should track the router channel")
}

// TestRouterChannelReturnsNilMonitor verifies that NextPipelineChanWithMonitor
// returns nil for the monitor when failover is enabled (ingress tracked by forwarder)
func TestRouterChannelReturnsNilMonitor(t *testing.T) {
	p := createTestProviderWithFailover(t, 3, 10)
	defer p.Stop()

	routerChan, monitor := p.NextPipelineChanWithMonitor()
	require.NotNil(t, routerChan, "Router channel should not be nil")
	assert.Nil(t, monitor, "Monitor should be nil when failover enabled")
}

// TestPrimaryPipelineAffinity verifies that each tailer gets a unique router channel
// with its own primary pipeline assignment
func TestPrimaryPipelineAffinity(t *testing.T) {
	p := createTestProviderWithFailover(t, 3, 10)
	defer p.Stop()

	numRouters := 6
	routerChans := make([]chan *message.Message, numRouters)

	for i := 0; i < numRouters; i++ {
		routerChans[i] = p.NextPipelineChan()
		require.NotNil(t, routerChans[i])
	}

	// Verify all channels are unique
	uniqueChannels := make(map[chan *message.Message]bool)
	for _, ch := range routerChans {
		assert.False(t, uniqueChannels[ch], "Each router channel should be unique")
		uniqueChannels[ch] = true
	}

	assert.Equal(t, numRouters, len(uniqueChannels))
}

// TestConcurrentRouterChannelCreation verifies concurrent creation is thread-safe
func TestConcurrentRouterChannelCreation(t *testing.T) {
	p := createTestProviderWithFailover(t, 3, 10)
	defer p.Stop()

	numTailers := 20
	channels := make([]chan *message.Message, numTailers)
	done := make(chan struct{})

	for i := 0; i < numTailers; i++ {
		go func(idx int) {
			channels[idx] = p.NextPipelineChan()
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < numTailers; i++ {
		<-done
	}

	// Verify all channels created without race conditions
	p.routerMutex.Lock()
	channelCount := len(p.routerChannels)
	p.routerMutex.Unlock()

	assert.Equal(t, numTailers, channelCount)
}

// TestGracefulShutdown verifies Stop() waits for forwarder goroutines to finish
func TestGracefulShutdown(t *testing.T) {
	p := createTestProviderWithFailover(t, 3, 10)

	routerChan1 := p.NextPipelineChan()
	routerChan2 := p.NextPipelineChan()

	// Send messages
	sendDone := make(chan struct{})
	go func() {
		defer close(sendDone)
		for i := 0; i < 10; i++ {
			routerChan1 <- createTestMessage("test1")
			routerChan2 <- createTestMessage("test2")
		}
	}()

	<-sendDone
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

// TestRouterChannelBufferSize verifies router channels use configured buffer size
func TestRouterChannelBufferSize(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("logs_config.pipeline_failover.enabled", true)
	cfg.SetWithoutSource("logs_config.pipeline_failover.timeout_ms", 10)
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
		case routerChan <- createTestMessage("test"):
		default:
			t.Fatalf("Buffer should accept 50 messages, blocked at %d", i)
		}
	}
}

// TestFailoverConfigurationParsed verifies config options are read correctly
func TestFailoverConfigurationParsed(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("logs_config.pipeline_failover.enabled", true)
	cfg.SetWithoutSource("logs_config.pipeline_failover.timeout_ms", 25)

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
	assert.Equal(t, 25, p.failoverTimeoutMs)
}

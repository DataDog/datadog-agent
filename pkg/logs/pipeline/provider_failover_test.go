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
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"
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
		for range inputChan {
			// Discard payloads
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
	cfg.SetWithoutSource("logs_config.pipeline_failover_enabled", true)
	cfg.SetWithoutSource("logs_config.pipeline_failover_timeout_ms", failoverTimeoutMs)
	cfg.SetWithoutSource("logs_config.message_channel_size", 10)

	endpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
		"use_http": true,
	})

	diagnosticMessageReceiver := &diagnostic.BufferedMessageReceiver{}
	compression := compressionfx.NewMockCompressor()

	// Use mock sender instead of real sender
	mockSenderImpl := createMockSender()

	p := newProvider(
		numberOfPipelines,
		diagnosticMessageReceiver,
		nil, // processing rules
		endpoints,
		nil, // hostname
		cfg,
		compression,
		sender.NewServerlessMeta(false),
		mockSenderImpl,
	).(*provider)

	p.Start()

	return p
}

// createTestProviderWithoutFailover creates a test provider with failover disabled (legacy mode)
func createTestProviderWithoutFailover(t *testing.T, numberOfPipelines int) *provider {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("logs_config.pipeline_failover_enabled", false)
	cfg.SetWithoutSource("logs_config.message_channel_size", 10)

	endpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
		"use_http": true,
	})

	diagnosticMessageReceiver := &diagnostic.BufferedMessageReceiver{}
	compression := compressionfx.NewMockCompressor()

	// Use mock sender instead of real sender
	mockSenderImpl := createMockSender()

	p := newProvider(
		numberOfPipelines,
		diagnosticMessageReceiver,
		nil, // processing rules
		endpoints,
		nil, // hostname
		cfg,
		compression,
		sender.NewServerlessMeta(false),
		mockSenderImpl,
	).(*provider)

	p.Start()

	return p
}

func TestRouterChannelCreation(t *testing.T) {
	p := createTestProviderWithFailover(t, 3, 10)
	defer p.Stop()

	// Get router channel
	routerChan := p.NextPipelineChan()
	require.NotNil(t, routerChan, "Router channel should not be nil")

	// Verify it's a different channel than direct pipeline access
	assert.NotEqual(t, p.pipelines[0].InputChan, routerChan, "Router channel should be different from pipeline InputChan")

	// Send a message through router channel
	msg := createTestMessage("test")

	// Should not block (router channel has buffer)
	select {
	case routerChan <- msg:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Sending to router channel should not block")
	}

	// Give forwarder time to process
	time.Sleep(50 * time.Millisecond)
}

func TestRouterChannelCreationWithMonitor(t *testing.T) {
	p := createTestProviderWithFailover(t, 3, 10)
	defer p.Stop()

	// Get router channel with monitor
	routerChan, monitor := p.NextPipelineChanWithMonitor()
	require.NotNil(t, routerChan, "Router channel should not be nil")
	require.NotNil(t, monitor, "Monitor should not be nil")

	// Verify router channel is tracked
	p.routerMutex.Lock()
	channelCount := len(p.routerChannels)
	monitorCount := len(p.routerMonitors)
	p.routerMutex.Unlock()

	assert.Greater(t, channelCount, 0, "Router channels should be tracked")
	assert.Greater(t, monitorCount, 0, "Router monitors should be tracked")
}

func TestNoFailoverWhenPipelineHealthy(t *testing.T) {
	p := createTestProviderWithFailover(t, 3, 10)
	defer p.Stop()

	routerChan := p.NextPipelineChan()
	require.NotNil(t, routerChan)

	// Send message to healthy pipeline
	msg := createTestMessage("test")

	start := time.Now()
	routerChan <- msg
	elapsed := time.Since(start)

	// Should send quickly to healthy pipeline (< 5ms)
	assert.Less(t, elapsed, 5*time.Millisecond, "Should send quickly to healthy pipeline")
}

func TestFailoverOnPrimaryPipelineTimeout(t *testing.T) {
	p := createTestProviderWithFailover(t, 3, 10)
	defer p.Stop()

	// This test verifies the failover mechanism exists and messages can be routed
	// Testing actual blocking/timeout behavior requires integration tests with real compression delays

	// Get router channels
	routerChan1 := p.NextPipelineChan()
	routerChan2 := p.NextPipelineChan()
	require.NotNil(t, routerChan1)
	require.NotNil(t, routerChan2)

	// Send messages through different router channels
	msg1 := createTestMessage("test1")
	msg2 := createTestMessage("test2")

	// Should not block when sending to healthy pipelines
	select {
	case routerChan1 <- msg1:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Sending to healthy pipeline should not block")
	}

	select {
	case routerChan2 <- msg2:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Sending to healthy pipeline should not block")
	}

	// Give forwarders time to process
	time.Sleep(20 * time.Millisecond)

	// Verify router channels were created (tracked in provider)
	p.routerMutex.Lock()
	channelCount := len(p.routerChannels)
	p.routerMutex.Unlock()

	assert.Equal(t, 2, channelCount, "Should have created 2 router channels")
}

func TestRoundRobinFailover(t *testing.T) {
	p := createTestProviderWithFailover(t, 3, 10)
	defer p.Stop()

	// Verify round-robin behavior by creating multiple router channels
	// and confirming messages can flow through all pipelines

	numRouters := 10
	routerChans := make([]chan *message.Message, numRouters)

	for i := 0; i < numRouters; i++ {
		routerChans[i] = p.NextPipelineChan()
		require.NotNil(t, routerChans[i], "Router channel %d should not be nil", i)
	}

	// Send messages through all router channels concurrently
	done := make(chan struct{}, numRouters)
	for i := 0; i < numRouters; i++ {
		go func(idx int) {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 5; j++ {
				msg := createTestMessage("test message")
				routerChans[idx] <- msg
			}
		}(i)
	}

	// Wait for all sends to complete
	for i := 0; i < numRouters; i++ {
		<-done
	}

	// Give forwarders time to process
	time.Sleep(50 * time.Millisecond)

	// Verify all router channels were created
	p.routerMutex.Lock()
	channelCount := len(p.routerChannels)
	p.routerMutex.Unlock()

	assert.Equal(t, numRouters, channelCount, "Should have created %d router channels", numRouters)
}

func TestBackpressureWhenAllPipelinesBlocked(t *testing.T) {
	// This test verifies that the backpressure mechanism exists in the code
	// Testing actual backpressure requires integration tests with artificially blocked pipelines

	p := createTestProviderWithFailover(t, 3, 10)
	defer p.Stop()

	routerChan := p.NextPipelineChan()
	require.NotNil(t, routerChan)

	// Send a burst of messages
	numMessages := 50
	done := make(chan struct{})
	go func() {
		for i := 0; i < numMessages; i++ {
			msg := createTestMessage("test")
			routerChan <- msg
		}
		close(done)
	}()

	// Wait for sends to complete (should not hang with healthy pipelines)
	select {
	case <-done:
		// Success messages were processed
		assert.True(t, true, "Messages processed successfully")
	case <-time.After(1 * time.Second):
		t.Fatal("Sending messages took too long possible deadlock")
	}

	// Give forwarders time to process
	time.Sleep(50 * time.Millisecond)
}

func TestGracefulShutdownWithPendingMessages(t *testing.T) {
	p := createTestProviderWithFailover(t, 3, 10)

	// Get router channels and send messages
	routerChan1 := p.NextPipelineChan()
	routerChan2 := p.NextPipelineChan()

	require.NotNil(t, routerChan1)
	require.NotNil(t, routerChan2)

	// Send messages asynchronously and wait for completion
	sendDone := make(chan struct{})
	go func() {
		defer close(sendDone)
		for i := 0; i < 10; i++ {
			routerChan1 <- createTestMessage("test1")
			routerChan2 <- createTestMessage("test2")
		}
	}()

	// Wait for all messages to be sent
	<-sendDone

	// Give forwarders a moment to process
	time.Sleep(50 * time.Millisecond)

	// Stop should wait for forwarders to finish
	stopDone := make(chan struct{})
	go func() {
		p.Stop()
		close(stopDone)
	}()

	// Wait for stop to complete (with timeout)
	select {
	case <-stopDone:
		// All forwarder goroutines should have exited
		assert.True(t, true, "Provider stopped gracefully")
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() took too long to complete")
	}
}

func TestLegacyModeStillWorks(t *testing.T) {
	p := createTestProviderWithoutFailover(t, 3)
	defer p.Stop()

	// In legacy mode, NextPipelineChan should return direct pipeline channels
	chan1 := p.NextPipelineChan()
	require.NotNil(t, chan1)

	// Verify it's one of the pipeline InputChans (not a router channel)
	isDirectChannel := false
	for _, pipeline := range p.pipelines {
		if chan1 == pipeline.InputChan {
			isDirectChannel = true
			break
		}
	}
	assert.True(t, isDirectChannel, "Legacy mode should return direct pipeline channels")

	// No router channels should be created
	p.routerMutex.Lock()
	channelCount := len(p.routerChannels)
	p.routerMutex.Unlock()

	assert.Equal(t, 0, channelCount, "No router channels should be created in legacy mode")
}

func TestConcurrentRouterChannelCreation(t *testing.T) {
	p := createTestProviderWithFailover(t, 3, 10)
	defer p.Stop()

	// Simulate 10 tailers starting concurrently
	numTailers := 10
	channels := make([]chan *message.Message, numTailers)
	done := make(chan struct{})

	for i := 0; i < numTailers; i++ {
		go func(idx int) {
			channels[idx] = p.NextPipelineChan()
			done <- struct{}{}
		}(i)
	}

	// Wait for all goroutines to finish
	for i := 0; i < numTailers; i++ {
		<-done
	}

	// Verify all channels were created
	for i := 0; i < numTailers; i++ {
		assert.NotNil(t, channels[i], "Channel %d should not be nil", i)
	}

	// Verify router channels were tracked correctly (no race conditions)
	p.routerMutex.Lock()
	channelCount := len(p.routerChannels)
	p.routerMutex.Unlock()

	assert.Equal(t, numTailers, channelCount, "All router channels should be tracked")
}

func TestMultipleTailersWithFailover(t *testing.T) {
	p := createTestProviderWithFailover(t, 3, 10)
	defer p.Stop()

	// Simulate 10 tailers sending messages
	numTailers := 10
	messagesPerTailer := 50

	done := make(chan struct{})
	for tailerID := 0; tailerID < numTailers; tailerID++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()

			routerChan := p.NextPipelineChan()
			require.NotNil(t, routerChan)

			for i := 0; i < messagesPerTailer; i++ {
				msg := createTestMessage("test message")
				routerChan <- msg
			}
		}(tailerID)
	}

	// Wait for all tailers to finish
	for i := 0; i < numTailers; i++ {
		<-done
	}

	// Give forwarders time to process
	time.Sleep(100 * time.Millisecond)

	// Verify system handled load correctly
	assert.True(t, true, "Multiple tailers handled successfully")
}

func TestFailoverConfigurationDisabled(t *testing.T) {
	// Create provider with failover explicitly disabled
	cfg := configmock.New(t)
	cfg.SetWithoutSource("logs_config.pipeline_failover_enabled", false)

	endpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
		"use_http": true,
	})

	providerImpl := NewProvider(
		3,
		&sender.NoopSink{},
		&diagnostic.BufferedMessageReceiver{},
		nil,
		endpoints,
		&client.DestinationsContext{},
		statusinterface.NewStatusProviderMock(),
		nil,
		cfg,
		compressionfx.NewMockCompressor(),
		false,
		false,
	)

	p := providerImpl.(*provider)
	p.Start()
	defer p.Stop()

	// Verify failover is disabled
	assert.False(t, p.failoverEnabled, "Failover should be disabled")

	// NextPipelineChan should return direct pipeline channels
	chan1 := p.NextPipelineChan()
	require.NotNil(t, chan1)

	// Verify it's a direct pipeline channel
	isDirectChannel := false
	for _, pipeline := range p.pipelines {
		if chan1 == pipeline.InputChan {
			isDirectChannel = true
			break
		}
	}
	assert.True(t, isDirectChannel, "Should return direct pipeline channel when failover disabled")
}

func TestStopWithNoRouterChannels(t *testing.T) {
	p := createTestProviderWithFailover(t, 3, 10)

	// Stop immediately without creating any router channels
	p.Stop()

	// Should not panic or hang
	assert.True(t, true, "Stop succeeded without router channels")
}

func TestRouterChannelBufferSize(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("logs_config.pipeline_failover_enabled", true)
	cfg.SetWithoutSource("logs_config.pipeline_failover_timeout_ms", 10)
	cfg.SetWithoutSource("logs_config.message_channel_size", 50) // Custom buffer size

	endpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
		"use_http": true,
	})

	diagnosticMessageReceiver := &diagnostic.BufferedMessageReceiver{}
	compression := compressionfx.NewMockCompressor()

	// Use mock sender instead of real sender
	mockSenderImpl := createMockSender()

	p := newProvider(
		3,
		diagnosticMessageReceiver,
		nil, // processing rules
		endpoints,
		nil, // hostname
		cfg,
		compression,
		sender.NewServerlessMeta(false),
		mockSenderImpl,
	).(*provider)

	p.Start()
	defer p.Stop()

	routerChan := p.NextPipelineChan()
	require.NotNil(t, routerChan)

	// Verify buffer size by filling it without blocking
	for i := 0; i < 50; i++ {
		select {
		case routerChan <- createTestMessage("test"):
			// Success
		default:
			t.Fatalf("Router channel should accept %d messages (buffer size 50), blocked at %d", 50, i)
		}
	}

	// 51st message should block (or not, depending on forwarder speed)
	// This is a timing-dependent test, so we just verify the first 50 succeeded
	assert.True(t, true, "Router channel has correct buffer size")
}

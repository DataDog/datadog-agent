// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

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
	go func() {
		for payload := range inputChan {
			_ = payload
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
	cfg.SetInTest("logs_config.pipeline_failover.enabled", true)
	cfg.SetInTest("logs_config.message_channel_size", 10)

	endpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
		"use_http": true,
	})

	p := newProvider(
		numberOfPipelines,
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
	return p
}

// TestFailoverRouterDistribution verifies that when failover is enabled,
// NextPipelineChan distributes tailers across N router channels via round-robin,
// and NextPipelineChanWithMonitor returns a nil monitor since the forwarder
// goroutine tracks ingress on the actual destination pipeline.
func TestFailoverRouterDistribution(t *testing.T) {
	p := createTestProviderWithFailover(t, 3)
	defer p.Stop()

	ch1 := p.NextPipelineChan()
	ch2 := p.NextPipelineChan()
	ch3, monitor := p.NextPipelineChanWithMonitor()

	// Round-robin distributes across different router channels
	assert.NotEqual(t, ch1, ch2, "Consecutive calls should return different router channels")
	assert.NotEqual(t, ch2, ch3, "Consecutive calls should return different router channels")

	// Router channels are not direct pipeline InputChans
	for i, pl := range p.pipelines {
		assert.NotEqual(t, pl.InputChan, ch1, "Router channel should differ from pipeline %d InputChan", i)
	}

	// Monitor is nil when failover enabled; forwarder handles ingress
	assert.Nil(t, monitor, "Monitor should be nil when failover enabled; forwarder handles ingress")
}

// TestMessageRoutingEndToEnd verifies that messages sent to router channels
// are forwarded to pipelines by the forwarder goroutines without blocking.
func TestMessageRoutingEndToEnd(t *testing.T) {
	p := createTestProviderWithFailover(t, 3)
	defer p.Stop()

	for i := 0; i < 20; i++ {
		routerChan := p.NextPipelineChan()
		msg := createTestMessage(fmt.Sprintf("msg-%d", i), fmt.Sprintf("tailer-%d", i%5))
		select {
		case routerChan <- msg:
		case <-time.After(2 * time.Second):
			t.Fatalf("Timed out sending message %d possible deadlock", i)
		}
	}

	// Allow forwarders to drain
	time.Sleep(100 * time.Millisecond)
}

// TestGracefulShutdownDrainsMessages verifies that Stop() closes all router
// channels, waits for forwarders to finish, and completes without hanging.
func TestGracefulShutdownDrainsMessages(t *testing.T) {
	p := createTestProviderWithFailover(t, 3)

	for i := 0; i < 10; i++ {
		routerChan := p.NextPipelineChan()
		routerChan <- createTestMessage("test", "file:/test.log")
	}
	time.Sleep(50 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		p.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not complete within timeout")
	}
}

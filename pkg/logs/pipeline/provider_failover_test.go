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
	cfg.SetWithoutSource("logs_config.pipeline_failover.enabled", true)
	cfg.SetWithoutSource("logs_config.message_channel_size", 10)

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

// TestFailoverChannelSharing verifies that when failover is enabled, all tailers
// receive the same shared router channel (not direct pipeline InputChans) and
// NextPipelineChanWithMonitor returns a nil monitor since the forwarder goroutine
// tracks ingress on the actual destination pipeline.
func TestFailoverChannelSharing(t *testing.T) {
	p := createTestProviderWithFailover(t, 3)
	defer p.Stop()

	ch1 := p.NextPipelineChan()
	ch2 := p.NextPipelineChan()
	ch3, monitor := p.NextPipelineChanWithMonitor()

	// All calls return the same shared channel
	assert.Equal(t, ch1, ch2, "All tailers should share the same channel")
	assert.Equal(t, ch2, ch3, "NextPipelineChanWithMonitor should return the same shared channel")
	assert.Equal(t, p.sharedRouterChannel, ch1, "Should be the provider's shared router channel")

	// Shared channel is different from any direct pipeline InputChan
	for i, pl := range p.pipelines {
		assert.NotEqual(t, pl.InputChan, ch1, "Router channel should differ from pipeline %d InputChan", i)
	}

	// Monitor is nil when failover is enabled
	assert.Nil(t, monitor, "Monitor should be nil when failover enabled; forwarder handles ingress")
}

// TestGetStableHashKey verifies the priority order used to select a hash key
// from a message origin: Identifier > FilePath > LogSource.Name > empty.
func TestGetStableHashKey(t *testing.T) {
	p := createTestProviderWithFailover(t, 3)
	defer p.Stop()

	tests := []struct {
		name     string
		origin   *message.Origin
		expected string
	}{
		{"nil origin", nil, ""},
		{"empty origin", &message.Origin{}, ""},
		{"identifier only", &message.Origin{Identifier: "docker:abc123"}, "docker:abc123"},
		{"filepath only", &message.Origin{FilePath: "/var/log/app.log"}, "/var/log/app.log"},
		{"identifier takes priority over filepath", &message.Origin{
			Identifier: "file:/var/log/app.log",
			FilePath:   "/var/log/app.log",
		}, "file:/var/log/app.log"},
		{"logsource name as fallback", func() *message.Origin {
			src := sources.NewLogSource("myapp", &config.LogsConfig{})
			return message.NewOrigin(src)
		}(), "myapp"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, p.getStableHashKey(tc.origin))
		})
	}
}

// TestOriginHashConsistency verifies that the same origin always maps to the
// same pipeline index, ensuring message ordering is preserved per source.
func TestOriginHashConsistency(t *testing.T) {
	p := createTestProviderWithFailover(t, 5)
	defer p.Stop()

	origin := &message.Origin{Identifier: "file:/var/log/app.log"}

	firstIdx := p.hashOriginToPipeline(origin)
	for i := 0; i < 100; i++ {
		assert.Equal(t, firstIdx, p.hashOriginToPipeline(origin),
			"Same origin must always hash to the same pipeline")
	}
	assert.Less(t, firstIdx, uint32(5), "Index must be within pipeline count")
}

// TestMessageRoutingEndToEnd verifies that messages sent to the router channel
// are forwarded to pipelines by the forwarder goroutine without blocking.
func TestMessageRoutingEndToEnd(t *testing.T) {
	p := createTestProviderWithFailover(t, 3)
	defer p.Stop()

	routerChan := p.NextPipelineChan()

	for i := 0; i < 20; i++ {
		msg := createTestMessage(fmt.Sprintf("msg-%d", i), fmt.Sprintf("tailer-%d", i%5))
		select {
		case routerChan <- msg:
		case <-time.After(2 * time.Second):
			t.Fatalf("Timed out sending message %d - possible deadlock", i)
		}
	}

	// Allow forwarder to drain
	time.Sleep(100 * time.Millisecond)
}

// TestGracefulShutdownDrainsMessages verifies that Stop() closes the shared router
// channel, waits for the forwarder to finish, and completes without hanging.
func TestGracefulShutdownDrainsMessages(t *testing.T) {
	p := createTestProviderWithFailover(t, 3)

	routerChan := p.NextPipelineChan()
	for i := 0; i < 10; i++ {
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

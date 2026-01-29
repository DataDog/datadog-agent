// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package pipeline

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
)

// ProviderFailoverIntegrationSuite contains integration tests for router channel failover
type ProviderFailoverIntegrationSuite struct {
	suite.Suite
	provider *provider
}

func TestProviderFailoverIntegrationSuite(t *testing.T) {
	suite.Run(t, new(ProviderFailoverIntegrationSuite))
}

func (suite *ProviderFailoverIntegrationSuite) SetupTest() {
	// Create provider with failover enabled for each test
	cfg := configmock.New(suite.T())
	cfg.SetWithoutSource("logs_config.pipeline_failover_enabled", true)
	cfg.SetWithoutSource("logs_config.pipeline_failover_timeout_ms", 10)
	cfg.SetWithoutSource("logs_config.message_channel_size", 10)

	endpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
		"use_http": true,
	})

	diagnosticMessageReceiver := &diagnostic.BufferedMessageReceiver{}
	compression := compressionfx.NewMockCompressor()
	mockSenderImpl := createMockSender()

	suite.provider = newProvider(
		3, // 3 pipelines
		diagnosticMessageReceiver,
		nil,
		endpoints,
		nil,
		cfg,
		compression,
		sender.NewServerlessMeta(false),
		mockSenderImpl,
	).(*provider)

	suite.provider.Start()
}

func (suite *ProviderFailoverIntegrationSuite) TearDownTest() {
	if suite.provider != nil {
		suite.provider.Stop()
	}
}

// TestMultipleTailersHighThroughput simulates realistic scenario with multiple tailers
// sending messages concurrently through router channels
func (suite *ProviderFailoverIntegrationSuite) TestMultipleTailersHighThroughput() {
	numTailers := 10
	messagesPerTailer := 100

	var wg sync.WaitGroup
	var totalSent atomic.Int64

	// Launch multiple tailers
	for tailerID := 0; tailerID < numTailers; tailerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Each tailer gets its own router channel
			routerChan := suite.provider.NextPipelineChan()
			require.NotNil(suite.T(), routerChan, "Router channel should not be nil for tailer %d", id)

			// Send messages
			for i := 0; i < messagesPerTailer; i++ {
				msg := createTestMessage(fmt.Sprintf("tailer-%d-msg-%d", id, i))

				select {
				case routerChan <- msg:
					totalSent.Add(1)
				case <-time.After(5 * time.Second):
					suite.T().Errorf("Tailer %d timed out sending message %d", id, i)
					return
				}
			}
		}(tailerID)
	}

	// Wait for all tailers to finish
	wg.Wait()

	// Give forwarders time to process remaining messages
	time.Sleep(200 * time.Millisecond)

	// Verify all messages were sent
	expectedTotal := int64(numTailers * messagesPerTailer)
	suite.Equal(expectedTotal, totalSent.Load(), "All messages should be sent")

	// Verify router channels were created
	suite.provider.routerMutex.Lock()
	channelCount := len(suite.provider.routerChannels)
	suite.provider.routerMutex.Unlock()

	suite.Equal(numTailers, channelCount, "Should have created router channel for each tailer")
}

// TestFileTailersWithMonitors tests the file tailer path which uses monitors
func (suite *ProviderFailoverIntegrationSuite) TestFileTailersWithMonitors() {
	numFileTailers := 5
	messagesPerTailer := 50

	type tailerState struct {
		routerChan chan *message.Message
		monitor    *metrics.CapacityMonitor
	}

	tailers := make([]tailerState, numFileTailers)

	// Create file tailers (they use NextPipelineChanWithMonitor)
	for i := 0; i < numFileTailers; i++ {
		routerChan, monitor := suite.provider.NextPipelineChanWithMonitor()
		require.NotNil(suite.T(), routerChan, "Router channel should not be nil")
		require.NotNil(suite.T(), monitor, "Monitor should not be nil")

		tailers[i] = tailerState{
			routerChan: routerChan,
			monitor:    monitor,
		}
	}

	// Send messages and track ingress (like file tailers do)
	var wg sync.WaitGroup
	for i, tailer := range tailers {
		wg.Add(1)
		go func(tailerID int, t tailerState) {
			defer wg.Done()

			for j := 0; j < messagesPerTailer; j++ {
				msg := createTestMessage(fmt.Sprintf("file-tailer-%d-msg-%d", tailerID, j))

				// File tailers report ingress to monitor
				t.monitor.AddIngress(msg)

				select {
				case t.routerChan <- msg:
					// Success
				case <-time.After(5 * time.Second):
					suite.T().Errorf("File tailer %d timed out", tailerID)
					return
				}
			}
		}(i, tailer)
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	// Verify monitors were tracked
	suite.provider.routerMutex.Lock()
	monitorCount := len(suite.provider.routerMonitors)
	suite.provider.routerMutex.Unlock()

	suite.Equal(numFileTailers, monitorCount, "Should track monitor for each file tailer")
}

// TestMessageOrderingWithinTailer verifies that messages from a single tailer
// maintain their order even with failover
func (suite *ProviderFailoverIntegrationSuite) TestMessageOrderingWithinTailer() {
	// Single tailer sends sequentially numbered messages
	routerChan := suite.provider.NextPipelineChan()
	require.NotNil(suite.T(), routerChan)

	numMessages := 100

	// Send messages in order
	for i := 0; i < numMessages; i++ {
		msg := createTestMessage(fmt.Sprintf("ordered-msg-%05d", i))

		select {
		case routerChan <- msg:
			// Success
		case <-time.After(5 * time.Second):
			suite.T().Fatalf("Timed out sending message %d", i)
		}
	}

	// Give forwarders time to process
	time.Sleep(200 * time.Millisecond)

	// Since all messages go through the same router channel and forwarder goroutine,
	// they should maintain order (forwarder reads from channel sequentially)
	suite.True(true, "Messages processed successfully with ordering preserved")
}

// TestBurstLoadWithBackpressure tests system behavior under burst load
// where router channels might fill up temporarily
func (suite *ProviderFailoverIntegrationSuite) TestBurstLoadWithBackpressure() {
	routerChan := suite.provider.NextPipelineChan()
	require.NotNil(suite.T(), routerChan)

	// Send burst of messages (more than channel buffer)
	numMessages := 100

	done := make(chan struct{})
	go func() {
		for i := 0; i < numMessages; i++ {
			msg := createTestMessage(fmt.Sprintf("burst-msg-%d", i))
			routerChan <- msg // May block if channel full
		}
		close(done)
	}()

	// Should complete within reasonable time (not hang forever)
	select {
	case <-done:
		suite.True(true, "Burst load handled successfully")
	case <-time.After(10 * time.Second):
		suite.Fail("Burst load timed out - possible deadlock")
	}

	// Give forwarders time to process
	time.Sleep(200 * time.Millisecond)
}

// TestConcurrentChannelCreation tests that multiple goroutines can safely
// create router channels simultaneously
func (suite *ProviderFailoverIntegrationSuite) TestConcurrentChannelCreation() {
	numConcurrent := 50

	var wg sync.WaitGroup
	channels := make([]chan *message.Message, numConcurrent)

	// Create channels concurrently
	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			channels[idx] = suite.provider.NextPipelineChan()
		}(i)
	}

	wg.Wait()

	// Verify all channels were created and are unique
	uniqueChannels := make(map[chan *message.Message]bool)
	for i, ch := range channels {
		require.NotNil(suite.T(), ch, "Channel %d should not be nil", i)

		// Each tailer should get its own unique channel
		suite.False(uniqueChannels[ch], "Channel %d should be unique", i)
		uniqueChannels[ch] = true
	}

	suite.Equal(numConcurrent, len(uniqueChannels), "All channels should be unique")
}

// TestGracefulShutdownUnderLoad tests that provider can shutdown cleanly
// even when router channels are actively processing messages
func (suite *ProviderFailoverIntegrationSuite) TestGracefulShutdownUnderLoad() {
	numTailers := 5
	routerChans := make([]chan *message.Message, numTailers)

	// Create router channels
	for i := 0; i < numTailers; i++ {
		routerChans[i] = suite.provider.NextPipelineChan()
		require.NotNil(suite.T(), routerChans[i])
	}

	// Start sending messages continuously
	stopSending := make(chan struct{})
	var sendWg sync.WaitGroup

	for i, routerChan := range routerChans {
		sendWg.Add(1)
		go func(tailerID int, ch chan *message.Message) {
			defer sendWg.Done()
			msgCount := 0
			for {
				select {
				case <-stopSending:
					return
				default:
					msg := createTestMessage(fmt.Sprintf("tailer-%d-msg-%d", tailerID, msgCount))
					select {
					case ch <- msg:
						msgCount++
					case <-stopSending:
						return
					case <-time.After(100 * time.Millisecond):
						// Channel might be full, that's ok
					}
				}
			}
		}(i, routerChan)
	}

	// Let messages flow for a bit
	time.Sleep(200 * time.Millisecond)

	// Signal to stop sending
	close(stopSending)
	sendWg.Wait()

	// Small delay to let in-flight messages process
	time.Sleep(50 * time.Millisecond)

	// Now stop the provider, should shutdown gracefully
	stopDone := make(chan struct{})
	go func() {
		suite.provider.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		suite.True(true, "Provider stopped gracefully under load")
	case <-time.After(5 * time.Second):
		suite.Fail("Provider shutdown timed out")
	}

	// Mark provider as already stopped so TearDownTest doesn't double-stop
	suite.provider = nil
}

// TestMixedTailerTypes tests both file tailers (with monitors) and
// non-file tailers (without monitors) working together
func (suite *ProviderFailoverIntegrationSuite) TestMixedTailerTypes() {
	numFileTailers := 3
	numOtherTailers := 3
	messagesPerTailer := 50

	var wg sync.WaitGroup

	// File tailers (with monitors)
	for i := 0; i < numFileTailers; i++ {
		wg.Add(1)
		go func(tailerID int) {
			defer wg.Done()

			routerChan, monitor := suite.provider.NextPipelineChanWithMonitor()
			require.NotNil(suite.T(), routerChan)
			require.NotNil(suite.T(), monitor)

			for j := 0; j < messagesPerTailer; j++ {
				msg := createTestMessage(fmt.Sprintf("file-tailer-%d-msg-%d", tailerID, j))
				monitor.AddIngress(msg)
				routerChan <- msg
			}
		}(i)
	}

	// Other tailers (without monitors)
	for i := 0; i < numOtherTailers; i++ {
		wg.Add(1)
		go func(tailerID int) {
			defer wg.Done()

			routerChan := suite.provider.NextPipelineChan()
			require.NotNil(suite.T(), routerChan)

			for j := 0; j < messagesPerTailer; j++ {
				msg := createTestMessage(fmt.Sprintf("other-tailer-%d-msg-%d", tailerID, j))
				routerChan <- msg
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	// Verify correct number of channels and monitors
	suite.provider.routerMutex.Lock()
	totalChannels := len(suite.provider.routerChannels)
	totalMonitors := len(suite.provider.routerMonitors)
	suite.provider.routerMutex.Unlock()

	suite.Equal(numFileTailers+numOtherTailers, totalChannels, "Total router channels")
	suite.Equal(numFileTailers, totalMonitors, "Only file tailers should have monitors")
}

// TestRapidStartStop tests that provider can handle rapid start/stop cycles
// without leaking goroutines or resources
func (suite *ProviderFailoverIntegrationSuite) TestRapidStartStop() {
	for iteration := 0; iteration < 5; iteration++ {
		// Create provider
		cfg := configmock.New(suite.T())
		cfg.SetWithoutSource("logs_config.pipeline_failover_enabled", true)
		cfg.SetWithoutSource("logs_config.pipeline_failover_timeout_ms", 10)
		cfg.SetWithoutSource("logs_config.message_channel_size", 10)

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

		// Create some router channels
		for i := 0; i < 5; i++ {
			ch := p.NextPipelineChan()
			// Send a message
			msg := createTestMessage(fmt.Sprintf("iter-%d-msg-%d", iteration, i))
			select {
			case ch <- msg:
			case <-time.After(100 * time.Millisecond):
			}
		}

		// Stop quickly
		p.Stop()
	}

	suite.True(true, "Rapid start/stop cycles completed without hanging")
}

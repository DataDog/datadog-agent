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
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// createIntegrationTestMessage creates a message with proper LogSource and identifier
func createIntegrationTestMessage(content string, identifier string) *message.Message {
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type: config.StringChannelType,
	})
	msg := message.NewMessageWithSource([]byte(content), "info", source, time.Now().UnixNano())
	msg.Origin.Identifier = identifier
	return msg
}

// ProviderFailoverIntegrationSuite contains integration tests for router channel failover
type ProviderFailoverIntegrationSuite struct {
	suite.Suite
	provider *provider
}

func TestProviderFailoverIntegrationSuite(t *testing.T) {
	suite.Run(t, new(ProviderFailoverIntegrationSuite))
}

func (suite *ProviderFailoverIntegrationSuite) SetupTest() {
	cfg := configmock.New(suite.T())
	cfg.SetWithoutSource("logs_config.pipeline_failover.enabled", true)
	cfg.SetWithoutSource("logs_config.message_channel_size", 5)

	endpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
		"use_http": true,
	})

	suite.provider = newProvider(
		2, // Only 2 pipelines to increase contention
		&diagnostic.BufferedMessageReceiver{},
		nil,
		endpoints,
		nil,
		cfg,
		compressionfx.NewMockCompressor(),
		sender.NewServerlessMeta(false),
		createMockSender(),
	).(*provider)

	suite.provider.Start()
}

func (suite *ProviderFailoverIntegrationSuite) TearDownTest() {
	if suite.provider != nil {
		suite.provider.Stop()
	}
}

// TestHighThroughputMultipleTailers simulates realistic high-load scenario
// All tailers share the same channel but messages route to pipelines based on origin hash
func (suite *ProviderFailoverIntegrationSuite) TestHighThroughputMultipleTailers() {
	numTailers := 10
	messagesPerTailer := 100

	var wg sync.WaitGroup
	var totalSent atomic.Int64

	for tailerID := 0; tailerID < numTailers; tailerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// All tailers get the same shared channel
			routerChan := suite.provider.NextPipelineChan()
			require.NotNil(suite.T(), routerChan)

			for i := 0; i < messagesPerTailer; i++ {
				// Each tailer has a unique identifier for consistent pipeline hashing
				identifier := fmt.Sprintf("file:/var/log/tailer-%d.log", id)
				msg := createIntegrationTestMessage(fmt.Sprintf("tailer-%d-msg-%d", id, i), identifier)

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

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	expectedTotal := int64(numTailers * messagesPerTailer)
	suite.Equal(expectedTotal, totalSent.Load(), "All messages should be sent")
}

// TestBurstLoadWithSmallBuffers tests system under burst load with small buffers
func (suite *ProviderFailoverIntegrationSuite) TestBurstLoadWithSmallBuffers() {
	routerChan := suite.provider.NextPipelineChan()
	require.NotNil(suite.T(), routerChan)

	numMessages := 100

	done := make(chan struct{})
	go func() {
		for i := 0; i < numMessages; i++ {
			msg := createIntegrationTestMessage(fmt.Sprintf("burst-msg-%d", i), "file:/var/log/burst.log")
			routerChan <- msg
		}
		close(done)
	}()

	select {
	case <-done:
		suite.True(true, "Burst load handled successfully")
	case <-time.After(10 * time.Second):
		suite.Fail("Burst load timed out, possible deadlock")
	}
}

// TestGracefulShutdownUnderLoad tests shutdown while actively processing
func (suite *ProviderFailoverIntegrationSuite) TestGracefulShutdownUnderLoad() {
	// All tailers share the same channel
	routerChan := suite.provider.NextPipelineChan()
	require.NotNil(suite.T(), routerChan)

	numSenders := 5
	stopSending := make(chan struct{})
	var sendWg sync.WaitGroup

	for i := 0; i < numSenders; i++ {
		sendWg.Add(1)
		go func(senderID int) {
			defer sendWg.Done()
			msgCount := 0
			identifier := fmt.Sprintf("file:/var/log/sender-%d.log", senderID)
			for {
				select {
				case <-stopSending:
					return
				default:
					msg := createIntegrationTestMessage(fmt.Sprintf("sender-%d-msg-%d", senderID, msgCount), identifier)
					select {
					case routerChan <- msg:
						msgCount++
					case <-stopSending:
						return
					case <-time.After(100 * time.Millisecond):
					}
				}
			}
		}(i)
	}

	time.Sleep(200 * time.Millisecond)

	close(stopSending)
	sendWg.Wait()
	time.Sleep(50 * time.Millisecond)

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

	suite.provider = nil
}

// TestMixedTailerTypes tests file tailers and non-file tailers working together
func (suite *ProviderFailoverIntegrationSuite) TestMixedTailerTypes() {
	numFileTailers := 3
	numOtherTailers := 3
	messagesPerTailer := 50

	var wg sync.WaitGroup
	var totalSent atomic.Int64

	// File tailers (with monitor, which is nil when failover enabled)
	for i := 0; i < numFileTailers; i++ {
		wg.Add(1)
		go func(tailerID int) {
			defer wg.Done()

			routerChan, monitor := suite.provider.NextPipelineChanWithMonitor()
			require.NotNil(suite.T(), routerChan)
			suite.Nil(monitor, "Monitor should be nil with failover enabled")

			identifier := fmt.Sprintf("file:/var/log/file-tailer-%d.log", tailerID)
			for j := 0; j < messagesPerTailer; j++ {
				msg := createIntegrationTestMessage(fmt.Sprintf("file-tailer-%d-msg-%d", tailerID, j), identifier)
				routerChan <- msg
				totalSent.Add(1)
			}
		}(i)
	}

	// Other tailers (no monitor)
	for i := 0; i < numOtherTailers; i++ {
		wg.Add(1)
		go func(tailerID int) {
			defer wg.Done()

			routerChan := suite.provider.NextPipelineChan()
			require.NotNil(suite.T(), routerChan)

			identifier := fmt.Sprintf("container:/other-tailer-%d", tailerID)
			for j := 0; j < messagesPerTailer; j++ {
				msg := createIntegrationTestMessage(fmt.Sprintf("other-tailer-%d-msg-%d", tailerID, j), identifier)
				routerChan <- msg
				totalSent.Add(1)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	expectedTotal := int64((numFileTailers + numOtherTailers) * messagesPerTailer)
	suite.Equal(expectedTotal, totalSent.Load(), "All messages should be sent")
}

// TestRapidStartStopCycles tests for resource leaks during rapid lifecycle changes
func (suite *ProviderFailoverIntegrationSuite) TestRapidStartStopCycles() {
	for iteration := 0; iteration < 5; iteration++ {
		cfg := configmock.New(suite.T())
		cfg.SetWithoutSource("logs_config.pipeline_failover.enabled", true)
		cfg.SetWithoutSource("logs_config.message_channel_size", 5)

		endpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
			"use_http": true,
		})

		p := newProvider(
			2,
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

		routerChan := p.NextPipelineChan()
		for i := 0; i < 5; i++ {
			msg := createIntegrationTestMessage(fmt.Sprintf("iter-%d-msg-%d", iteration, i), "file:/test.log")
			select {
			case routerChan <- msg:
			case <-time.After(100 * time.Millisecond):
			}
		}

		p.Stop()
	}

	suite.True(true, "Rapid start/stop cycles completed without hanging")
}

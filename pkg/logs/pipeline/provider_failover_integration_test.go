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
	cfg.SetInTest("logs_config.pipeline_failover.enabled", true)
	cfg.SetInTest("logs_config.message_channel_size", 5)

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

func createIntegrationTestMessage(content string, identifier string) *message.Message {
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type: config.StringChannelType,
	})
	msg := message.NewMessageWithSource([]byte(content), "info", source, time.Now().UnixNano())
	msg.Origin.Identifier = identifier
	return msg
}

// TestConcurrentHighThroughput verifies that many concurrent tailers can send
// messages through router channels without data loss or deadlocks.
func (suite *ProviderFailoverIntegrationSuite) TestConcurrentHighThroughput() {
	numTailers := 10
	messagesPerTailer := 100

	var wg sync.WaitGroup
	var totalSent atomic.Int64

	for tailerID := 0; tailerID < numTailers; tailerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			routerChan := suite.provider.NextPipelineChan()
			require.NotNil(suite.T(), routerChan)

			identifier := fmt.Sprintf("file:/var/log/tailer-%d.log", id)
			for i := 0; i < messagesPerTailer; i++ {
				msg := createIntegrationTestMessage(fmt.Sprintf("tailer-%d-msg-%d", id, i), identifier)
				select {
				case routerChan <- msg:
					totalSent.Add(1)
				case <-time.After(5 * time.Second):
					suite.T().Errorf("Tailer %d timed out on message %d", id, i)
					return
				}
			}
		}(tailerID)
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	suite.Equal(int64(numTailers*messagesPerTailer), totalSent.Load(), "All messages should be sent")
}

// TestGracefulShutdownUnderConcurrentLoad verifies that Stop() completes cleanly
// even while multiple goroutines are actively sending messages.
func (suite *ProviderFailoverIntegrationSuite) TestGracefulShutdownUnderConcurrentLoad() {
	routerChan := suite.provider.NextPipelineChan()
	require.NotNil(suite.T(), routerChan)

	stopSending := make(chan struct{})
	var sendWg sync.WaitGroup

	for i := 0; i < 5; i++ {
		sendWg.Add(1)
		go func(senderID int) {
			defer sendWg.Done()
			identifier := fmt.Sprintf("file:/var/log/sender-%d.log", senderID)
			for {
				select {
				case <-stopSending:
					return
				default:
					msg := createIntegrationTestMessage("data", identifier)
					select {
					case routerChan <- msg:
					case <-stopSending:
						return
					case <-time.After(100 * time.Millisecond):
					}
				}
			}
		}(i)
	}

	time.Sleep(200 * time.Millisecond)

	// Stop senders first, then stop provider
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
		// Success
	case <-time.After(5 * time.Second):
		suite.Fail("Provider shutdown timed out")
	}

	suite.provider = nil
}

// TestRapidStartStopCycles verifies no resource leaks across repeated lifecycle changes.
func (suite *ProviderFailoverIntegrationSuite) TestRapidStartStopCycles() {
	for iteration := 0; iteration < 5; iteration++ {
		cfg := configmock.New(suite.T())
		cfg.SetInTest("logs_config.pipeline_failover.enabled", true)
		cfg.SetInTest("logs_config.message_channel_size", 5)

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
}

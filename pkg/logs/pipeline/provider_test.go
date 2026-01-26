// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"
)

// mockSenderFactory captures the configuration passed to NewSenderV2
type mockSenderFactory struct {
	queueCount      int
	workersPerQueue int
	minConcurrency  int
	maxConcurrency  int
	serverlessMeta  sender.ServerlessMeta
	isHTTP          bool
}

func newMockSenderFactory() *mockSenderFactory {
	return &mockSenderFactory{}
}

func (f *mockSenderFactory) NewTCPSender(
	_ pkgconfigmodel.Reader,
	_ sender.Sink,
	_ int,
	serverlessMeta sender.ServerlessMeta,
	_ *config.Endpoints,
	_ *client.DestinationsContext,
	_ statusinterface.Status,
	_ string,
	queueCount int,
	workersPerQueue int,
) *sender.Sender {
	f.queueCount = queueCount
	f.workersPerQueue = workersPerQueue
	f.minConcurrency = 1
	f.maxConcurrency = 1
	f.serverlessMeta = serverlessMeta
	f.isHTTP = false

	return &sender.Sender{}
}

func (f *mockSenderFactory) NewHTTPSender(
	_ pkgconfigmodel.Reader,
	_ sender.Sink,
	_ int,
	serverlessMeta sender.ServerlessMeta,
	_ *config.Endpoints,
	_ *client.DestinationsContext,
	_ string,
	_ string,
	_ string,
	queueCount int,
	workersPerQueue int,
	minWorkerConcurrency int,
	maxWorkerConcurrency int,
) *sender.Sender {
	f.queueCount = queueCount
	f.workersPerQueue = workersPerQueue
	f.minConcurrency = minWorkerConcurrency
	f.maxConcurrency = maxWorkerConcurrency
	f.serverlessMeta = serverlessMeta
	f.isHTTP = true

	return &sender.Sender{}
}

func TestProviderConfigurations(t *testing.T) {
	tests := []struct {
		name                   string
		useHTTP                bool
		legacyMode             bool
		numberOfPipelines      int
		serverless             bool
		expectedQueues         int
		expectedWorkers        int
		expectedMinConcurrency int
		expectedMaxConcurrency int
		batchMaxConcurrentSend int
	}{
		{
			name:                   "TCP sender default",
			useHTTP:                false,
			legacyMode:             false,
			numberOfPipelines:      3,
			serverless:             false,
			expectedQueues:         sender.DefaultQueuesCount, // 1
			expectedWorkers:        3,                         // numberOfPipelines
			expectedMinConcurrency: 1,
			expectedMaxConcurrency: 1,
			batchMaxConcurrentSend: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
		},
		{
			name:                   "TCP sender legacy",
			useHTTP:                false,
			legacyMode:             true,
			numberOfPipelines:      3,
			serverless:             false,
			expectedQueues:         3, // numberOfPipelines
			expectedWorkers:        1, // 1 worker per queue
			expectedMinConcurrency: 1,
			expectedMaxConcurrency: 1,
			batchMaxConcurrentSend: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
		},
		{
			name:                   "HTTP sender default",
			useHTTP:                true,
			legacyMode:             false,
			numberOfPipelines:      3,
			serverless:             false,
			expectedQueues:         sender.DefaultQueuesCount,     // 1
			expectedWorkers:        sender.DefaultWorkersPerQueue, // 1
			expectedMinConcurrency: 3,
			expectedMaxConcurrency: 30,
			batchMaxConcurrentSend: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
		},
		{
			name:                   "HTTP sender with batch_max_concurrent_send",
			useHTTP:                true,
			legacyMode:             false,
			numberOfPipelines:      3,
			serverless:             false,
			expectedQueues:         sender.DefaultQueuesCount,     // 1
			expectedWorkers:        sender.DefaultWorkersPerQueue, // 1
			expectedMinConcurrency: 24,
			expectedMaxConcurrency: 24,
			batchMaxConcurrentSend: 8,
		},
		{
			name:                   "Http sender legacy",
			useHTTP:                true,
			legacyMode:             true,
			numberOfPipelines:      3,
			serverless:             false,
			expectedQueues:         3, // numberOfPipelines
			expectedWorkers:        1, // 1 worker per queue
			expectedMinConcurrency: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
			expectedMaxConcurrency: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
			batchMaxConcurrentSend: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
		},
		{
			name:                   "Http sender legacy with batch_max_concurrent_send",
			useHTTP:                true,
			legacyMode:             true,
			numberOfPipelines:      3,
			serverless:             false,
			expectedQueues:         3, // numberOfPipelines
			expectedWorkers:        1, // 1 worker per queue
			expectedMinConcurrency: 8,
			expectedMaxConcurrency: 8,
			batchMaxConcurrentSend: 8,
		},
		{
			name:                   "Serverless default",
			useHTTP:                true,
			legacyMode:             false,
			numberOfPipelines:      2,
			serverless:             true,
			expectedQueues:         1, // 1 queue
			expectedWorkers:        2, // numberOfPipelines
			expectedMinConcurrency: 1,
			expectedMaxConcurrency: 1,
			batchMaxConcurrentSend: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
		},
		{
			name:                   "Serverless legacy",
			useHTTP:                true,
			legacyMode:             true,
			numberOfPipelines:      2,
			serverless:             true,
			expectedQueues:         2, // numberOfPipelines
			expectedWorkers:        1, // 1 workers per queue
			expectedMinConcurrency: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
			expectedMaxConcurrency: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
			batchMaxConcurrentSend: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			cfg := configmock.New(t)

			endpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
				"use_http":                  tc.useHTTP,
				"batch_max_concurrent_send": tc.batchMaxConcurrentSend,
			})

			mockFactory := newMockSenderFactory()
			originalHTTPFactory := httpSenderFactory
			originalTCPFactory := tcpSenderFactory
			httpSenderFactory = mockFactory.NewHTTPSender
			tcpSenderFactory = mockFactory.NewTCPSender
			defer func() {
				httpSenderFactory = originalHTTPFactory
				tcpSenderFactory = originalTCPFactory
			}()

			destinationsContext := &client.DestinationsContext{}
			diagnosticMessageReceiver := &diagnostic.BufferedMessageReceiver{}
			status := statusinterface.NewStatusProviderMock()
			compression := compressionfx.NewMockCompressor()

			providerImpl := NewProvider(
				tc.numberOfPipelines,
				&sender.NoopSink{},
				diagnosticMessageReceiver,
				nil, // processing rules
				endpoints,
				destinationsContext,
				status,
				nil, // hostname
				cfg,
				compression,
				tc.legacyMode,
				tc.serverless,
			)
			require.NotNil(t, providerImpl)

			// Verify sender configuration
			assert.Equal(t, tc.expectedQueues, mockFactory.queueCount, "incorrect queue count")
			assert.Equal(t, tc.expectedWorkers, mockFactory.workersPerQueue, "incorrect workers per queue")
			assert.Equal(t, tc.expectedMinConcurrency, mockFactory.minConcurrency, "incorrect min concurrency")
			assert.Equal(t, tc.expectedMaxConcurrency, mockFactory.maxConcurrency, "incorrect max concurrency")
			assert.Equal(t, tc.useHTTP, mockFactory.isHTTP, "incorrect sender type")

			if tc.serverless {
				assert.True(t, mockFactory.serverlessMeta.IsEnabled())
			} else {
				assert.False(t, mockFactory.serverlessMeta.IsEnabled())
			}
		})
	}
}

func TestPipelineChannelDistribution(t *testing.T) {
	tests := []struct {
		name              string
		numberOfPipelines int
		expectedChannels  int
	}{
		{
			name:              "single pipeline",
			numberOfPipelines: 1,
			expectedChannels:  1,
		},
		{
			name:              "multiple pipelines",
			numberOfPipelines: 3,
			expectedChannels:  3,
		},
		{
			name:              "many pipelines",
			numberOfPipelines: 10,
			expectedChannels:  10,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			cfg := configmock.New(t)
			endpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
				"use_http": true,
			})

			destinationsContext := &client.DestinationsContext{}
			diagnosticMessageReceiver := &diagnostic.BufferedMessageReceiver{}
			status := statusinterface.NewStatusProviderMock()
			compression := compressionfx.NewMockCompressor()

			providerImpl := NewProvider(
				tc.numberOfPipelines,
				&sender.NoopSink{},
				diagnosticMessageReceiver,
				nil, // processing rules
				endpoints,
				destinationsContext,
				status,
				nil, // hostname
				cfg,
				compression,
				false, // legacy mode
				false, // serverless
			)

			require.NotNil(t, providerImpl)
			p := providerImpl.(*provider)

			// Start provider and verify pipelines
			p.Start()
			assert.Equal(t, tc.numberOfPipelines, len(p.pipelines))

			// Test pipeline channel distribution
			seenChannels := make(map[chan *message.Message]struct{})
			for i := 0; i < tc.numberOfPipelines*2; i++ {
				ch := p.NextPipelineChan()
				require.NotNil(t, ch)
				seenChannels[ch] = struct{}{}
			}
			assert.Equal(t, tc.expectedChannels, len(seenChannels), "expected %d unique channels, got %d", tc.expectedChannels, len(seenChannels))

			// Test NextPipelineChanWithMonitor
			ch, monitor := p.NextPipelineChanWithMonitor()
			require.NotNil(t, ch)
			require.NotNil(t, monitor)

			// Cleanup
			p.Stop()
			assert.Empty(t, p.pipelines)
		})
	}
}

func TestCompressorPool(t *testing.T) {
	// Test CompressorPool creation and access
	poolSize := 5
	pool := NewCompressorPool(poolSize)

	require.NotNil(t, pool)
	assert.Equal(t, poolSize, pool.Size())

	// Test setting and getting compressors
	compression := compressionfx.NewMockCompressor()
	for i := 0; i < poolSize; i++ {
		compressor := compression.NewCompressor("none", 0)
		pool.SetCompressor(i, compressor)
		retrieved := pool.GetCompressor(i)
		assert.NotNil(t, retrieved)
		assert.Equal(t, compressor, retrieved)
	}

	// Test bounds checking
	assert.Nil(t, pool.GetCompressor(-1), "should return nil for negative index")
	assert.Nil(t, pool.GetCompressor(poolSize), "should return nil for index >= size")

	// Test that SetCompressor doesn't panic on out of bounds
	pool.SetCompressor(-1, compression.NewCompressor("none", 0))
	pool.SetCompressor(poolSize, compression.NewCompressor("none", 0))
}

func TestProviderCompressorPoolInitialization(t *testing.T) {
	tests := []struct {
		name              string
		numberOfPipelines int
		useHTTP           bool
		useCompression    bool
	}{
		{
			name:              "HTTP with compression",
			numberOfPipelines: 3,
			useHTTP:           true,
			useCompression:    true,
		},
		{
			name:              "HTTP without compression",
			numberOfPipelines: 3,
			useHTTP:           true,
			useCompression:    false,
		},
		{
			name:              "TCP without compression",
			numberOfPipelines: 3,
			useHTTP:           false,
			useCompression:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			cfg := configmock.New(t)
			endpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
				"use_http":        tc.useHTTP,
				"use_compression": tc.useCompression,
			})

			destinationsContext := &client.DestinationsContext{}
			diagnosticMessageReceiver := &diagnostic.BufferedMessageReceiver{}
			status := statusinterface.NewStatusProviderMock()
			compression := compressionfx.NewMockCompressor()

			providerImpl := NewProvider(
				tc.numberOfPipelines,
				&sender.NoopSink{},
				diagnosticMessageReceiver,
				nil, // processing rules
				endpoints,
				destinationsContext,
				status,
				nil, // hostname
				cfg,
				compression,
				false, // legacy mode
				false, // serverless
			)

			require.NotNil(t, providerImpl)
			p := providerImpl.(*provider)

			// Verify compressor pool is created
			require.NotNil(t, p.compressorPool, "compressor pool should be initialized")
			assert.Equal(t, tc.numberOfPipelines, p.compressorPool.Size(), "compressor pool size should match number of pipelines")

			// Start provider to initialize compressors
			p.Start()

			// Verify all compressors in the pool are initialized
			for i := 0; i < tc.numberOfPipelines; i++ {
				compressor := p.compressorPool.GetCompressor(i)
				assert.NotNil(t, compressor, "compressor at index %d should be initialized", i)
			}

			// Cleanup
			p.Stop()
		})
	}
}

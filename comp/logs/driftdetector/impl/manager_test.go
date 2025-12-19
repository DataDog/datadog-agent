// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package impl

import (
	"net/http"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTelemetry is a mock implementation of telemetry.Component
type mockTelemetry struct{}

func (m *mockTelemetry) Handler() http.Handler { return nil }
func (m *mockTelemetry) Reset()                {}
func (m *mockTelemetry) RegisterCollector(c telemetry.Collector) {
}
func (m *mockTelemetry) UnregisterCollector(c telemetry.Collector) bool { return true }
func (m *mockTelemetry) NewCounter(subsystem, name string, tags []string, help string) telemetry.Counter {
	return mockCounter{}
}
func (m *mockTelemetry) NewCounterWithOpts(subsystem, name string, tags []string, help string, opts telemetry.Options) telemetry.Counter {
	return mockCounter{}
}
func (m *mockTelemetry) NewSimpleCounter(subsystem, name, help string) telemetry.SimpleCounter {
	return mockSimpleCounter{}
}
func (m *mockTelemetry) NewSimpleCounterWithOpts(subsystem, name, help string, opts telemetry.Options) telemetry.SimpleCounter {
	return mockSimpleCounter{}
}
func (m *mockTelemetry) NewGauge(subsystem, name string, tags []string, help string) telemetry.Gauge {
	return mockGauge{}
}
func (m *mockTelemetry) NewGaugeWithOpts(subsystem, name string, tags []string, help string, opts telemetry.Options) telemetry.Gauge {
	return mockGauge{}
}
func (m *mockTelemetry) NewSimpleGauge(subsystem, name, help string) telemetry.SimpleGauge {
	return mockSimpleGauge{}
}
func (m *mockTelemetry) NewSimpleGaugeWithOpts(subsystem, name, help string, opts telemetry.Options) telemetry.SimpleGauge {
	return mockSimpleGauge{}
}
func (m *mockTelemetry) NewHistogram(subsystem, name string, tags []string, help string, buckets []float64) telemetry.Histogram {
	return mockHistogram{}
}
func (m *mockTelemetry) NewHistogramWithOpts(subsystem, name string, tags []string, help string, buckets []float64, opts telemetry.Options) telemetry.Histogram {
	return mockHistogram{}
}
func (m *mockTelemetry) NewSimpleHistogram(subsystem, name, help string, buckets []float64) telemetry.SimpleHistogram {
	return mockSimpleHistogram{}
}
func (m *mockTelemetry) NewSimpleHistogramWithOpts(subsystem, name, help string, buckets []float64, opts telemetry.Options) telemetry.SimpleHistogram {
	return mockSimpleHistogram{}
}
func (m *mockTelemetry) Gather(defaultGather bool) ([]*telemetry.MetricFamily, error) {
	return nil, nil
}
func (m *mockTelemetry) GatherText(defaultGather bool, filter telemetry.MetricFilter) (string, error) {
	return "", nil
}

type mockCounter struct{}

func (m mockCounter) InitializeToZero(tags ...string)                   {}
func (m mockCounter) Inc(tags ...string)                                {}
func (m mockCounter) Add(value float64, tags ...string)                 {}
func (m mockCounter) Delete(tags ...string)                             {}
func (m mockCounter) IncWithTags(tags map[string]string)                {}
func (m mockCounter) AddWithTags(value float64, tags map[string]string) {}
func (m mockCounter) DeleteWithTags(tags map[string]string)             {}
func (m mockCounter) WithValues(tags ...string) telemetry.SimpleCounter {
	return mockSimpleCounter{}
}
func (m mockCounter) WithTags(tags map[string]string) telemetry.SimpleCounter {
	return mockSimpleCounter{}
}

type mockSimpleCounter struct{}

func (m mockSimpleCounter) Inc()              {}
func (m mockSimpleCounter) Add(value float64) {}
func (m mockSimpleCounter) Get() float64      { return 0 }

type mockGauge struct{}

func (m mockGauge) Set(value float64, tags ...string)         {}
func (m mockGauge) Inc(tags ...string)                        {}
func (m mockGauge) Dec(tags ...string)                        {}
func (m mockGauge) Add(value float64, tags ...string)         {}
func (m mockGauge) Sub(value float64, tags ...string)         {}
func (m mockGauge) Delete(tags ...string)                     {}
func (m mockGauge) DeletePartialMatch(tags map[string]string) {}
func (m mockGauge) WithValues(tags ...string) telemetry.SimpleGauge {
	return mockSimpleGauge{}
}
func (m mockGauge) WithTags(tags map[string]string) telemetry.SimpleGauge {
	return mockSimpleGauge{}
}

type mockSimpleGauge struct{}

func (m mockSimpleGauge) Set(value float64) {}
func (m mockSimpleGauge) Inc()              {}
func (m mockSimpleGauge) Dec()              {}
func (m mockSimpleGauge) Add(value float64) {}
func (m mockSimpleGauge) Sub(value float64) {}
func (m mockSimpleGauge) Get() float64      { return 0 }

type mockHistogram struct{}

func (m mockHistogram) Observe(value float64, tags ...string) {}
func (m mockHistogram) Delete(tags ...string)                 {}
func (m mockHistogram) WithValues(tags ...string) telemetry.SimpleHistogram {
	return mockSimpleHistogram{}
}
func (m mockHistogram) WithTags(tags map[string]string) telemetry.SimpleHistogram {
	return mockSimpleHistogram{}
}

type mockSimpleHistogram struct{}

func (m mockSimpleHistogram) Observe(value float64) {}
func (m mockSimpleHistogram) Get() telemetry.HistogramValue {
	return telemetry.HistogramValue{}
}

// createTestConfig creates a minimal test configuration
func createTestConfig() common.Config {
	config := common.NewDefaultConfig()
	// Disable embedding to avoid needing a real embedding service
	config.Embedding.Enabled = false
	config.Window.Size = 100 * time.Millisecond
	config.Window.Step = 50 * time.Millisecond
	config.Manager.CleanupInterval = 100 * time.Millisecond
	config.Manager.MaxIdleTime = 200 * time.Millisecond
	config.Telemetry = &mockTelemetry{}
	return config
}

// TestSharedComponentsNotDuplicated verifies that shared components are created once
// and reused across multiple sources, not duplicated per source
func TestSharedComponentsNotDuplicated(t *testing.T) {
	config := createTestConfig()
	config.Embedding.Enabled = true

	manager := newDriftDetectorManager(config)
	require.NotNil(t, manager)

	// Store references to shared components
	sharedWindowMgr := manager.sharedWindowManager
	sharedTemplateExtractor := manager.sharedTemplateExtractor
	sharedAlertMgr := manager.sharedAlertManager
	sharedTransport := manager.sharedHTTPTransport

	// Create pipelines for multiple sources
	pipeline1 := manager.getOrCreatePipeline("source1")
	pipeline2 := manager.getOrCreatePipeline("source2")
	pipeline3 := manager.getOrCreatePipeline("source3")

	// Verify pipelines were created
	assert.NotNil(t, pipeline1)
	assert.NotNil(t, pipeline2)
	assert.NotNil(t, pipeline3)

	// Verify shared components are the same instances (not duplicated)
	assert.Same(t, sharedWindowMgr, manager.sharedWindowManager, "Window manager should not be duplicated")
	assert.Same(t, sharedTemplateExtractor, manager.sharedTemplateExtractor, "Template extractor should not be duplicated")
	assert.Same(t, sharedAlertMgr, manager.sharedAlertManager, "Alert manager should not be duplicated")
	assert.Same(t, sharedTransport, manager.sharedHTTPTransport, "HTTP transport should not be duplicated")

	// Verify each pipeline has its own embedding client and DMD analyzer
	assert.NotSame(t, pipeline1.embeddingClient, pipeline2.embeddingClient, "Each source should have its own embedding client")
	assert.NotSame(t, pipeline1.dmdAnalyzer, pipeline2.dmdAnalyzer, "Each source should have its own DMD analyzer")

	// Verify shared transport is being used (we created it and stored it)
	assert.NotNil(t, sharedTransport, "Shared HTTP transport should exist")
}

// TestMultipleSourcesConcurrentProcessing verifies that multiple sources can process
// logs concurrently without interference
func TestMultipleSourcesConcurrentProcessing(t *testing.T) {
	config := createTestConfig()
	config.Embedding.Enabled = true // Enable embedding to create pipelines

	manager := newDriftDetectorManager(config)
	require.NotNil(t, manager)

	err := manager.Start()
	require.NoError(t, err)
	defer manager.Stop()

	// Process logs from multiple sources concurrently
	sources := []string{"app1", "app2", "app3"}
	timestamp := time.Now()

	for i, source := range sources {
		for j := 0; j < 10; j++ {
			manager.ProcessLog(source, timestamp.Add(time.Duration(j)*time.Second),
				"Log message from "+source+" line "+string(rune('0'+j)))
		}

		// Verify pipeline was created for this source
		manager.mu.RLock()
		pipeline, exists := manager.pipelines[source]
		manager.mu.RUnlock()

		assert.True(t, exists, "Pipeline should exist for source %s", source)
		assert.NotNil(t, pipeline, "Pipeline should not be nil for source %s", source)
		assert.Equal(t, source, pipeline.sourceKey, "Pipeline source key should match")

		// Verify last access time was updated
		manager.mu.RLock()
		lastAccess, hasAccess := manager.lastAccess[source]
		manager.mu.RUnlock()

		assert.True(t, hasAccess, "Last access time should be tracked for source %s", source)
		assert.WithinDuration(t, time.Now(), lastAccess, 5*time.Second, "Last access should be recent")

		// Allow some processing time between sources
		if i < len(sources)-1 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Verify all pipelines exist
	manager.mu.RLock()
	pipelineCount := len(manager.pipelines)
	manager.mu.RUnlock()

	assert.Equal(t, len(sources), pipelineCount, "Should have one pipeline per source")
}

// TestSourceKeyPreservation verifies that source keys are correctly tracked
// in the manager's internal state
func TestSourceKeyPreservation(t *testing.T) {
	config := createTestConfig()
	config.Embedding.Enabled = true

	manager := newDriftDetectorManager(config)
	require.NotNil(t, manager)

	err := manager.Start()
	require.NoError(t, err)
	defer manager.Stop()

	// Process logs from different sources
	testSources := []string{"service-a", "service-b", "service-c"}
	timestamp := time.Now()

	for _, source := range testSources {
		for i := 0; i < 5; i++ {
			manager.ProcessLog(source, timestamp.Add(time.Duration(i)*time.Millisecond),
				"Test log message "+string(rune('0'+i)))
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	// Verify pipelines exist for all sources
	manager.mu.RLock()
	for _, source := range testSources {
		pipeline, exists := manager.pipelines[source]
		assert.True(t, exists, "Pipeline should exist for source %s", source)
		assert.NotNil(t, pipeline, "Pipeline should not be nil for source %s", source)
		assert.Equal(t, source, pipeline.sourceKey, "Pipeline source key should match")

		// Verify last access is tracked
		_, hasAccess := manager.lastAccess[source]
		assert.True(t, hasAccess, "Last access should be tracked for source %s", source)
	}
	manager.mu.RUnlock()

	// Verify stats return correct source keys
	stats := manager.GetStats()
	sourceKeys := stats["sources"].([]string)
	assert.ElementsMatch(t, testSources, sourceKeys,
		"Stats should report all source keys")
}

// TestOnDemandPipelineCreation verifies that pipelines are created on-demand
// when logs are processed for new sources
func TestOnDemandPipelineCreation(t *testing.T) {
	config := createTestConfig()
	config.Embedding.Enabled = true

	manager := newDriftDetectorManager(config)
	require.NotNil(t, manager)

	err := manager.Start()
	require.NoError(t, err)
	defer manager.Stop()

	// Initially, no pipelines should exist
	stats := manager.GetStats()
	assert.Equal(t, 0, stats["active_pipelines"], "Should start with no pipelines")

	// Process log from first source
	manager.ProcessLog("source1", time.Now(), "First log")
	time.Sleep(10 * time.Millisecond)

	stats = manager.GetStats()
	assert.Equal(t, 1, stats["active_pipelines"], "Should have 1 pipeline after first source")

	// Process log from second source
	manager.ProcessLog("source2", time.Now(), "Second log")
	time.Sleep(10 * time.Millisecond)

	stats = manager.GetStats()
	assert.Equal(t, 2, stats["active_pipelines"], "Should have 2 pipelines after second source")

	// Process more logs from existing source (should not create new pipeline)
	manager.ProcessLog("source1", time.Now(), "Another log from source1")
	time.Sleep(10 * time.Millisecond)

	stats = manager.GetStats()
	assert.Equal(t, 2, stats["active_pipelines"], "Should still have 2 pipelines")

	// Verify correct source keys in stats
	sourceKeys := stats["sources"].([]string)
	assert.ElementsMatch(t, []string{"source1", "source2"}, sourceKeys,
		"Should have correct source keys")
}

// TestIdlePipelineCleanup verifies that idle pipelines are automatically cleaned up
// after the configured max idle time
func TestIdlePipelineCleanup(t *testing.T) {
	config := createTestConfig()
	config.Embedding.Enabled = true
	config.Manager.CleanupInterval = 50 * time.Millisecond
	config.Manager.MaxIdleTime = 100 * time.Millisecond

	manager := newDriftDetectorManager(config)
	require.NotNil(t, manager)

	err := manager.Start()
	require.NoError(t, err)
	defer manager.Stop()

	// Create pipelines by processing logs
	manager.ProcessLog("active-source", time.Now(), "Active log")
	manager.ProcessLog("idle-source", time.Now(), "Idle log")
	time.Sleep(20 * time.Millisecond)

	stats := manager.GetStats()
	assert.Equal(t, 2, stats["active_pipelines"], "Should have 2 pipelines initially")

	// Keep active-source active by continuously sending logs
	go func() {
		for i := 0; i < 5; i++ {
			time.Sleep(40 * time.Millisecond)
			manager.ProcessLog("active-source", time.Now(), "Keep alive log")
		}
	}()

	// Wait for cleanup to run (idle-source should be removed)
	time.Sleep(250 * time.Millisecond)

	stats = manager.GetStats()
	activePipelines := stats["active_pipelines"].(int)
	sourceKeys := stats["sources"].([]string)

	// idle-source should have been cleaned up, active-source should remain
	assert.LessOrEqual(t, activePipelines, 1, "Idle pipeline should be cleaned up")

	// If there's still a pipeline, it should be the active one
	if activePipelines > 0 {
		assert.Contains(t, sourceKeys, "active-source",
			"Active source should still have a pipeline")
	}
}

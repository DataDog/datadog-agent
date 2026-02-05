// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package dogtelextension

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

// Mock implementations for components that don't have existing mocks or where custom mocks are simpler
type mockSerializer struct {
	serializer.MetricSerializer
	mock.Mock
}

func (m *mockSerializer) SendIterableSeries(serieSource metrics.SerieSource) error {
	args := m.Called(serieSource)
	return args.Error(0)
}

type mockHostname struct {
	hostnameinterface.Component
	mock.Mock
}

func (m *mockHostname) Get(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

type mockTagger struct {
	tagger.Component
	mock.Mock
}

type mockTelemetry struct {
	telemetry.Component
	mock.Mock
}

// factory holds mock components for testing (mirrors production factory)
type mockFactory struct {
	config     coreconfig.Component
	log        log.Component
	serializer serializer.MetricSerializer
	hostname   hostnameinterface.Component
	tagger     tagger.Component
	ipc        ipc.Component
	telemetry  telemetry.Component
}

func createMockComponents(t *testing.T, standaloneMode bool) *mockFactory {
	// Use existing mock for config with overrides
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"otel_standalone": standaloneMode,
	})

	return &mockFactory{
		config:     cfg,
		log:        logmock.New(t),
		serializer: &mockSerializer{},
		hostname:   &mockHostname{},
		tagger:     &mockTagger{},
		ipc:        ipcmock.New(t),
		telemetry:  &mockTelemetry{},
	}
}

func TestNewExtension_ValidConfig(t *testing.T) {
	ctx := context.Background()
	settings := extension.Settings{
		TelemetrySettings: component.TelemetrySettings{},
	}

	cfg := &Config{
		MetadataInterval:        300,
		EnableTaggerServer:      false,
		TaggerServerPort:        5000,
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	components := createMockComponents(t, true)

	f := &factory{
		config:     components.config,
		log:        components.log,
		serializer: components.serializer,
		hostname:   components.hostname,
		tagger:     components.tagger,
		ipc:        components.ipc,
		telemetry:  components.telemetry,
	}
	ext, err := newExtension(ctx, settings, cfg, f)
	require.NoError(t, err)
	require.NotNil(t, ext)

	dogtelExt, ok := ext.(*dogtelExtension)
	require.True(t, ok)
	assert.Equal(t, cfg, dogtelExt.config)
}

func TestNewExtension_InvalidConfig(t *testing.T) {
	ctx := context.Background()
	settings := extension.Settings{
		TelemetrySettings: component.TelemetrySettings{},
	}

	cfg := &Config{
		MetadataInterval:        -1, // Invalid
		TaggerServerPort:        5000,
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	components := createMockComponents(t, true)

	f := &factory{
		config:     components.config,
		log:        components.log,
		serializer: components.serializer,
		hostname:   components.hostname,
		tagger:     components.tagger,
		ipc:        components.ipc,
		telemetry:  components.telemetry,
	}
	ext, err := newExtension(ctx, settings, cfg, f)
	assert.Error(t, err)
	assert.Nil(t, ext)
	assert.Contains(t, err.Error(), "invalid configuration")
}

func TestExtension_Start_StandaloneMode(t *testing.T) {
	ctx := context.Background()
	settings := extension.Settings{
		TelemetrySettings: component.TelemetrySettings{},
	}

	cfg := &Config{
		MetadataInterval:        300,
		EnableTaggerServer:      false, // Disabled to simplify test
		TaggerServerPort:        0,
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	components := createMockComponents(t, true)

	// Setup mocks for metric submission
	mockHostname := components.hostname.(*mockHostname)
	mockHostname.On("Get", mock.Anything).Return("test-hostname", nil)

	mockSer := components.serializer.(*mockSerializer)
	mockSer.On("SendIterableSeries", mock.Anything).Return(nil)

	f := &factory{
		config:     components.config,
		log:        components.log,
		serializer: components.serializer,
		hostname:   components.hostname,
		tagger:     components.tagger,
		ipc:        components.ipc,
		telemetry:  components.telemetry,
	}
	ext, err := newExtension(ctx, settings, cfg, f)
	require.NoError(t, err)

	dogtelExt := ext.(*dogtelExtension)

	// Cleanup: Ensure the extension is shut down after the test
	t.Cleanup(func() {
		_ = dogtelExt.Shutdown(ctx)
	})

	// Start the extension
	err = ext.Start(ctx, nil)
	assert.NoError(t, err)
}

func TestExtension_Start_NotStandaloneMode(t *testing.T) {
	ctx := context.Background()
	settings := extension.Settings{
		TelemetrySettings: component.TelemetrySettings{},
	}

	cfg := &Config{
		MetadataInterval:        300,
		EnableTaggerServer:      false,
		TaggerServerPort:        0,
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	components := createMockComponents(t, false) // Not standalone

	f := &factory{
		config:     components.config,
		log:        components.log,
		serializer: components.serializer,
		hostname:   components.hostname,
		tagger:     components.tagger,
		ipc:        components.ipc,
		telemetry:  components.telemetry,
	}
	ext, err := newExtension(ctx, settings, cfg, f)
	require.NoError(t, err)

	// Start the extension
	err = ext.Start(ctx, nil)
	assert.NoError(t, err)
}

func TestExtension_Shutdown(t *testing.T) {
	ctx := context.Background()
	settings := extension.Settings{
		TelemetrySettings: component.TelemetrySettings{},
	}

	cfg := &Config{
		MetadataInterval:        300,
		EnableTaggerServer:      false,
		TaggerServerPort:        0,
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	components := createMockComponents(t, true)

	// Setup mocks for metric submission
	mockHostname := components.hostname.(*mockHostname)
	mockHostname.On("Get", mock.Anything).Return("test-hostname", nil)

	mockSer := components.serializer.(*mockSerializer)
	mockSer.On("SendIterableSeries", mock.Anything).Return(nil)

	f := &factory{
		config:     components.config,
		log:        components.log,
		serializer: components.serializer,
		hostname:   components.hostname,
		tagger:     components.tagger,
		ipc:        components.ipc,
		telemetry:  components.telemetry,
	}
	ext, err := newExtension(ctx, settings, cfg, f)
	require.NoError(t, err)

	// Start and shutdown
	err = ext.Start(ctx, nil)
	require.NoError(t, err)

	err = ext.(*dogtelExtension).Shutdown(ctx)
	assert.NoError(t, err)
}

func TestExtension_GetTaggerServerPort(t *testing.T) {
	ctx := context.Background()
	settings := extension.Settings{
		TelemetrySettings: component.TelemetrySettings{},
	}

	cfg := &Config{
		MetadataInterval:        300,
		EnableTaggerServer:      false,
		TaggerServerPort:        0,
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	components := createMockComponents(t, true)

	f := &factory{
		config:     components.config,
		log:        components.log,
		serializer: components.serializer,
		hostname:   components.hostname,
		tagger:     components.tagger,
		ipc:        components.ipc,
		telemetry:  components.telemetry,
	}
	ext, err := newExtension(ctx, settings, cfg, f)
	require.NoError(t, err)

	dogtelExt := ext.(*dogtelExtension)

	// Initially should be 0
	assert.Equal(t, 0, dogtelExt.GetTaggerServerPort())

	// Set a port
	dogtelExt.taggerServerPort = 5000
	assert.Equal(t, 5000, dogtelExt.GetTaggerServerPort())
}

func TestExtension_ShutdownWithoutStart(t *testing.T) {
	ctx := context.Background()
	settings := extension.Settings{
		TelemetrySettings: component.TelemetrySettings{},
	}

	cfg := &Config{
		MetadataInterval:        300,
		EnableTaggerServer:      false,
		TaggerServerPort:        0,
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	components := createMockComponents(t, true)

	f := &factory{
		config:     components.config,
		log:        components.log,
		serializer: components.serializer,
		hostname:   components.hostname,
		tagger:     components.tagger,
		ipc:        components.ipc,
		telemetry:  components.telemetry,
	}
	ext, err := newExtension(ctx, settings, cfg, f)
	require.NoError(t, err)

	// Shutdown without Start should not error
	err = ext.(*dogtelExtension).Shutdown(ctx)
	assert.NoError(t, err)
}

func TestExtension_SubmitRunningMetric(t *testing.T) {
	ctx := context.Background()
	settings := extension.Settings{
		TelemetrySettings: component.TelemetrySettings{},
	}

	cfg := &Config{
		MetadataInterval:        300,
		EnableTaggerServer:      false,
		TaggerServerPort:        0,
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	components := createMockComponents(t, true)

	// Setup mock hostname
	mockHostname := components.hostname.(*mockHostname)
	mockHostname.On("Get", mock.Anything).Return("test-hostname", nil)

	// Setup mock serializer
	mockSer := components.serializer.(*mockSerializer)
	mockSer.On("SendIterableSeries", mock.Anything).Return(nil)

	f := &factory{
		config:     components.config,
		log:        components.log,
		serializer: components.serializer,
		hostname:   components.hostname,
		tagger:     components.tagger,
		ipc:        components.ipc,
		telemetry:  components.telemetry,
	}
	ext, err := newExtension(ctx, settings, cfg, f)
	require.NoError(t, err)

	dogtelExt := ext.(*dogtelExtension)

	// Submit the metric
	dogtelExt.submitRunningMetric()

	// Verify hostname was called
	mockHostname.AssertCalled(t, "Get", mock.Anything)

	// Verify serializer was called
	mockSer.AssertCalled(t, "SendIterableSeries", mock.Anything)
}

func TestExtension_SubmitRunningMetric_HostnameError(t *testing.T) {
	ctx := context.Background()
	settings := extension.Settings{
		TelemetrySettings: component.TelemetrySettings{},
	}

	cfg := &Config{
		MetadataInterval:        300,
		EnableTaggerServer:      false,
		TaggerServerPort:        0,
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	components := createMockComponents(t, true)

	// Setup mock hostname to return error
	mockHostname := components.hostname.(*mockHostname)
	mockHostname.On("Get", mock.Anything).Return("", assert.AnError)

	// Setup mock serializer
	mockSer := components.serializer.(*mockSerializer)
	mockSer.On("SendIterableSeries", mock.Anything).Return(nil)

	f := &factory{
		config:     components.config,
		log:        components.log,
		serializer: components.serializer,
		hostname:   components.hostname,
		tagger:     components.tagger,
		ipc:        components.ipc,
		telemetry:  components.telemetry,
	}
	ext, err := newExtension(ctx, settings, cfg, f)
	require.NoError(t, err)

	dogtelExt := ext.(*dogtelExtension)

	// Submit the metric (should use "unknown" as hostname)
	dogtelExt.submitRunningMetric()

	// Verify hostname was called
	mockHostname.AssertCalled(t, "Get", mock.Anything)

	// Verify serializer was still called
	mockSer.AssertCalled(t, "SendIterableSeries", mock.Anything)
}

func TestExtension_SubmitRunningMetric_SerializerError(t *testing.T) {
	ctx := context.Background()
	settings := extension.Settings{
		TelemetrySettings: component.TelemetrySettings{},
	}

	cfg := &Config{
		MetadataInterval:        300,
		EnableTaggerServer:      false,
		TaggerServerPort:        0,
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	components := createMockComponents(t, true)

	// Setup mock hostname
	mockHostname := components.hostname.(*mockHostname)
	mockHostname.On("Get", mock.Anything).Return("test-hostname", nil)

	// Setup mock serializer to return error
	mockSer := components.serializer.(*mockSerializer)
	mockSer.On("SendIterableSeries", mock.Anything).Return(assert.AnError)

	f := &factory{
		config:     components.config,
		log:        components.log,
		serializer: components.serializer,
		hostname:   components.hostname,
		tagger:     components.tagger,
		ipc:        components.ipc,
		telemetry:  components.telemetry,
	}
	ext, err := newExtension(ctx, settings, cfg, f)
	require.NoError(t, err)

	dogtelExt := ext.(*dogtelExtension)

	// Submit the metric (should not panic despite error)
	dogtelExt.submitRunningMetric()

	// Verify serializer was called
	mockSer.AssertCalled(t, "SendIterableSeries", mock.Anything)
}

func TestExtension_StartMetricSubmission_DisabledInterval(t *testing.T) {
	ctx := context.Background()
	settings := extension.Settings{
		TelemetrySettings: component.TelemetrySettings{},
	}

	cfg := &Config{
		MetadataInterval:        0, // Disabled
		EnableTaggerServer:      false,
		TaggerServerPort:        0,
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	components := createMockComponents(t, true)

	f := &factory{
		config:     components.config,
		log:        components.log,
		serializer: components.serializer,
		hostname:   components.hostname,
		tagger:     components.tagger,
		ipc:        components.ipc,
		telemetry:  components.telemetry,
	}
	ext, err := newExtension(ctx, settings, cfg, f)
	require.NoError(t, err)

	dogtelExt := ext.(*dogtelExtension)

	// Start metric submission with disabled interval
	dogtelExt.startMetricSubmission()

	// Verify context is not created
	assert.Nil(t, dogtelExt.metricCtx)
	assert.Nil(t, dogtelExt.metricCancel)
}

func TestExtension_StartMetricSubmission_ValidInterval(t *testing.T) {
	ctx := context.Background()
	settings := extension.Settings{
		TelemetrySettings: component.TelemetrySettings{},
	}

	cfg := &Config{
		MetadataInterval:        1, // 1 second for faster test
		EnableTaggerServer:      false,
		TaggerServerPort:        0,
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	components := createMockComponents(t, true)

	// Setup mock hostname
	mockHostname := components.hostname.(*mockHostname)
	mockHostname.On("Get", mock.Anything).Return("test-hostname", nil)

	// Setup mock serializer
	mockSer := components.serializer.(*mockSerializer)
	mockSer.On("SendIterableSeries", mock.Anything).Return(nil)

	f := &factory{
		config:     components.config,
		log:        components.log,
		serializer: components.serializer,
		hostname:   components.hostname,
		tagger:     components.tagger,
		ipc:        components.ipc,
		telemetry:  components.telemetry,
	}
	ext, err := newExtension(ctx, settings, cfg, f)
	require.NoError(t, err)

	dogtelExt := ext.(*dogtelExtension)

	// Start metric submission
	dogtelExt.startMetricSubmission()

	// Verify context is created
	assert.NotNil(t, dogtelExt.metricCtx)
	assert.NotNil(t, dogtelExt.metricCancel)

	// Give the goroutine time to submit the immediate metric
	time.Sleep(50 * time.Millisecond)

	// Stop metric submission
	dogtelExt.stopMetricSubmission()

	// Verify serializer was called at least once (immediate submission)
	mockSer.AssertCalled(t, "SendIterableSeries", mock.Anything)
}

func TestExtension_StopMetricSubmission_NilCancel(t *testing.T) {
	ctx := context.Background()
	settings := extension.Settings{
		TelemetrySettings: component.TelemetrySettings{},
	}

	cfg := &Config{
		MetadataInterval:        300,
		EnableTaggerServer:      false,
		TaggerServerPort:        0,
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	components := createMockComponents(t, true)

	f := &factory{
		config:     components.config,
		log:        components.log,
		serializer: components.serializer,
		hostname:   components.hostname,
		tagger:     components.tagger,
		ipc:        components.ipc,
		telemetry:  components.telemetry,
	}
	ext, err := newExtension(ctx, settings, cfg, f)
	require.NoError(t, err)

	dogtelExt := ext.(*dogtelExtension)

	// Stop without start (should not panic)
	dogtelExt.stopMetricSubmission()
}

func TestExtension_StartAndShutdown_WithMetrics(t *testing.T) {
	ctx := context.Background()
	settings := extension.Settings{
		TelemetrySettings: component.TelemetrySettings{},
	}

	cfg := &Config{
		MetadataInterval:        1, // 1 second for faster test
		EnableTaggerServer:      false,
		TaggerServerPort:        0,
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	components := createMockComponents(t, true)

	// Setup mock hostname
	mockHostname := components.hostname.(*mockHostname)
	mockHostname.On("Get", mock.Anything).Return("test-hostname", nil)

	// Setup mock serializer
	mockSer := components.serializer.(*mockSerializer)
	mockSer.On("SendIterableSeries", mock.Anything).Return(nil)

	f := &factory{
		config:     components.config,
		log:        components.log,
		serializer: components.serializer,
		hostname:   components.hostname,
		tagger:     components.tagger,
		ipc:        components.ipc,
		telemetry:  components.telemetry,
	}
	ext, err := newExtension(ctx, settings, cfg, f)
	require.NoError(t, err)

	// Start the extension (should start metric submission)
	err = ext.Start(ctx, nil)
	require.NoError(t, err)

	dogtelExt := ext.(*dogtelExtension)

	// Verify metric submission started
	assert.NotNil(t, dogtelExt.metricCtx)
	assert.NotNil(t, dogtelExt.metricCancel)

	// Shutdown the extension (should stop metric submission)
	err = dogtelExt.Shutdown(ctx)
	assert.NoError(t, err)

	// Verify serializer was called
	mockSer.AssertCalled(t, "SendIterableSeries", mock.Anything)
}

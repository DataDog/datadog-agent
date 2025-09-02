// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp && test

package otlp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil"
	pkgconfigmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func TestGetComponents(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	_, err := getComponents(serializermock.NewMetricSerializer(t), make(chan *message.Message), fakeTagger, hostnameimpl.NewHostnameService(), nil)
	// No duplicate component
	require.NoError(t, err)
}

func AssertSucessfulRun(t *testing.T, pcfg PipelineConfig) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	p, err := NewPipeline(pcfg, serializermock.NewMetricSerializer(t), make(chan *message.Message), fakeTagger, hostnameimpl.NewHostnameService(), nil)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	colDone := make(chan struct{})
	go func() {
		defer close(colDone)
		require.NoError(t, p.Run(ctx))
	}()

	assert.Eventually(t, func() bool {
		return otelcol.StateRunning == p.col.GetState()
	}, time.Second*2, time.Millisecond*200)

	p.Stop()
	p.Stop()
	<-colDone

	assert.Eventually(t, func() bool {
		return otelcol.StateClosed == p.col.GetState()
	}, time.Second*2, time.Millisecond*200)
}

func AssertFailedRun(t *testing.T, pcfg PipelineConfig, expected string) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	p, err := NewPipeline(pcfg, serializermock.NewMetricSerializer(t), make(chan *message.Message), fakeTagger, hostnameimpl.NewHostnameService(), nil)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pipelineError := p.Run(ctx)
	assert.ErrorContains(t, pipelineError, expected)
}

func TestStartPipeline(t *testing.T) {
	cfg := pkgconfigmock.New(t)
	cfg.SetWithoutSource("hostname", "otlp-testhostname")

	pcfg := getTestPipelineConfig()
	AssertSucessfulRun(t, pcfg)
}

func TestStartPipelineFromConfig(t *testing.T) {
	cfg := pkgconfigmock.New(t)
	cfg.SetWithoutSource("hostname", "otlp-testhostname")

	tests := []struct {
		path string
		err  string
	}{
		{
			path: "receiver/noprotocols.yaml",
			err:  "invalid configuration: receivers::otlp: must specify at least one protocol when using the OTLP receiver",
		},
		{path: "receiver/simple.yaml"},
		{path: "receiver/advanced.yaml"},
		{
			path: "receiver/typo.yaml",
			err:  "'protocols' has invalid keys: htttp",
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.path, func(t *testing.T) {
			cfg, err := testutil.LoadConfig(t, "./testdata/"+testInstance.path)
			require.NoError(t, err)
			pcfg, err := FromAgentConfig(cfg)
			require.NoError(t, err)
			if testInstance.err == "" {
				AssertSucessfulRun(t, pcfg)
			} else {
				AssertFailedRun(t, pcfg, testInstance.err)
			}
		})
	}
}

func TestRecoverPanic(t *testing.T) {
	panicTest := func(v any) {
		defer recoverAndStoreError()
		panic(v)
	}
	require.NotPanics(t, func() {
		panicTest("this is a test")
	})
	assert.EqualError(t, pipelineError.Load(), "OTLP pipeline had a panic: this is a test")
}

// TestGetComponentsIoTOptimization tests that IoT agent builds exclude unnecessary components
func TestGetComponentsIoTOptimization(t *testing.T) {
	// Note: Since flavor.GetFlavor() is global state, this test verifies the current
	// behavior rather than mocking different flavors. The actual optimization logic
	// is tested through component presence/absence verification.

	// Test component creation to verify optimization logic works
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	logsChannel := make(chan *message.Message, 1)

	// Test the component creation
	factories, err := getComponents(
		serializermock.NewMetricSerializer(t),
		logsChannel,
		fakeTagger,
		hostnameimpl.NewHostnameService(),
		nil, // telemetry
	)
	require.NoError(t, err)

	// Verify that essential components are always present regardless of flavor
	assert.Contains(t, factories.Receivers, component.MustNewType("otlp"), "OTLP receiver should always be present")
	assert.Contains(t, factories.Processors, component.MustNewType("batch"), "Batch processor should always be present")
	assert.Contains(t, factories.Exporters, component.MustNewType("serializer"), "Serializer exporter should always be present")
	assert.Contains(t, factories.Exporters, component.MustNewType("otlp"), "OTLP exporter should always be present")

	// Test current flavor behavior
	currentFlavor := flavor.GetFlavor()
	isCurrentlyIoT := currentFlavor == flavor.IotAgent

	// Verify debug exporter behavior based on current flavor
	_, hasDebugExporter := factories.Exporters[component.MustNewType("debug")]
	if isCurrentlyIoT {
		assert.False(t, hasDebugExporter, "IoT agent should not have debug exporter")
		assert.Empty(t, factories.Extensions, "IoT agent should have empty extensions")
	} else {
		assert.True(t, hasDebugExporter, "Standard agent should have debug exporter")
		// Extensions might be empty if no extension factories are registered, so we don't assert on this
	}

	// Verify infraattributes processor behavior
	_, hasInfraAttributes := factories.Processors[component.MustNewType("infraattributes")]
	if isCurrentlyIoT {
		assert.False(t, hasInfraAttributes, "IoT agent should not have infraattributes processor by default")
	} else {
		assert.True(t, hasInfraAttributes, "Standard agent should have infraattributes processor when tagger is available")
	}
}

// TestGetComponentsWithoutTagger tests component creation when tagger is nil
func TestGetComponentsWithoutTagger(t *testing.T) {

	factories, err := getComponents(
		serializermock.NewMetricSerializer(t),
		make(chan *message.Message, 1),
		nil, // No tagger
		hostnameimpl.NewHostnameService(),
		nil, // telemetry
	)
	require.NoError(t, err)

	// Should not have infraattributes processor when tagger is nil
	_, hasInfraAttributes := factories.Processors[component.MustNewType("infraattributes")]
	assert.False(t, hasInfraAttributes, "Should not have infraattributes processor without tagger")

	// Should still have batch processor
	_, hasBatch := factories.Processors[component.MustNewType("batch")]
	assert.True(t, hasBatch, "Should always have batch processor")
}

// TestGetComponentsWithoutTelemetry tests component creation when telemetry is nil
func TestGetComponentsWithoutTelemetry(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	factories, err := getComponents(
		serializermock.NewMetricSerializer(t),
		make(chan *message.Message, 1),
		fakeTagger,
		hostnameimpl.NewHostnameService(),
		nil, // No telemetry
	)
	require.NoError(t, err)

	// Should succeed without telemetry component
	assert.NotNil(t, factories)
	assert.NotEmpty(t, factories.Exporters)
	assert.NotEmpty(t, factories.Processors)
	assert.NotEmpty(t, factories.Receivers)
}

// TestGetComponentsWithoutLogsChannel tests that logs exporter is conditional
func TestGetComponentsWithoutLogsChannel(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	factories, err := getComponents(
		serializermock.NewMetricSerializer(t),
		nil, // No logs channel
		fakeTagger,
		hostnameimpl.NewHostnameService(),
		nil, // telemetry
	)
	require.NoError(t, err)

	// Should not have logsagent exporter when channel is nil
	_, hasLogsAgent := factories.Exporters[component.MustNewType("logsagent")]
	assert.False(t, hasLogsAgent, "Should not have logsagent exporter without logs channel")

	// Should still have other exporters
	_, hasSerializer := factories.Exporters[component.MustNewType("serializer")]
	assert.True(t, hasSerializer, "Should have serializer exporter")
}

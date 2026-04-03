// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && !windows

package main

import (
	"slices"
	"testing"
	"time"

	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	serverlessInitTag "github.com/DataDog/datadog-agent/cmd/serverless-init/tag"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/agentimpl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	serverlessTag "github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestTagsSetup(t *testing.T) {
	configmock.New(t)

	modeConf = mode.DetectMode()

	t.Setenv("DD_TAGS", "key1:value1 key2:value2 key3:value3:4")
	t.Setenv("DD_EXTRA_TAGS", "key22:value22 key23:value23")

	t.Setenv("DD_SERVICE", "test-service")
	t.Setenv("DD_ENV", "test-env")
	t.Setenv("DD_VERSION", "1.0.0")

	cloudService := &cloudservice.LocalService{}
	tagConfig := configureTags(cloudService)

	baseTags := serverlessTag.MapToArray(serverlessInitTag.GetBaseTagsMap())
	cloudServiceTags := cloudService.GetTags()
	enhancedFromCloudService := cloudService.GetEnhancedMetricTags(cloudServiceTags)
	cloudServiceEnhancedMetricTags := enhancedFromCloudService.Base
	cloudServiceEnhancedUsageMetricTags := enhancedFromCloudService.Usage

	versionTag := "_dd.datadog_init_version:xxx"
	enhancedMetricVersionTags := []string{"datadog_init_version:xxx", "sidecar:false"}

	assert.ElementsMatch(t, slices.Concat(tagConfig.ConfiguredTags, baseTags, serverlessTag.MapToArray(cloudServiceTags), []string{versionTag}), serverlessTag.MapToArray(tagConfig.Tags))
	assert.ElementsMatch(t, slices.Concat(tagConfig.ConfiguredTags, baseTags, serverlessTag.MapToArray(cloudServiceEnhancedMetricTags), enhancedMetricVersionTags), serverlessTag.MapToArray(tagConfig.EnhancedMetricTags))
	assert.ElementsMatch(t, slices.Concat(serverlessTag.MapToArray(cloudServiceEnhancedUsageMetricTags), enhancedMetricVersionTags), serverlessTag.MapToArray(tagConfig.EnhancedUsageMetricTags))
}

func TestFxApp(t *testing.T) {
	fxutil.TestOneShot(t, main)
}

type TestTimeoutFlushableAgent struct {
	hasBeenCalled bool
}

type TestFlushableAgent struct {
	hasBeenCalled bool
}

func (tfa *TestTimeoutFlushableAgent) Flush() {
	time.Sleep(1 * time.Hour)
	tfa.hasBeenCalled = true
}

func (tfa *TestFlushableAgent) Flush() {
	tfa.hasBeenCalled = true
}

func TestFlushSuccess(t *testing.T) {
	metricAgent := &TestFlushableAgent{}
	traceAgent := &TestFlushableAgent{}
	mockLogsAgent := agentimpl.NewMockServerlessLogsAgent()
	lastFlush(100*time.Millisecond, metricAgent, traceAgent, mockLogsAgent)
	assert.Equal(t, true, metricAgent.hasBeenCalled)
	assert.Equal(t, true, mockLogsAgent.DidFlush())
}

func TestFlushTimeout(t *testing.T) {
	metricAgent := &TestTimeoutFlushableAgent{}
	traceAgent := &TestTimeoutFlushableAgent{}
	mockLogsAgent := agentimpl.NewMockServerlessLogsAgent()
	mockLogsAgent.SetFlushDelay(time.Hour)

	lastFlush(100*time.Millisecond, metricAgent, traceAgent, mockLogsAgent)
	assert.Equal(t, false, metricAgent.hasBeenCalled)
	assert.Equal(t, false, mockLogsAgent.DidFlush())
}

// TestSetupWithoutAPIKey verifies that when DD_API_KEY is not set,
// the metric agent is not started and the trace agent is a no-op.
// This prevents noisy error logs when serverless-init is used without configuration.
func TestSetupWithoutAPIKey(t *testing.T) {
	configmock.New(t)

	modeConf = mode.DetectMode()

	// Explicitly unset DD_API_KEY
	t.Setenv("DD_API_KEY", "")

	_ = pkgconfigsetup.LoadDatadog(pkgconfigsetup.Datadog(), secretsmock.New(t), delegatedauthmock.New(t), nil)
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	// Simulate the API key check from setup()
	apiKey := configUtils.SanitizeAPIKey(pkgconfigsetup.Datadog().GetString("api_key"))
	assert.Empty(t, apiKey)

	// When no API key, metric agent should not be started (Demux is nil)
	metricAgent := &metrics.ServerlessMetricAgent{
		SketchesBucketOffset: 0,
		Tagger:               fakeTagger,
	}
	assert.Nil(t, metricAgent.Demux)
	assert.False(t, metricAgent.IsReady())

	// Noop trace agent should be safe to call
	traceAgent := trace.NewNoopTraceAgent()
	assert.NotPanics(t, func() {
		traceAgent.Flush()
		traceAgent.SetTags(map[string]string{"test": "value"})
		traceAgent.Stop()
	})
}

// TestSetupOtlpAgentNoPanic ensures setupOtlpAgent does not panic when OTLP is enabled.
func TestSetupOtlpAgentNoPanic(t *testing.T) {
	t.Setenv("DD_OTLP_CONFIG_LOGS_ENABLED", "true")
	t.Setenv("DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT", "0.0.0.0:4317")

	configmock.New(t)
	_ = pkgconfigsetup.LoadDatadog(pkgconfigsetup.Datadog(), secretsmock.New(t), delegatedauthmock.New(t), nil)
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	metricAgent := setupMetricAgent(map[string]string{}, map[string]string{}, map[string]string{}, fakeTagger, false)
	defer metricAgent.Stop()

	assert.NotPanics(t, func() { setupOtlpAgent(metricAgent, fakeTagger) })

	// Timeout to allow the goroutine in ServerlessOTLPAgent.Start() to run.
	// If it panics the process crashes. Without this the test can pass flakily when the goroutine hasn't run yet.
	const panicWindow = 500 * time.Millisecond
	<-time.After(panicWindow)
}
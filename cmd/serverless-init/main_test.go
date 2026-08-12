// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	serverlessInitTag "github.com/DataDog/datadog-agent/cmd/serverless-init/tag"
	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	agentmock "github.com/DataDog/datadog-agent/comp/logs/agent/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkgmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics/metricstest"
	serverlessTag "github.com/DataDog/datadog-agent/pkg/serverless/tags"
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

func TestFlushLogsAgentSuccess(t *testing.T) {
	mockLogsAgent := agentmock.NewMockServerlessLogsAgent()
	flushLogsAgent(100*time.Millisecond, mockLogsAgent)
	assert.Equal(t, true, mockLogsAgent.DidFlush())
}

func TestFlushLogsAgentTimeout(t *testing.T) {
	mockLogsAgent := agentmock.NewMockServerlessLogsAgent()
	mockLogsAgent.SetFlushDelay(time.Hour)

	flushLogsAgent(100*time.Millisecond, mockLogsAgent)
	assert.Equal(t, false, mockLogsAgent.DidFlush())
}

func TestFlushLogsAgentNil(t *testing.T) {
	assert.NotPanics(t, func() {
		flushLogsAgent(100*time.Millisecond, nil)
	})
}

// TestSetupWithoutAPIKey verifies that when DD_API_KEY is not set, the metric
// agent is left as a bare struct (Demux nil) and reporting metrics through it
// is a safe no-op. sendMetricSample (in pkg/serverless/metrics/metric.go) takes
// the early-return path when Demux is nil, preventing noisy panics when
// serverless-init is used without configuration.
func TestSetupWithoutAPIKey(t *testing.T) {
	metricAgent := &metrics.ServerlessMetricAgent{}
	assert.NotPanics(t, func() {
		metricAgent.AddEnhancedMetric("enhanced.metric", 1.0, pkgmetrics.MetricSourceServerless, 0)
		metricAgent.AddLegacyEnhancedMetric("legacy.metric", 1.0, pkgmetrics.MetricSourceServerless)
	})
}

// TestSetupOtlpAgentNoPanic ensures setupOtlpAgent does not panic when OTLP is enabled.
func TestSetupOtlpAgentNoPanic(t *testing.T) {
	t.Setenv("DD_OTLP_CONFIG_LOGS_ENABLED", "true")
	t.Setenv("DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT", "0.0.0.0:4317")

	configmock.New(t)
	_ = pkgconfigsetup.LoadDatadog(pkgconfigsetup.Datadog(), secretsmock.New(t), delegatedauthmock.New(t), nil)
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	bundle := metricstest.New(t, fakeTagger)
	metricAgent := metrics.New(bundle.Demux, metrics.Tags{})

	assert.NotPanics(t, func() { setupOtlpAgent(metricAgent, fakeTagger) })

	// Timeout to allow the goroutine in ServerlessOTLPAgent.Start() to run.
	// If it panics the process crashes. Without this the test can pass flakily when the goroutine hasn't run yet.
	const panicWindow = 500 * time.Millisecond
	<-time.After(panicWindow)
}

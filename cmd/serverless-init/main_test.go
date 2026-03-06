// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	"github.com/DataDog/datadog-agent/comp/logs/agent/agentimpl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	serverlessTag "github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestTagsSetup(t *testing.T) {
	configmock.New(t)

	modeConf = mode.DetectMode()

	ddTagsEnv := "key1:value1 key2:value2 key3:value3:4"
	ddExtraTagsEnv := "key22:value22 key23:value23"
	t.Setenv("DD_TAGS", ddTagsEnv)
	t.Setenv("DD_EXTRA_TAGS", ddExtraTagsEnv)
	cloudService := &cloudservice.LocalService{}
	configuredTags, metricTags, enhancedMetricTags, enhancedMetricTagsAll := configureTags(cloudService)

	cloudServiceTags := cloudService.GetTags()
	cloudServiceEnhancedMetricTags, cloudServiceEnhancedMetricTagsHighCardinality := cloudService.GetEnhancedMetricTags(cloudServiceTags)

	assert.ElementsMatch(t, append(configuredTags, append(serverlessTag.MapToArray(cloudServiceTags), "_dd.datadog_init_version:xxx")...), serverlessTag.MapToArray(metricTags))
	assert.ElementsMatch(t, append(configuredTags, append(serverlessTag.MapToArray(cloudServiceEnhancedMetricTags), []string{"datadog_init_version:xxx", "sidecar:false"}...)...), serverlessTag.MapToArray(enhancedMetricTags))
	assert.ElementsMatch(t, append(configuredTags, append(serverlessTag.MapToArray(cloudServiceEnhancedMetricTags), append(serverlessTag.MapToArray(cloudServiceEnhancedMetricTagsHighCardinality), []string{"datadog_init_version:xxx", "sidecar:false"}...)...)...), serverlessTag.MapToArray(enhancedMetricTagsAll))
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

func TestEnhancedMetricsEnabledByDefault(t *testing.T) {
	enhancedMetricsEnabled := pkgconfigsetup.Datadog().GetBool("enhanced_metrics")
	assert.Equal(t, true, enhancedMetricsEnabled)
}

func TestEnhancedMetricsDisabledWithEnvVar(t *testing.T) {
	t.Setenv("DD_ENHANCED_METRICS_ENABLED", "false")
	enhancedMetricsEnabled := pkgconfigsetup.Datadog().GetBool("enhanced_metrics")
	assert.Equal(t, false, enhancedMetricsEnabled)
}

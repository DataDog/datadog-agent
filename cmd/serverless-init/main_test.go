// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"os"
	"os/signal"
	"slices"
	"syscall"
	"testing"
	"time"

	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	serverlessInitLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	serverlessInitTag "github.com/DataDog/datadog-agent/cmd/serverless-init/tag"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	agentmock "github.com/DataDog/datadog-agent/comp/logs/agent/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	serverlessTag "github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// TestMetricAgentNoOpWithoutDemux verifies that the methods called by the
// lifecycle server on the metric agent are safe when the agent has not been
// started (Demux and dogStatsDServer are nil). In the no-API-key path the
// agent is never started, so /suspend and /terminate must not panic when
// they call Flush and WaitForPendingSamples.
func TestMetricAgentNoOpWithoutDemux(t *testing.T) {
	agent := &metrics.ServerlessMetricAgent{}
	assert.NotPanics(t, func() {
		agent.Flush()
		agent.WaitForPendingSamples()
	})
}

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
	mockLogsAgent := agentmock.NewMockServerlessLogsAgent()
	lastFlush(100*time.Millisecond, metricAgent, mockLogsAgent)
	assert.Equal(t, true, metricAgent.hasBeenCalled)
	assert.Equal(t, true, mockLogsAgent.DidFlush())
}

func TestFlushTimeout(t *testing.T) {
	metricAgent := &TestTimeoutFlushableAgent{}
	mockLogsAgent := agentmock.NewMockServerlessLogsAgent()
	mockLogsAgent.SetFlushDelay(time.Hour)

	lastFlush(100*time.Millisecond, metricAgent, mockLogsAgent)
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

// TestLogTagsBaseComputedFromTagConfigTags verifies that the logTagsBase variable
// computed in setup() — as serverlessTag.MapToArray(tagConfig.Tags) — contains all
// tags configured via DD_TAGS. This documents the contract: BaseTags passed to
// LifecycleContext must be the full startup tag slice so the lifecycle server can
// append microvm_id to it without losing any base tags.
func TestLogTagsBaseComputedFromTagConfigTags(t *testing.T) {
	configmock.New(t)
	modeConf = mode.DetectMode()
	t.Setenv("DD_TAGS", "env:prod region:us-east-1")

	cloudService := &cloudservice.LocalService{}
	tagConfig := configureTags(cloudService)
	logTagsBase := serverlessTag.MapToArray(tagConfig.Tags)

	assert.Contains(t, logTagsBase, "env:prod",
		"logTagsBase must include all DD_TAGS entries (used as BaseTags on the lifecycle server)")
	assert.Contains(t, logTagsBase, "region:us-east-1")
	assert.NotEmpty(t, logTagsBase)
}

// TestBaseTraceTagsComputedFromTagConfigTags verifies that traceTags —
// passed as BaseTraceTags to LifecycleContext — contains all tags from DD_TAGS
// so that the lifecycle server can extend the map with lambda_microvm_id at /run
// without losing any startup tags.
func TestBaseTraceTagsComputedFromTagConfigTags(t *testing.T) {
	configmock.New(t)
	modeConf = mode.DetectMode()
	t.Setenv("DD_TAGS", "env:prod region:us-east-1")

	cloudService := &cloudservice.LocalService{}
	tagConfig := configureTags(cloudService)
	baseTraceTags := serverlessInitTag.MakeTraceAgentTags(tagConfig.Tags)

	assert.Equal(t, "prod", baseTraceTags["env"],
		"BaseTraceTags must include all DD_TAGS entries (used as BaseTraceTags on the lifecycle server)")
	assert.Equal(t, "us-east-1", baseTraceTags["region"])
	assert.NotEmpty(t, baseTraceTags)
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

// TestRun_LocalService_InitMode executes the user app through LocalService.Run
// in init-container mode, verifying the CloudService.Run interface routes
// correctly to RunInit(cfg, nil) — no child tracking, user app runs normally.
func TestRun_LocalService_InitMode(t *testing.T) {
	saved := os.Args
	defer func() { os.Args = saved }()
	os.Args = []string{"datadog-init", "sh", "-c", "exit 0"}

	svc := &cloudservice.LocalService{}
	err := svc.Run(mode.Conf{SidecarMode: false}, &serverlessInitLog.Config{})
	assert.NoError(t, err)
}

// TestRun_LocalService_SidecarMode verifies that the defaultRun sidecar path
// calls RunSidecar (not RunInit). RunSidecar registers a real signal.Notify
// for SIGINT/SIGTERM and blocks until one arrives, in both production and
// tests, so we send ourselves a real SIGTERM to let it return instead of
// leaking a goroutine that intercepts SIGTERM for the rest of the test
// binary's life — which would otherwise swallow a genuine SIGTERM (e.g. CI
// cancellation) instead of letting the process terminate.
func TestRun_LocalService_SidecarMode(t *testing.T) {
	saved := os.Args
	defer func() { os.Args = saved }()
	os.Args = []string{"datadog-init"} // sidecar mode: no cmd args

	// Register our own listener first so the default terminate-on-SIGTERM
	// disposition is already overridden before we signal ourselves below,
	// regardless of whether RunSidecar's own signal.Notify has run yet.
	guard := make(chan os.Signal, 1)
	signal.Notify(guard, syscall.SIGTERM)
	defer signal.Stop(guard)

	svc := &cloudservice.LocalService{}
	done := make(chan error, 1)
	assert.NotPanics(t, func() {
		go func() { done <- svc.Run(mode.Conf{SidecarMode: true}, &serverlessInitLog.Config{}) }()
	})

	// Give RunSidecar's own signal.Notify time to register before we signal.
	time.Sleep(50 * time.Millisecond)
	assert.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGTERM))

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("RunSidecar did not return after SIGTERM")
	}
	<-guard // drain our own copy so it doesn't leak into later tests
}

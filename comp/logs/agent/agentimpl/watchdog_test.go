// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package agentimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	logsmetrics "github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

func testSummary(processor, strategy, sender float64) logsmetrics.SaturationSummary {
	return logsmetrics.SaturationSummary{
		MaxFill5m: map[string]float64{
			logsmetrics.ProcessorTlmName: processor,
			logsmetrics.StrategyTlmName:  strategy,
			logsmetrics.SenderTlmName:    sender,
		},
	}
}

func testLimits() autoProfileLimits {
	return autoProfileLimits{
		baselinePipelines:   4,
		maxPipelines:        8,
		baselineConcurrency: 0,
	}
}

func TestDecideAutoProfileAction_StrategyPrefersCompression(t *testing.T) {
	summary := testSummary(0.1, 0.9, 0.1)
	current := autoProfileRuntimeValues{
		pipelines:              4,
		batchMaxConcurrentSend: 0,
		useCompression:         false,
		compressionKind:        "gzip",
		zstdCompressionLevel:   1,
		gzipCompressionLevel:   6,
	}

	action := decideAutoProfileAction(summary, current, testLimits())
	assert.Equal(t, "normalize_compression", action.name)
	assert.Equal(t, autoReasonStrategy, action.reason)
	assert.Equal(t, true, action.changes["logs_config.use_compression"])
	assert.Equal(t, "zstd", action.changes["logs_config.compression_kind"])
}

func TestDecideAutoProfileAction_StrategyThenPipelines(t *testing.T) {
	summary := testSummary(0.1, 0.9, 0.1)
	current := autoProfileRuntimeValues{
		pipelines:              4,
		batchMaxConcurrentSend: 0,
		useCompression:         true,
		compressionKind:        "zstd",
		zstdCompressionLevel:   1,
		gzipCompressionLevel:   6,
	}

	action := decideAutoProfileAction(summary, current, testLimits())
	assert.Equal(t, "increase_pipelines", action.name)
	assert.Equal(t, autoReasonStrategy, action.reason)
	assert.Equal(t, 5, action.changes["logs_config.pipelines"])
}

func TestDecideAutoProfileAction_SenderLadder(t *testing.T) {
	summary := testSummary(0.1, 0.1, 0.9)
	current := autoProfileRuntimeValues{
		pipelines:              4,
		batchMaxConcurrentSend: 5,
		useCompression:         true,
		compressionKind:        "zstd",
		zstdCompressionLevel:   1,
		gzipCompressionLevel:   6,
	}

	action := decideAutoProfileAction(summary, current, testLimits())
	assert.Equal(t, "increase_concurrency", action.name)
	assert.Equal(t, autoReasonSender, action.reason)
	assert.Equal(t, 10, action.changes["logs_config.batch_max_concurrent_send"])
}

func TestDecideAutoProfileAction_ProcessorIncreasesPipelines(t *testing.T) {
	summary := testSummary(0.9, 0.1, 0.1)
	current := autoProfileRuntimeValues{
		pipelines:              3,
		batchMaxConcurrentSend: 0,
		useCompression:         true,
		compressionKind:        "zstd",
		zstdCompressionLevel:   1,
		gzipCompressionLevel:   6,
	}

	action := decideAutoProfileAction(summary, current, testLimits())
	assert.Equal(t, "increase_pipelines", action.name)
	assert.Equal(t, autoReasonProcess, action.reason)
	assert.Equal(t, 4, action.changes["logs_config.pipelines"])
}

func TestDecideAutoProfileAction_RecoveryDownscales(t *testing.T) {
	summary := testSummary(0.1, 0.1, 0.1)
	current := autoProfileRuntimeValues{
		pipelines:              6,
		batchMaxConcurrentSend: 10,
		useCompression:         true,
		compressionKind:        "zstd",
		zstdCompressionLevel:   1,
		gzipCompressionLevel:   6,
	}

	action := decideAutoProfileAction(summary, current, testLimits())
	assert.Equal(t, "decrease_pipelines", action.name)
	assert.Equal(t, autoReasonRecovered, action.reason)
	assert.Equal(t, 5, action.changes["logs_config.pipelines"])

	current.pipelines = 4
	action = decideAutoProfileAction(summary, current, testLimits())
	assert.Equal(t, "decrease_concurrency", action.name)
	assert.Equal(t, 5, action.changes["logs_config.batch_max_concurrent_send"])
}

func TestWatchdogDecideGuardrails(t *testing.T) {
	cfg := configComponent.NewMockWithOverrides(t, map[string]interface{}{
		"api_key":                        "test",
		"logs_config.logs_agent_profile": "auto",
	})
	agent := &logAgent{config: cfg}
	w := newAutoProfileWatchdog(agent)

	now := time.Now()

	w.startTime = now.Add(-30 * time.Second)
	_, skip := w.decide(now)
	assert.Equal(t, "warmup", skip)

	w.startTime = now.Add(-2 * time.Minute)
	w.cooldownUntil = now.Add(1 * time.Minute)
	_, skip = w.decide(now)
	assert.Equal(t, "cooldown", skip)

	w.cooldownUntil = time.Time{}
	w.applyHistory = []time.Time{
		now.Add(-10 * time.Minute),
		now.Add(-20 * time.Minute),
		now.Add(-30 * time.Minute),
	}
	_, skip = w.decide(now)
	assert.Equal(t, "budget", skip)
}

func TestClearAutoProfileRuntimeOverrides(t *testing.T) {
	cfg := configComponent.NewMockWithOverrides(t, map[string]interface{}{
		"api_key": "test",
	})
	cfg.Set("logs_config.pipelines", 7, pkgconfigmodel.SourceAgentRuntime)
	cfg.Set("logs_config.batch_max_concurrent_send", 10, pkgconfigmodel.SourceAgentRuntime)
	cfg.Set("logs_config.compression_kind", "zstd", pkgconfigmodel.SourceAgentRuntime)

	agent := &logAgent{config: cfg}
	cleared, err := agent.clearAutoProfileRuntimeOverrides()
	assert.NoError(t, err)
	assert.True(t, cleared)
	assert.NotEqual(t, pkgconfigmodel.SourceAgentRuntime, cfg.GetSource("logs_config.pipelines"))
	assert.NotEqual(t, pkgconfigmodel.SourceAgentRuntime, cfg.GetSource("logs_config.batch_max_concurrent_send"))
	assert.NotEqual(t, pkgconfigmodel.SourceAgentRuntime, cfg.GetSource("logs_config.compression_kind"))
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
)

func newMockCfg(t *testing.T) coreconfig.Component {
	t.Helper()
	return coreconfig.NewMockWithOverrides(t, map[string]any{"api_key": "test"})
}

// 1. No profile → system default returned for PipelinesCount.
func TestNoProfile_PipelinesCount(t *testing.T) {
	cfg := newMockCfg(t)
	// System default is min(4, GOMAXPROCS). Don't pin the exact value; just confirm no panic
	// and that the result matches cfg.GetInt directly.
	assert.Equal(t, cfg.GetInt("logs_config.pipelines"), PipelinesCount(cfg))
}

// 2. low_resource, no user override → pipelines=1, channels halved.
func TestLowResource_Defaults(t *testing.T) {
	cfg := newMockCfg(t)
	cfg.SetWithoutSource("logs_config.logs_agent_profile", "low_resource")

	assert.Equal(t, 1, PipelinesCount(cfg))
	assert.Equal(t, 50, MessageChannelSize(cfg))
	assert.Equal(t, 5, PayloadChannelSize(cfg))
}

// 3. low_resource + explicit pipelines=3 → user wins.
func TestLowResource_ExplicitPipelines(t *testing.T) {
	cfg := newMockCfg(t)
	cfg.SetWithoutSource("logs_config.logs_agent_profile", "low_resource")
	cfg.SetWithoutSource("logs_config.pipelines", 3)

	assert.Equal(t, 3, PipelinesCount(cfg))
	// channel sizes still come from profile since user didn't override them
	assert.Equal(t, 50, MessageChannelSize(cfg))
	assert.Equal(t, 5, PayloadChannelSize(cfg))
}

// 4. wan_optimized → batchMaxConcurrentSend=20.
func TestWanOptimized_BatchMaxConcurrentSend(t *testing.T) {
	cfg := newMockCfg(t)
	cfg.SetWithoutSource("logs_config.logs_agent_profile", "wan_optimized")

	lck := defaultLogsConfigKeys(cfg)
	assert.Equal(t, 20, lck.batchMaxConcurrentSend())
}

// 5. wan_optimized + explicit batch_max_concurrent_send=5 → 5.
func TestWanOptimized_ExplicitBatchMaxConcurrentSend(t *testing.T) {
	cfg := newMockCfg(t)
	cfg.SetWithoutSource("logs_config.logs_agent_profile", "wan_optimized")
	cfg.SetWithoutSource("logs_config.batch_max_concurrent_send", 5)

	lck := defaultLogsConfigKeys(cfg)
	assert.Equal(t, 5, lck.batchMaxConcurrentSend())
}

// 6. Unknown profile name → system default + no panic (warning logged).
func TestUnknownProfile_FallsThrough(t *testing.T) {
	cfg := newMockCfg(t)
	cfg.SetWithoutSource("logs_config.logs_agent_profile", "does_not_exist")

	assert.Equal(t, cfg.GetInt("logs_config.pipelines"), PipelinesCount(cfg))
	assert.Equal(t, cfg.GetInt("logs_config.message_channel_size"), MessageChannelSize(cfg))
	assert.Equal(t, cfg.GetInt("logs_config.payload_channel_size"), PayloadChannelSize(cfg))
}

// auto is a runtime mode and should not apply static profile overrides.
func TestAutoProfile_FallsThrough(t *testing.T) {
	cfg := newMockCfg(t)
	cfg.SetWithoutSource("logs_config.logs_agent_profile", AutoLogsAgentProfile)

	assert.True(t, IsAutoProfileEnabled(cfg))
	assert.Equal(t, cfg.GetInt("logs_config.pipelines"), PipelinesCount(cfg))
	assert.Equal(t, cfg.GetInt("logs_config.message_channel_size"), MessageChannelSize(cfg))
	assert.Equal(t, cfg.GetInt("logs_config.payload_channel_size"), PayloadChannelSize(cfg))
}

func TestIsAutoProfileEnabled(t *testing.T) {
	cfg := newMockCfg(t)
	assert.False(t, IsAutoProfileEnabled(cfg))

	cfg.SetWithoutSource("logs_config.logs_agent_profile", AutoLogsAgentProfile)
	assert.True(t, IsAutoProfileEnabled(cfg))
}

// 7. bandwidth_saver → gzip kind, level 9.
func TestBandwidthSaver_CompressionKindAndLevel(t *testing.T) {
	cfg := newMockCfg(t)
	cfg.SetWithoutSource("logs_config.logs_agent_profile", "bandwidth_saver")

	lck := defaultLogsConfigKeys(cfg)
	assert.Equal(t, GzipCompressionKind, lck.compressionKind())
	assert.Equal(t, 9, lck.compressionLevel())
}

// 8. max_throughput → useCompression=false.
func TestMaxThroughput_UseCompression(t *testing.T) {
	cfg := newMockCfg(t)
	cfg.SetWithoutSource("logs_config.logs_agent_profile", "max_throughput")

	lck := defaultLogsConfigKeys(cfg)
	assert.False(t, lck.useCompression())
}

// 9. performance → pipelines=GOMAXPROCS.
func TestPerformance_Pipelines(t *testing.T) {
	cfg := newMockCfg(t)
	cfg.SetWithoutSource("logs_config.logs_agent_profile", "performance")

	assert.Equal(t, runtime.GOMAXPROCS(0), PipelinesCount(cfg))
}

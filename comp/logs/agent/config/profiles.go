// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"runtime"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// logsAgentProfile holds optional overrides for logs agent tuning settings.
// A nil pointer field means "no override; use the system default."
type logsAgentProfile struct {
	pipelines              *int
	batchMaxConcurrentSend *int
	useCompression         *bool
	compressionKind        *string
	gzipCompressionLevel   *int
	zstdCompressionLevel   *int
	messageChannelSize     *int
	payloadChannelSize     *int
}

func ptr[T any](v T) *T { return &v }

// profiles is the registry of named performance profiles.
// Fields left as nil fall through to the system default.
var profiles = map[string]*logsAgentProfile{
	// balanced: all defaults — present for self-documenting explicitness.
	"balanced": {},

	// wan_optimized: more concurrent HTTP requests to hide WAN latency.
	"wan_optimized": {
		batchMaxConcurrentSend: ptr(20),
	},

	// max_throughput: adds no-compression to wan_optimized; CPU freed, relies on fast network.
	"max_throughput": {
		batchMaxConcurrentSend: ptr(20),
		useCompression:         ptr(false),
	},

	// low_resource: single pipeline, halved channel buffers; saves goroutines + memory.
	"low_resource": {
		pipelines:          ptr(1),
		messageChannelSize: ptr(50),
		payloadChannelSize: ptr(5),
	},

	// bandwidth_saver: gzip level 9 maximises compression ratio (trades CPU for bandwidth).
	"bandwidth_saver": {
		compressionKind:      ptr(GzipCompressionKind),
		gzipCompressionLevel: ptr(9),
	},

	// performance: all CPUs as pipelines + high send concurrency; for high-ingestion hosts.
	"performance": {
		pipelines:              ptr(runtime.GOMAXPROCS(0)),
		batchMaxConcurrentSend: ptr(20),
	},
}

// getActiveProfile returns the profile selected by logs_config.logs_agent_profile, or nil
// when the profile is empty/unset ("" or "balanced" both result in no overrides).
// An unknown profile name emits a warning and returns nil.
func getActiveProfile(cfg pkgconfigmodel.Reader) *logsAgentProfile {
	name := cfg.GetString("logs_config.logs_agent_profile")
	if name == "" {
		return nil
	}
	p, ok := profiles[name]
	if !ok {
		log.Warnf("Unknown logs_agent_profile %q; ignoring. Valid values: balanced, wan_optimized, max_throughput, low_resource, bandwidth_saver, performance", name)
		return nil
	}
	return p
}

// PipelinesCount returns the number of log processing pipelines to use.
// The active profile is consulted when the user has not explicitly set the key.
func PipelinesCount(cfg pkgconfigmodel.Reader) int {
	const key = "logs_config.pipelines"
	if cfg.IsConfigured(key) {
		return cfg.GetInt(key)
	}
	if p := getActiveProfile(cfg); p != nil && p.pipelines != nil {
		return *p.pipelines
	}
	return cfg.GetInt(key)
}

// MessageChannelSize returns the size of the per-pipeline message channel.
// The active profile is consulted when the user has not explicitly set the key.
func MessageChannelSize(cfg pkgconfigmodel.Reader) int {
	const key = "logs_config.message_channel_size"
	if cfg.IsConfigured(key) {
		return cfg.GetInt(key)
	}
	if p := getActiveProfile(cfg); p != nil && p.messageChannelSize != nil {
		return *p.messageChannelSize
	}
	return cfg.GetInt(key)
}

// PayloadChannelSize returns the size of the payload channel used by senders.
// The active profile is consulted when the user has not explicitly set the key.
func PayloadChannelSize(cfg pkgconfigmodel.Reader) int {
	const key = "logs_config.payload_channel_size"
	if cfg.IsConfigured(key) {
		return cfg.GetInt(key)
	}
	if p := getActiveProfile(cfg); p != nil && p.payloadChannelSize != nil {
		return *p.payloadChannelSize
	}
	return cfg.GetInt(key)
}

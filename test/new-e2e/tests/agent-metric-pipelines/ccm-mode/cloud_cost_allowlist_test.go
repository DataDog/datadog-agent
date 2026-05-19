// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ccmmode

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// defaultCloudCostAllowlistedMetrics mirrors integration.cloud_cost_only.metrics in
// pkg/config/setup/common_settings.go. Update both when changing the default allowlist.
var defaultCloudCostAllowlistedMetrics = []string{
	"kubernetes.cpu.usage.total",
	"kubernetes.memory.usage",
	"kubernetes_state.pod.uptime",
	"gpu.gr_engine_active",
	"aws.ebs.volume_read_bytes",
	"aws.ebs.volume_write_bytes",
	"aws.ebs.volume_read_ops",
	"aws.ebs.volume_write_ops",
	"kubernetes.kubelet.volume.stats.used_bytes",
	"kubernetes.kubelet.volume.stats.available_bytes",
	"system.cpu.user",
	"system.mem.pct_usable",
	"system.net.bytes_rcvd",
	"system.net.bytes_sent",
}

// ec2CloudCostAllowlistedMetrics are default-allowlist metrics emitted by core checks on awshost.
func ec2CloudCostAllowlistedMetrics() []string {
	var metrics []string
	for _, name := range defaultCloudCostAllowlistedMetrics {
		if strings.HasPrefix(name, "system.") {
			metrics = append(metrics, name)
		}
	}
	return metrics
}

func TestDefaultCloudCostAllowlistMatchesAgentConfig(t *testing.T) {
	cfg := configmock.New(t)
	assert.Equal(t, defaultCloudCostAllowlistedMetrics,
		cfg.GetStringSlice("integration.cloud_cost_only.metrics"),
		"update defaultCloudCostAllowlistedMetrics when changing the agent default allowlist")
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package allowlist holds shared cloud_cost_only metric allowlist defaults.
package allowlist

// DefaultCloudCostMetrics is the built-in integration metric allowlist for
// infrastructure_mode=cloud_cost_only. Used when integration.cloud_cost_only.metrics
// is unset and as the agent config default in pkg/config/setup/common_settings.go.
var DefaultCloudCostMetrics = []string{
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

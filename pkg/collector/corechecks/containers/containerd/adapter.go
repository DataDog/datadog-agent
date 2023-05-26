// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

var metricsNameMapping = map[string]string{
	"container.uptime":                         "containerd.uptime",
	"container.cpu.usage":                      "containerd.cpu.total",
	"container.cpu.user":                       "containerd.cpu.user",
	"container.cpu.system":                     "containerd.cpu.system",
	"container.cpu.throttled":                  "containerd.cpu.throttled.time",
	"container.cpu.throttled.periods":          "containerd.cpu.throttled.periods",
	"container.cpu.limit":                      "containerd.cpu.limit",
	"container.memory.usage":                   "containerd.mem.current.usage",
	"container.memory.kernel":                  "containerd.mem.kernel.usage",
	"container.memory.limit":                   "containerd.mem.current.limit",
	"container.memory.soft_limit":              "", // Not present in legacy check
	"container.memory.rss":                     "containerd.mem.rss",
	"container.memory.cache":                   "containerd.mem.cache",
	"container.memory.swap":                    "containerd.mem.swap.usage",
	"container.memory.oom_events":              "containerd.mem.current.failcnt",
	"container.memory.working_set":             "containerd.mem.private_working_set",
	"container.memory.commit":                  "containerd.mem.commit",
	"container.memory.commit.peak":             "containerd.mem.commit_peak",
	"container.io.read":                        "", // Remapping requires retagging, handled in extension
	"container.io.read.operations":             "", // Remapping requires retagging, handled in extension
	"container.io.write":                       "", // Remapping requires retagging, handled in extension
	"container.io.write.operations":            "", // Remapping requires retagging, handled in extension
	"container.pid.thread_count":               "",
	"container.pid.thread_limit":               "",
	"container.pid.open_files":                 "containerd.proc.open_fds",
	"container.net.sent":                       "",                                         // Not present in legacy check
	"container.net.rcvd":                       "",                                         // Not present in legacy check
	"containerd.blkio.service_recursive_bytes": "containerd.blkio.service_recursive_bytes", // Passthrough for custom metrics extension
	"containerd.blkio.serviced_recursive":      "containerd.blkio.serviced_recursive",      // Passthrough for custom metrics extension
}

// metricsAdapter implements the generic.MetricsAdapter interface
type metricsAdapter struct{}

// AdaptTags can be used to change Tagger tags before submitting the metrics
func (a metricsAdapter) AdaptTags(tags []string, c *workloadmeta.Container) []string {
	return append(tags, "runtime:containerd")
}

// AdaptMetrics can be used to change metrics (change name or value) before submitting the metric.
func (a metricsAdapter) AdaptMetrics(metricName string, value float64) (string, float64) {
	return metricsNameMapping[metricName], value
}

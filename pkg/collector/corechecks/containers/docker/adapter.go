// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

var metricsNameMapping = map[string]string{
	"container.uptime":                "docker.uptime",
	"container.cpu.usage":             "docker.cpu.usage",
	"container.cpu.user":              "docker.cpu.user",
	"container.cpu.system":            "docker.cpu.system",
	"container.cpu.throttled":         "docker.cpu.throttled.time",
	"container.cpu.throttled.periods": "docker.cpu.throttled",
	"container.cpu.limit":             "docker.cpu.limit",
	"container.memory.usage":          "", // Not present in legacy Docker check
	"container.memory.kernel":         "docker.kmem.usage",
	"container.memory.limit":          "docker.mem.limit",
	"container.memory.soft_limit":     "docker.mem.soft_limit",
	"container.memory.rss":            "docker.mem.rss",
	"container.memory.cache":          "docker.mem.cache",
	"container.memory.swap":           "docker.mem.swap",
	"container.memory.oom_events":     "docker.mem.failed_count",
	"container.memory.working_set":    "docker.mem.private_working_set",
	"container.memory.commit":         "docker.mem.commit_bytes",
	"container.memory.commit.peak":    "docker.mem.commit_peak_bytes",
	"container.io.read":               "docker.io.read_bytes",
	"container.io.read.operations":    "docker.io.read_operations",
	"container.io.write":              "docker.io.write_bytes",
	"container.io.write.operations":   "docker.io.write_operations",
	"container.pid.thread_count":      "docker.thread.count",
	"container.pid.thread_limit":      "docker.thread.limit",
	"container.pid.open_files":        "docker.container.open_fds",
	"container.net.sent":              "",                      // Removed, handled by custom network extension
	"container.net.rcvd":              "",                      // Removed, handled by custom network extension
	"docker.net.bytes_sent":           "docker.net.bytes_sent", // Passthrough for custom network extension
	"docker.net.bytes_rcvd":           "docker.net.bytes_rcvd", // Passthrough for custom network extension
	"docker.mem.in_use":               "docker.mem.in_use",     // Passthrough for custom metrics extension
	"docker.cpu.shares":               "docker.cpu.shares",     // Passthrough for custom metrics extension
	"docker.mem.sw_limit":             "docker.mem.sw_limit",   // Passthrough for custom metrics extension
	"docker.mem.rss":                  "docker.mem.rss",        // Passthrough for custom metrics extension
}

var metricsValuesConverter = map[string]func(float64) float64{
	"container.cpu.usage":     generic.ConvertNanosecondsToHz,
	"container.cpu.user":      generic.ConvertNanosecondsToHz,
	"container.cpu.system":    generic.ConvertNanosecondsToHz,
	"container.cpu.throttled": generic.ConvertNanosecondsToHz,
	"container.cpu.limit":     generic.ConvertNanosecondsToHz,
}

// metricsAdapter implements the generic.MetricsAdapter interface
type metricsAdapter struct{}

// AdaptTags can be used to change Tagger tags before submitting the metrics
func (a metricsAdapter) AdaptTags(tags []string, c *workloadmeta.Container) []string {
	return append(tags, "runtime:docker")
}

// AdaptMetrics can be used to change metrics (change name or value) before submitting the metric.
func (a metricsAdapter) AdaptMetrics(metricName string, value float64) (string, float64) {
	if convertFunc, found := metricsValuesConverter[metricName]; found {
		value = convertFunc(value)
	}
	metricName = metricsNameMapping[metricName]

	return metricName, value
}

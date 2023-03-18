// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cri

package cri

import (
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

var metricsNameMapping = map[string]string{
	"container.uptime":       "cri.uptime",
	"container.cpu.usage":    "cri.cpu.usage",
	"container.memory.usage": "cri.mem.rss",
	"cri.disk.used":          "cri.disk.used",   // Passthrough for custom metrics extension
	"cri.disk.inodes":        "cri.disk.inodes", // Passthrough for custom metrics extension
}

// metricsAdapter implements the generic.MetricsAdapter interface
type metricsAdapter struct{}

// AdaptTags can be used to change Tagger tags before submitting the metrics
func (a metricsAdapter) AdaptTags(tags []string, c *workloadmeta.Container) []string {
	return append(tags, "runtime:"+string(c.Runtime))
}

// AdaptMetrics can be used to change metrics (change name or value) before submitting the metric.
func (a metricsAdapter) AdaptMetrics(metricName string, value float64) (string, float64) {
	return metricsNameMapping[metricName], value
}

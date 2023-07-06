// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux_bpf

package ebpf

type DebugFsStatCollector struct{}

func NewDebugFsStatCollector() *DebugFsStatCollector {
	return &DebugFsStatCollector{}
}

// Describe returns all descriptions of the collector
func (c *DebugFsStatCollector) Describe(ch chan<- *prometheus.Desc) {}

// Collect returns the current state of all metrics of the collector
func (c *DebugFsStatCollector) Collect(ch chan<- prometheus.Metric) {}

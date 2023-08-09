// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux_bpf

package ebpf

import "github.com/prometheus/client_golang/prometheus"

// DebugFsStatCollector exported type should have comment or be unexported
type DebugFsStatCollector struct{}

// NewDebugFsStatCollector exported function should have comment or be unexported
func NewDebugFsStatCollector() *DebugFsStatCollector {
	return &DebugFsStatCollector{}
}

// Describe returns all descriptions of the collector
func (c *DebugFsStatCollector) Describe(chan<- *prometheus.Desc) {}

// Collect returns the current state of all metrics of the collector
func (c *DebugFsStatCollector) Collect(chan<- prometheus.Metric) {}

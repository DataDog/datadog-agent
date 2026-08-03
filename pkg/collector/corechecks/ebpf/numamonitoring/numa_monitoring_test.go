// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package numamonitoring

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/numamonitoring/model"
)

func pointer(value float64) *float64 { return &value }

func TestSubmitCapabilityDependentMetricSubset(t *testing.T) {
	metricSender := mocksender.NewMockSender("numa-monitoring")
	metricSender.SetupAcceptAll()
	stats := model.ContainerStats{
		RuntimeShares:     map[int]float64{0: 0.75},
		ResidentBytes:     map[int]uint64{0: 4096},
		PlacementMismatch: pointer(0.2),
		BadnessScore:      pointer(0.2),
		Domains: []model.DomainStats{{
			Domain:         "00",
			LLCOccupancy:   pointer(1024),
			TotalBandwidth: pointer(2048),
		}},
	}
	tags := []string{"container_id:abc"}
	submitStats(metricSender, stats, tags)

	metricSender.AssertMetric(t, "Gauge", "system.numa.cpu.runtime_share", 0.75, "", []string{"container_id:abc", "numa_node:0"})
	metricSender.AssertMetric(t, "Gauge", "system.numa.memory.resident", 4096, "", []string{"container_id:abc", "numa_node:0"})
	metricSender.AssertMetric(t, "Gauge", "system.numa.cache.llc_occupancy", 1024, "", []string{"container_id:abc", "resctrl_domain:00"})
	metricSender.AssertMetric(t, "Gauge", "system.numa.memory.bandwidth.total", 2048, "", []string{"container_id:abc", "resctrl_domain:00"})
	metricSender.AssertMetricMissing(t, "Gauge", "system.numa.memory.bandwidth.local")
	metricSender.AssertMetricMissing(t, "Gauge", "system.numa.memory.bandwidth.remote_estimated")
	metricSender.AssertMetricMissing(t, "Gauge", "system.numa.remote_ratio")
}

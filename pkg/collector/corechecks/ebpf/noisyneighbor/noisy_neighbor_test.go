// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package noisyneighbor

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor/model"
)

func TestSubmitPMUMetricsUsesCountsAndContainerTags(t *testing.T) {
	sender := mocksender.NewMockSender(t, id.ID("noisy_neighbor"))
	sender.SetupAcceptAll()
	tags := []string{"container_id:abc", "container_name:workload"}
	stat := model.NoisyNeighborStats{
		Cycles:           1,
		Instructions:     2,
		CacheMisses:      3,
		CacheReferences:  4,
		ITLBMisses:       5,
		BranchMisses:     6,
		CPUMigrations:    7,
		SampledEventMask: model.HardwareEventMask | model.EventCPUMigrations,
	}

	(&NoisyNeighborCheck{}).submitPMUMetrics(sender, stat, tags)

	sender.AssertMetric(t, "Count", "noisy_neighbor.cycles", 1, "", tags)
	sender.AssertMetric(t, "Count", "noisy_neighbor.instructions", 2, "", tags)
	sender.AssertMetric(t, "Count", "noisy_neighbor.cache_misses", 3, "", tags)
	sender.AssertMetric(t, "Count", "noisy_neighbor.cache_references", 4, "", tags)
	sender.AssertMetric(t, "Count", "noisy_neighbor.itlb_misses", 5, "", tags)
	sender.AssertMetric(t, "Count", "noisy_neighbor.branch_misses", 6, "", tags)
	sender.AssertMetric(t, "Count", "noisy_neighbor.cpu_migrations", 7, "", tags)
	sender.AssertNumberOfCalls(t, "Count", 7)
}

func TestSubmitPMUMetricsOmitsUnsampledEvents(t *testing.T) {
	sender := mocksender.NewMockSender(t, id.ID("noisy_neighbor"))
	sender.SetupAcceptAll()
	stat := model.NoisyNeighborStats{
		Cycles:           11,
		Instructions:     22,
		SampledEventMask: model.EventCycles,
	}

	(&NoisyNeighborCheck{}).submitPMUMetrics(sender, stat, []string{"container_id:abc"})

	sender.AssertNumberOfCalls(t, "Count", 1)
	sender.AssertMetric(t, "Count", "noisy_neighbor.cycles", 11, "", []string{"container_id:abc"})
}

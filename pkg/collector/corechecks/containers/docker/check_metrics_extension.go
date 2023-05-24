// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

type dockerCustomMetricsExtension struct {
	sender    generic.SenderFunc
	aggSender aggregator.Sender
}

func (dn *dockerCustomMetricsExtension) PreProcess(sender generic.SenderFunc, aggSender aggregator.Sender) {
	dn.sender = sender
	dn.aggSender = aggSender
}

func (dn *dockerCustomMetricsExtension) Process(tags []string, container *workloadmeta.Container, collector metrics.Collector, cacheValidity time.Duration) {
	// Duplicate call with generic.Processor, but cache should allow for a fast response.
	// We only need it for PIDs
	containerStats, err := collector.GetContainerStats(container.Namespace, container.ID, cacheValidity)
	if err != nil {
		log.Debugf("Gathering container metrics for container: %v failed, metrics may be missing, err: %v", container, err)
		return
	}

	if containerStats == nil {
		log.Debugf("Metrics provider returned nil stats for container: %v", container)
		return
	}

	if containerStats.Memory != nil {
		// Re-implement Docker check behaviour: PrivateWorkingSet is mapped to RSS
		if containerStats.Memory.PrivateWorkingSet != nil {
			dn.sender(dn.aggSender.Gauge, "docker.mem.rss", containerStats.Memory.PrivateWorkingSet, tags)
		}

		if containerStats.Memory.SwapLimit != nil {
			dn.sender(dn.aggSender.Gauge, "docker.mem.sw_limit", containerStats.Memory.SwapLimit, tags)
		}

		if containerStats.Memory.Limit != nil && *containerStats.Memory.Limit > 0 {
			if containerStats.Memory.RSS != nil {
				memoryPct := *containerStats.Memory.RSS / *containerStats.Memory.Limit
				dn.sender(dn.aggSender.Gauge, "docker.mem.in_use", &memoryPct, tags)
			} else if containerStats.Memory.CommitBytes != nil {
				memoryPct := *containerStats.Memory.CommitBytes / *containerStats.Memory.Limit
				dn.sender(dn.aggSender.Gauge, "docker.mem.in_use", &memoryPct, tags)
			}
		}
	}

	if containerStats.CPU != nil {
		dn.sender(dn.aggSender.Gauge, "docker.cpu.shares", containerStats.CPU.Shares, tags)
	}
}

// PostProcess is called once during each check run, after all calls to `Process`
func (dn *dockerCustomMetricsExtension) PostProcess() {
	// Nothing to do here
}

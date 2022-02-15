// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package docker

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
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

func (dn *dockerCustomMetricsExtension) Process(tags []string, container *workloadmeta.Container, collector provider.Collector, cacheValidity time.Duration) {
	// Duplicate call with generic.Processor, but cache should allow for a fast response.
	// We only need it for PIDs
	containerStats, err := collector.GetContainerStats(container.ID, cacheValidity)
	if err != nil {
		log.Debugf("Gathering container metrics for container: %v failed, metrics may be missing, err: %w", container, err)
		return
	}

	if containerStats.Memory != nil && containerStats.Memory.UsageTotal != nil && containerStats.Memory.Limit != nil && *containerStats.Memory.Limit > 0 {
		memoryPct := *containerStats.Memory.UsageTotal / *containerStats.Memory.Limit
		dn.sender(dn.aggSender.Gauge, "docker.mem.in_use", &memoryPct, tags)
	}

	if containerStats.CPU != nil {
		dn.sender(dn.aggSender.Gauge, "docker.cpu.shares", containerStats.CPU.Shares, tags)
	}
}

// PostProcess is called once during each check run, after all calls to `Process`
func (dn *dockerCustomMetricsExtension) PostProcess() {
	// Nothing to do here
}

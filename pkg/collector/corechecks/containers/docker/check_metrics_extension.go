// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"math"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type dockerCustomMetricsExtension struct {
	sender    generic.SenderFunc
	aggSender sender.Sender
}

func (dn *dockerCustomMetricsExtension) PreProcess(sender generic.SenderFunc, aggSender sender.Sender) {
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
		// Things to note about the "docker.cpu.shares" metric:
		// - In cgroups v1 the metric that indicates how to share CPUs when
		// there's contention is called shares. In v2 it's weight.
		// - The idea is the same. The value of CPU shares and weight doesn't
		// have any meaning by itself, it needs to be compared against the
		// shares/weight of the other containers of the same host.
		// - CPU shares and weight have different default values and different
		// range of valid values. The range for shares is [2,262144], for weight
		// it is [1,10000].
		// - Even when using cgroups v2, the "docker run" command only accepts
		// cpu shares as a parameter. "docker inspect" also shows shares. The
		// formulas used to convert between shares and weights are these:
		// https://github.com/kubernetes/kubernetes/blob/release-1.28/pkg/kubelet/cm/cgroup_manager_linux.go#L565
		// - Because docker shows shares everywhere regardless of the cgroup
		// version and "docker.cpu.shares" is a docker-specific metric, we think
		// that it is less confusing to always report shares to match what
		// the docker client reports.
		// - "docker inspect" reports 0 shares when the container is created
		// without specifying the number of shares. When that's the case, the
		// default applies: 1024 for shares and 100 for weight.
		// - The value emitted by the check is not exactly the same as in
		// Docker because of the rounding applied in the conversions. Example:
		//   - Run a container with 2048 shares in a system with cgroups v2.
		//   - The 2048 shares are converted to weight in cgroups v2:
		//     weight = (((shares - 2) * 9999) / 262142) + 1 = 79.04 (cgroups rounds to 79)
		//   - This check converts the weight to shares again to report the same as in docker:
		//     shares = (((weight - 1) * 262142) / 9999) + 2 = 2046.91 (will be rounded to 2047, instead of the original 2048).

		var cpuShares float64
		if containerStats.CPU.Shares != nil {
			cpuShares = *containerStats.CPU.Shares
		} else if containerStats.CPU.Weight != nil {
			cpuShares = math.Round(cpuWeightToCPUShares(*containerStats.CPU.Weight))
		}

		// 0 is not a valid value for shares. cpuShares == 0 means that we
		// couldn't collect the number of shares.
		if cpuShares != 0 {
			dn.sender(dn.aggSender.Gauge, "docker.cpu.shares", &cpuShares, tags)
		}
	}
}

// PostProcess is called once during each check run, after all calls to `Process`
func (dn *dockerCustomMetricsExtension) PostProcess(tagger.Component) {
	// Nothing to do here
}

// From https://github.com/kubernetes/kubernetes/blob/release-1.28/pkg/kubelet/cm/cgroup_manager_linux.go#L571
func cpuWeightToCPUShares(cpuWeight float64) float64 {
	return (((cpuWeight - 1) * 262142) / 9999) + 2
}

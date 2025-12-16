// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"context"
	"math"
	"time"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// cpuSharesWeightMapping represents the formula used to convert between
// cgroup v1 CPU shares and cgroup v2 CPU weight.
type cpuSharesWeightMapping int

const (
	// mappingUnknown indicates the mapping hasn't been detected yet
	mappingUnknown cpuSharesWeightMapping = iota
	// mappingLinear is the old linear mapping from Kubernetes/runc < 1.3.2
	// Formula: weight = 1 + ((shares - 2) * 9999) / 262142
	mappingLinear
	// mappingNonLinear is the new quadratic mapping from runc >= 1.3.2
	// Reference: https://github.com/opencontainers/runc/pull/4785
	mappingNonLinear
)

type dockerCustomMetricsExtension struct {
	sender    generic.SenderFunc
	aggSender sender.Sender

	// mapping tracks which CPU shares<->weight conversion formula the runtime uses.
	// It's detected lazily on the first container with enough data.
	mapping cpuSharesWeightMapping
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
		// formulas used to convert between shares and weights depend on the
		// runtime version:
		//   - runc < 1.3.2 / crun < 1.23: linear mapping (old Kubernetes formula)
		//     https://github.com/kubernetes/kubernetes/blob/release-1.28/pkg/kubelet/cm/cgroup_manager_linux.go#L565
		//   - runc >= 1.3.2 / crun >= 1.23: quadratic mapping
		//     https://github.com/opencontainers/runc/pull/4785
		// - We detect which mapping is in use by comparing the actual weight
		// with expected values computed from Docker's configured shares.
		// - The value emitted by the check with the old linear formula is not
		// exactly the same as in Docker because of the rounding applied in
		// the conversions. Example:
		//   - Run a container with 2048 shares in a system with cgroups v2.
		//   - The 2048 shares are converted to weight:
		//     weight = (((shares - 2) * 9999) / 262142) + 1 = 79.04 (rounds to 79)
		//   - This check converts the weight back to shares:
		//     shares = (((weight - 1) * 262142) / 9999) + 2 = 2046.91 (rounds to 2047)
		// - Because docker shows shares everywhere regardless of the cgroup
		// version and "docker.cpu.shares" is a docker-specific metric, we think
		// that it is less confusing to always report shares to match what
		// the docker client reports.
		// - "docker inspect" reports 0 shares when the container is created
		// without specifying the number of shares. When that's the case, the
		// default applies: 1024 for shares and 100 for weight.

		var cpuShares float64
		if containerStats.CPU.Shares != nil {
			// we have the logical shares value directly from cgroups v1.
			//
			// Cgroup v1 CPU shares has a range of [2^1...2^18], i.e. [2...262144],
			// and the default value is 1024.
			cpuShares = *containerStats.CPU.Shares
		} else if containerStats.CPU.Weight != nil {
			// cgroups v2: we only have weight, need to convert back to shares.
			// First, try to detect the mapping if we haven't already.
			// Cgroup v2 CPU weight has a range of [10^0...10^4], i.e. [1...10000],
			// and the default value is 100.
			if dn.mapping == mappingUnknown {
				dn.detectMapping(container.ID, *containerStats.CPU.Weight)
			}

			weight := *containerStats.CPU.Weight
			switch dn.mapping {
			case mappingLinear:
				// Old mapping
				cpuShares = math.Round(cpuWeightToSharesLinear(weight))
			case mappingNonLinear:
				// New mapping
				cpuShares = math.Round(cpuWeightToSharesNonLinear(weight))
			default:
				// Cannot determine mapping, don't emit potentially wrong metric
				return
			}
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

// detectMapping attempts to detect which CPU shares<->weight mapping formula
// the container runtime is using by comparing the actual weight from cgroups
// with expected values computed from Docker's configured shares.
func (dn *dockerCustomMetricsExtension) detectMapping(containerID string, actualWeight float64) {
	if actualWeight == 0 {
		return // Can't detect without a valid weight
	}

	du, err := docker.GetDockerUtil()
	if err != nil {
		log.Debugf("docker check: couldn't get docker util for mapping detection: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	inspect, err := du.Inspect(ctx, containerID, false)
	if err != nil {
		log.Debugf("docker check: couldn't inspect container %s for mapping detection: %v", containerID, err)
		return
	}

	if inspect.HostConfig == nil {
		return
	}

	configuredShares := uint64(inspect.HostConfig.CPUShares)
	// Docker returns 0 when shares weren't explicitly set, meaning "use default" (1024)
	if configuredShares == 0 {
		configuredShares = 1024
	}

	weight := uint64(actualWeight)
	expectedLinear := cpuSharesToWeightLinear(configuredShares)
	expectedNonLinear := cpuSharesToWeightNonLinear(configuredShares)

	// Use tolerance of ±1 to handle rounding edge cases
	matchesLinear := absDiff(weight, expectedLinear) <= 1
	matchesNonLinear := absDiff(weight, expectedNonLinear) <= 1

	switch {
	case matchesLinear && !matchesNonLinear:
		dn.mapping = mappingLinear
		log.Debugf("docker check: detected linear (old) shares<->weight mapping (shares=%d, weight=%d)", configuredShares, weight)
	case matchesNonLinear && !matchesLinear:
		dn.mapping = mappingNonLinear
		log.Debugf("docker check: detected non-linear (new) shares<->weight mapping (shares=%d, weight=%d)", configuredShares, weight)
	default:
		// Ambiguous or unknown runtime - don't set mapping, will retry detection.
		// This avoids emitting potentially wrong metrics.
		log.Debugf("docker check: couldn't determine shares<->weight mapping (shares=%d, weight=%d, expectedLinear=%d, expectedNonLinear=%d), will retry",
			configuredShares, weight, expectedLinear, expectedNonLinear)
	}
}

// cpuSharesToWeightLinear converts CPU shares to weight using the old linear
// formula from Kubernetes/runc < 1.3.2.
// Reference: https://github.com/kubernetes/kubernetes/blob/release-1.28/pkg/kubelet/cm/cgroup_manager_linux.go#L565
func cpuSharesToWeightLinear(cpuShares uint64) uint64 {
	if cpuShares < 2 {
		cpuShares = 2
	} else if cpuShares > 262144 {
		cpuShares = 262144
	}
	return 1 + ((cpuShares-2)*9999)/262142
}

// cpuSharesToWeightNonLinear converts CPU shares to weight using the new
// quadratic formula from runc >= 1.3.2 / crun >= 1.23.
// This formula ensures min, max, and default values all map correctly:
//   - shares=2 (min) -> weight=1 (min)
//   - shares=1024 (default) -> weight=100 (default)
//   - shares=262144 (max) -> weight=10000 (max)
//
// Reference: https://github.com/opencontainers/runc/pull/4785
func cpuSharesToWeightNonLinear(cpuShares uint64) uint64 {
	if cpuShares == 0 {
		return 0
	}
	if cpuShares <= 2 {
		return 1
	}
	if cpuShares >= 262144 {
		return 10000
	}
	l := math.Log2(float64(cpuShares))
	exponent := (l*l+125*l)/612.0 - 7.0/34.0
	return uint64(math.Ceil(math.Pow(10, exponent)))
}

// cpuWeightToSharesLinear converts CPU weight to shares using the inverse of
// the old linear formula from Kubernetes/runc < 1.3.2.
func cpuWeightToSharesLinear(cpuWeight float64) float64 {
	if cpuWeight <= 0 {
		return 0
	}
	return (((cpuWeight - 1) * 262142) / 9999) + 2
}

// cpuWeightToSharesNonLinear converts CPU weight to shares using the inverse
// of the quadratic formula from runc >= 1.3.2.
// Forward: l = log2(shares); exponent = (l² + 125l) / 612 - 7/34; weight = ceil(10^exponent)
// (reference: https://github.com/opencontainers/cgroups/blob/fd95216684463f30144d5f5e41b6f54528feedee/utils.go#L425-L441)
// Inverse: solve quadratic l² + 125l - 612*(exponent + 7/34) = 0
// We use geometric mean sqrt((weight-1)*weight) to estimate the original 10^exponent
// value before ceil() was applied.
func cpuWeightToSharesNonLinear(cpuWeight float64) float64 {
	if cpuWeight <= 0 {
		return 0
	}
	if cpuWeight <= 1 {
		return 2
	}
	if cpuWeight >= 10000 {
		return 262144
	}

	// Use geometric mean to estimate original value before ceil()
	targetValue := math.Sqrt((cpuWeight - 1) * cpuWeight)
	exponent := math.Log10(targetValue)

	constant := 612.0 * (exponent + 7.0/34.0)
	discriminant := 125.0*125.0 + 4.0*constant
	l := (-125.0 + math.Sqrt(discriminant)) / 2.0
	return math.Round(math.Pow(2, l))
}

// absDiff returns the absolute difference between two uint64 values.
func absDiff(a, b uint64) uint64 {
	if a > b {
		return a - b
	}
	return b - a
}

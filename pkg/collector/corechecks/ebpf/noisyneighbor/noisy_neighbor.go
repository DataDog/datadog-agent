// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package noisyneighbor

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Check is the agent-side core check that consumes per-cgroup scheduling and
// PMU stats from the system-probe noisyneighbor module and emits Datadog
// metrics tagged by container. Run is not safe for concurrent invocations;
// the collector framework serializes Run per check instance. The tagger
// field is set by Factory; sysProbeClient and cgroupReader are set lazily
// by Configure.
type Check struct {
	core.CheckBase
	tagger         tagger.Component
	sysProbeClient *sysprobeclient.CheckClient
	cgroupReader   *cgroups.Reader
}

// Factory returns the check.Check constructor used by the collector to
// instantiate the noisy_neighbor check.
func Factory(t tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return &Check{
			CheckBase: core.NewCheckBaseWithInterval(CheckName, 10*time.Second),
			tagger:    t,
		}
	})
}

// Configure sets up the check by parsing its configuration, building a
// system-probe client, and initializing the cgroup reader used for tagging.
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source, provider string) error {
	if err := c.CommonConfigure(senderManager, initConfig, config, source, provider); err != nil {
		return fmt.Errorf("noisy_neighbor: common configure: %w", err)
	}
	socketPath := pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")
	c.sysProbeClient = sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(socketPath))
	reader, err := cgroups.NewReader(cgroups.WithReaderFilter(cgroups.ContainerFilter))
	if err != nil {
		return fmt.Errorf("noisy_neighbor: cgroup reader init failed: %w", err)
	}
	c.cgroupReader = reader
	return nil
}

// Run fetches the latest per-cgroup stats from system-probe, refreshes the
// cgroup reader, and emits Datadog metrics for each tracked container.
func (c *Check) Run() error {
	stats, err := sysprobeclient.GetCheck[[]model.Stats](c.sysProbeClient, sysconfig.NoisyNeighborModule)
	if err != nil {
		return fmt.Errorf("noisy_neighbor: get check from system-probe: %w", err)
	}

	s, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("noisy_neighbor: get metric sender: %w", err)
	}

	if err := c.cgroupReader.RefreshCgroups(0); err != nil {
		return fmt.Errorf("noisy_neighbor: refresh cgroups: %w", err)
	}

	for i := range stats {
		stat := &stats[i]
		cg := c.cgroupReader.GetCgroupByInode(stat.CgroupID)
		tags := containerTags(c.tagger, cg)
		submitPrimaryMetrics(s, stat, tags)
		submitRawCounters(s, stat, tags)
		submitPSIFullMetrics(s, cg, stat, tags)
	}
	s.Gauge("noisy_neighbor.system.cgroups_tracked", float64(len(stats)), "", nil)
	s.Commit()
	return nil
}

// containerTags returns high-cardinality container tags for the given
// cgroup, or nil if the cgroup is not a container or the tagger has no
// entry for it.
func containerTags(t tagger.Component, cg cgroups.Cgroup) []string {
	if cg == nil {
		return nil
	}
	containerID := cg.Identifier()
	if containerID == "" {
		return nil
	}
	entityID := types.NewEntityID(types.ContainerID, containerID)
	if entityID.Empty() {
		return nil
	}
	tags, err := t.Tag(entityID, types.HighCardinality)
	if err != nil {
		log.Warnf("noisy_neighbor: tagger error for container %s: %v", containerID, err)
		return nil
	}
	return tags
}

// submitPrimaryMetrics sends the main PSL (per-process scheduling latency)
// and PSP (per-process preemption) metrics. Note: "process" in metric names
// follows kernel convention, but these are thread-level measurements.
func submitPrimaryMetrics(s sender.Sender, stat *model.Stats, tags []string) {
	if stat.UniquePidCount == 0 {
		return
	}

	psl := float64(stat.SumLatenciesNs) / float64(stat.UniquePidCount)
	s.Gauge("noisy_neighbor.process_scheduling_latency.per_process", psl, "", tags)

	psp := float64(stat.PreemptionCount) / float64(stat.UniquePidCount)
	s.Gauge("noisy_neighbor.process_scheduler_preemptions.per_process", psp, "", tags)
}

// submitRawCounters emits cumulative scheduling and PMU counters as Counts
// and the unique-PID cardinality as a Gauge.
func submitRawCounters(s sender.Sender, stat *model.Stats, tags []string) {
	s.Count("noisy_neighbor.events.total", float64(stat.EventCount), "", tags)
	s.Gauge("noisy_neighbor.unique_processes", float64(stat.UniquePidCount), "", tags)
	s.Count("noisy_neighbor.cycles", float64(stat.SumCycles), "", tags)
	s.Count("noisy_neighbor.instructions", float64(stat.SumInstructions), "", tags)
	s.Count("noisy_neighbor.llc_misses", float64(stat.SumLLCMisses), "", tags)
	s.Count("noisy_neighbor.cache_references", float64(stat.SumCacheReferences), "", tags)
	s.Count("noisy_neighbor.itlb_misses", float64(stat.SumITLBMisses), "", tags)
	s.Count("noisy_neighbor.branch_misses", float64(stat.SumBranchMisses), "", tags)
	s.Count("noisy_neighbor.cpu_migrations", float64(stat.SumCPUMigrations), "", tags)
	s.Count("noisy_neighbor.softirq_ns", float64(stat.SumSoftirqNs), "", tags)
	s.Count("noisy_neighbor.block_io_requests", float64(stat.BlockIORequests), "", tags)
	s.Count("noisy_neighbor.wakeups", float64(stat.WakeupCount), "", tags)
}

// submitPSIFullMetrics emits the cgroup-scoped PSI "full" stalls for memory
// and io. The "some" variants are already emitted by the generic container
// processor as container.{memory,io}.partial_stall — we deliberately do not
// duplicate those here. CPU "full" PSI is not exposed at the cgroup level
// by the kernel.
//
// Emitted as Gauge of the cumulative-since-cgroup-creation microsecond
// counter. Backend or dashboard math can compute deltas; emitting Gauge
// (rather than Rate or MonotonicCount) means single-shot `agent check`
// invocations include the value, which Rate/MonotonicCount would drop for
// lack of a prior sample.
func submitPSIFullMetrics(s sender.Sender, cg cgroups.Cgroup, stat *model.Stats, tags []string) {
	if cg == nil {
		log.Debugf("noisy_neighbor: cgroup not found for inode %d, skipping PSI metrics", stat.CgroupID)
		return
	}
	memStats := &cgroups.MemoryStats{}
	if err := cg.GetMemoryStats(memStats); err != nil {
		log.Debugf("noisy_neighbor: GetMemoryStats failed for cgroup %d: %v", stat.CgroupID, err)
	} else if memStats.PSIFull.Total != nil {
		s.Gauge("noisy_neighbor.memory.pressure.full.total_us", float64(*memStats.PSIFull.Total), "", tags)
	}
	ioStats := &cgroups.IOStats{}
	if err := cg.GetIOStats(ioStats); err != nil {
		log.Debugf("noisy_neighbor: GetIOStats failed for cgroup %d: %v", stat.CgroupID, err)
	} else if ioStats.PSIFull.Total != nil {
		s.Gauge("noisy_neighbor.io.pressure.full.total_us", float64(*ioStats.PSIFull.Total), "", tags)
	}
}

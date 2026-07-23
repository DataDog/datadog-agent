// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package noisyneighbor

import (
	"fmt"
	"time"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// NoisyNeighborConfig holds the YAML-parseable configuration for the
// noisy_neighbor check. The check currently has no tunable knobs, but the
// type is kept so the standard check Configure flow can call Parse.
type NoisyNeighborConfig struct{}

// NoisyNeighborCheck is the agent-side core check that consumes per-cgroup
// scheduling and PMU stats from the system-probe noisyneighbor module and
// emits Datadog metrics tagged by container.
type NoisyNeighborCheck struct {
	core.CheckBase
	config         *NoisyNeighborConfig
	tagger         tagger.Component
	sysProbeClient *sysprobeclient.CheckClient
	cgroupReader   *cgroups.Reader
	// pmuMetricsEnabled is captured once during Configure to avoid a config
	// lookup per metric per Run tick. Keys are the agent-side metric short
	// names ("cycles", "instructions", ...); a missing or false entry means
	// the metric is not emitted.
	pmuMetricsEnabled map[string]bool
}

var pmuMetricConfigKeys = []string{
	"cycles", "instructions", "cache_misses", "cache_references",
	"itlb_misses", "branch_misses", "cpu_migrations",
}

// Factory returns the check.Check constructor used by the collector to
// instantiate the noisy_neighbor check.
func Factory(tagger tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(tagger)
	})
}

func newCheck(tagger tagger.Component) check.Check {
	return &NoisyNeighborCheck{
		CheckBase: core.NewCheckBaseWithInterval(CheckName, 10*time.Second),
		config:    &NoisyNeighborConfig{},
		tagger:    tagger,
	}
}

// Parse unmarshals the check's YAML configuration into c.
func (c *NoisyNeighborConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

// Configure sets up the check by parsing its configuration, building a
// system-probe client, and initializing the cgroup reader used for tagging.
func (n *NoisyNeighborCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string, provider string) error {
	if err := n.CommonConfigure(senderManager, initConfig, config, source, provider); err != nil {
		return err
	}
	if err := n.config.Parse(config); err != nil {
		return fmt.Errorf("noisy_neighbor check config: %w", err)
	}
	sysCfg := pkgconfigsetup.SystemProbe()
	n.sysProbeClient = sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(sysCfg.GetString("system_probe_config.sysprobe_socket")))
	reader, err := cgroups.NewReader(cgroups.WithReaderFilter(cgroups.ContainerFilter))
	if err != nil {
		return fmt.Errorf("noisy_neighbor: cgroup reader init failed: %w", err)
	}
	n.cgroupReader = reader
	n.pmuMetricsEnabled = make(map[string]bool, len(pmuMetricConfigKeys))
	for _, name := range pmuMetricConfigKeys {
		n.pmuMetricsEnabled[name] = sysCfg.GetBool("noisy_neighbor.pmu_metrics." + name)
	}
	return nil
}

// Run fetches the latest per-cgroup stats from system-probe, refreshes the
// cgroup reader, and emits Datadog metrics for each tracked container.
func (n *NoisyNeighborCheck) Run() error {
	stats, err := sysprobeclient.GetCheck[[]model.NoisyNeighborStats](n.sysProbeClient, sysconfig.NoisyNeighborModule)
	if err != nil {
		return fmt.Errorf("get noisy neighbor check: %w", err)
	}

	sender, err := n.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %w", err)
	}

	err = n.cgroupReader.RefreshCgroups(0)
	if err != nil {
		return fmt.Errorf("unable to refresh cgroups: %w", err)
	}

	var totalCgroups uint64
	for _, stat := range stats {
		totalCgroups++
		tags := n.getContainerTags(stat)
		n.submitPrimaryMetrics(sender, stat, tags)
		n.submitRawCounters(sender, stat, tags)
	}
	sender.Gauge("noisy_neighbor.system.cgroups_tracked", float64(totalCgroups), "", nil)
	sender.Commit()
	return nil
}

func (n *NoisyNeighborCheck) getContainerTags(stat model.NoisyNeighborStats) []string {
	if cg := n.cgroupReader.GetCgroupByInode(stat.CgroupID); cg != nil {
		containerID := cg.Identifier()
		if containerID != "" {
			entityID := types.NewEntityID(types.ContainerID, containerID)
			if !entityID.Empty() {
				taggerTags, err := n.tagger.Tag(entityID, types.HighCardinality)
				if err != nil {
					log.Warnf("noisy_neighbor: tagger error for container %s: %v", containerID, err)
				} else {
					return taggerTags
				}
			}
		}
	}
	return []string{}
}

// submitPrimaryMetrics sends the main PSL and PSP metrics
// Note: "process" in metric names follows kernel convention, but these are thread-level measurements
func (n *NoisyNeighborCheck) submitPrimaryMetrics(sender sender.Sender, stat model.NoisyNeighborStats, tags []string) {
	if stat.UniquePidCount == 0 {
		return
	}

	psl := float64(stat.SumLatenciesNs) / float64(stat.UniquePidCount)
	sender.Gauge("noisy_neighbor.process_scheduling_latency.per_process", psl, "", tags)

	psp := float64(stat.PreemptionCount) / float64(stat.UniquePidCount)
	sender.Gauge("noisy_neighbor.process_scheduler_preemptions.per_process", psp, "", tags)
}

func (n *NoisyNeighborCheck) submitRawCounters(sender sender.Sender, stat model.NoisyNeighborStats, tags []string) {
	// Always-on counters: scheduling and software accounting that don't
	// consume PMU counter slots.
	sender.Count("noisy_neighbor.events.total", float64(stat.EventCount), "", tags)
	sender.Gauge("noisy_neighbor.unique_processes", float64(stat.UniquePidCount), "", tags)
	sender.Count("noisy_neighbor.softirq_ns", float64(stat.SumSoftirqNs), "", tags)
	sender.Count("noisy_neighbor.block_io_requests", float64(stat.BlockIORequests), "", tags)
	sender.Count("noisy_neighbor.wakeups", float64(stat.WakeupCount), "", tags)

	// PMU counters: each is independently gated by noisy_neighbor.pmu_metrics.X.
	// Disabled events don't emit (no zero-valued noise on dashboards).
	if n.pmuMetricsEnabled["cycles"] {
		sender.Count("noisy_neighbor.cycles", float64(stat.SumCycles), "", tags)
	}
	if n.pmuMetricsEnabled["instructions"] {
		sender.Count("noisy_neighbor.instructions", float64(stat.SumInstructions), "", tags)
	}
	if n.pmuMetricsEnabled["cache_misses"] {
		sender.Count("noisy_neighbor.cache_misses", float64(stat.SumCacheMisses), "", tags)
	}
	if n.pmuMetricsEnabled["cache_references"] {
		sender.Count("noisy_neighbor.cache_references", float64(stat.SumCacheReferences), "", tags)
	}
	if n.pmuMetricsEnabled["itlb_misses"] {
		sender.Count("noisy_neighbor.itlb_misses", float64(stat.SumITLBMisses), "", tags)
	}
	if n.pmuMetricsEnabled["branch_misses"] {
		sender.Count("noisy_neighbor.branch_misses", float64(stat.SumBranchMisses), "", tags)
	}
	if n.pmuMetricsEnabled["cpu_migrations"] {
		sender.Count("noisy_neighbor.cpu_migrations", float64(stat.SumCPUMigrations), "", tags)
	}
}

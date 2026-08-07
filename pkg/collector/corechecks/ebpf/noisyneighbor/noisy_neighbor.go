// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package noisyneighbor

import (
	"errors"
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

type NoisyNeighborConfig struct{}

type NoisyNeighborCheck struct {
	core.CheckBase
	config         *NoisyNeighborConfig
	tagger         tagger.Component
	sysProbeClient *sysprobeclient.CheckClient
	cgroupReader   *cgroups.Reader
	rotator        watchlistRotator
	eventMask      uint64
	maxTracked     int
}

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

func (c *NoisyNeighborConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

func (n *NoisyNeighborCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string, provider string) error {
	if err := n.CommonConfigure(senderManager, initConfig, config, source, provider); err != nil {
		return err
	}
	if err := n.config.Parse(config); err != nil {
		return fmt.Errorf("noisy_neighbor check config: %s", err)
	}
	n.sysProbeClient = sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")))
	reader, err := cgroups.NewReader(cgroups.WithReaderFilter(cgroups.ContainerFilter))
	if err != nil {
		return fmt.Errorf("noisy_neighbor: cgroup reader init failed: %s", err)
	}
	n.cgroupReader = reader
	n.eventMask = configuredEventMask()
	n.maxTracked = pkgconfigsetup.SystemProbe().GetInt("noisy_neighbor.max_tracked_cgroups")
	if n.maxTracked < 1 || n.maxTracked > 128 {
		return errors.New("noisy_neighbor max_tracked_cgroups must be between 1 and 128")
	}
	return nil
}

func (n *NoisyNeighborCheck) Run() error {
	stats, err := sysprobeclient.GetCheck[[]model.NoisyNeighborStats](n.sysProbeClient, sysconfig.NoisyNeighborModule)
	if err != nil {
		return fmt.Errorf("get noisy neighbor check: %s", err)
	}

	sender, err := n.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %s", err)
	}

	err = n.cgroupReader.RefreshCgroups(0)
	if err != nil {
		return fmt.Errorf("unable to refresh cgroups: %s", err)
	}

	var totalCgroups uint64
	for _, stat := range stats {
		if stat.EventCount != 0 {
			totalCgroups++
		}
		tags, recognized := n.getContainerTags(stat)
		n.submitPrimaryMetrics(sender, stat, tags)
		n.submitRawCounters(sender, stat, tags)
		if recognized {
			n.submitPMUMetrics(sender, stat, tags)
		}
	}
	sender.Gauge("noisy_neighbor.system.cgroups_tracked", float64(totalCgroups), "", nil)
	if n.eventMask != 0 {
		n.rotateWatchlist()
	}
	sender.Commit()
	return nil
}

func (n *NoisyNeighborCheck) getContainerTags(stat model.NoisyNeighborStats) ([]string, bool) {
	if cg := n.cgroupReader.GetCgroupByInode(stat.CgroupID); cg != nil {
		containerID := cg.Identifier()
		if containerID != "" {
			entityID := types.NewEntityID(types.ContainerID, containerID)
			if !entityID.Empty() {
				taggerTags, err := n.tagger.Tag(entityID, types.HighCardinality)
				if err != nil {
					log.Warnf("noisy_neighbor: tagger error for container %s: %v", containerID, err)
				} else {
					return taggerTags, true
				}
			}
		}
	}
	return []string{}, false
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
	if stat.EventCount == 0 {
		return
	}
	sender.Count("noisy_neighbor.events.total", float64(stat.EventCount), "", tags)
	sender.Gauge("noisy_neighbor.unique_processes", float64(stat.UniquePidCount), "", tags)
}

func (n *NoisyNeighborCheck) submitPMUMetrics(sender sender.Sender, stat model.NoisyNeighborStats, tags []string) {
	metrics := []struct {
		name  string
		mask  uint64
		value uint64
	}{
		{"noisy_neighbor.cycles", model.EventCycles, stat.Cycles},
		{"noisy_neighbor.instructions", model.EventInstructions, stat.Instructions},
		{"noisy_neighbor.cache_misses", model.EventCacheMisses, stat.CacheMisses},
		{"noisy_neighbor.cache_references", model.EventCacheReferences, stat.CacheReferences},
		{"noisy_neighbor.itlb_misses", model.EventITLBMisses, stat.ITLBMisses},
		{"noisy_neighbor.branch_misses", model.EventBranchMisses, stat.BranchMisses},
		{"noisy_neighbor.cpu_migrations", model.EventCPUMigrations, stat.CPUMigrations},
	}
	for _, metric := range metrics {
		if stat.SampledEventMask&metric.mask != 0 {
			sender.Count(metric.name, float64(metric.value), "", tags)
		}
	}
}

func (n *NoisyNeighborCheck) rotateWatchlist() {
	live := make([]uint64, 0)
	for _, cgroup := range n.cgroupReader.ListCgroups() {
		if cgroup.Identifier() != "" && cgroup.Inode() != 0 {
			live = append(live, cgroup.Inode())
		}
	}
	request := model.WatchlistRequest{
		CgroupIDs:       n.rotator.selectNext(live, n.maxTracked),
		EligibleCgroups: len(live),
	}
	if _, err := sysprobeclient.Post[model.WatchlistResponse](n.sysProbeClient, "/watchlist", request, sysconfig.NoisyNeighborModule); err != nil {
		log.Warnf("noisy_neighbor: unable to update PMU watchlist: %v", err)
	}
}

func configuredEventMask() uint64 {
	cfg := pkgconfigsetup.SystemProbe()
	var mask uint64
	settings := []struct {
		key  string
		mask uint64
	}{
		{"cycles", model.EventCycles},
		{"instructions", model.EventInstructions},
		{"cache_misses", model.EventCacheMisses},
		{"cache_references", model.EventCacheReferences},
		{"itlb_misses", model.EventITLBMisses},
		{"branch_misses", model.EventBranchMisses},
		{"cpu_migrations", model.EventCPUMigrations},
	}
	for _, setting := range settings {
		if cfg.GetBool("noisy_neighbor.pmu_metrics." + setting.key) {
			mask |= setting.mask
		}
	}
	return mask
}

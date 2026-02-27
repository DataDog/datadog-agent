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

type NoisyNeighborConfig struct{}

type NoisyNeighborCheck struct {
	core.CheckBase
	config         *NoisyNeighborConfig
	tagger         tagger.Component
	sysProbeClient *sysprobeclient.CheckClient
	cgroupReader   *cgroups.Reader
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

func (n *NoisyNeighborCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if err := n.CommonConfigure(senderManager, initConfig, config, source); err != nil {
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
	sender.Count("noisy_neighbor.events.total", float64(stat.EventCount), "", tags)
	sender.Gauge("noisy_neighbor.unique_processes", float64(stat.UniquePidCount), "", tags)
}

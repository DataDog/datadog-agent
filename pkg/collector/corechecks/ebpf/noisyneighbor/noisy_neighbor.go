// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package noisyneighbor

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
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
}

func Factory(tagger tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(tagger)
	})
}

func newCheck(tagger tagger.Component) check.Check {
	return &NoisyNeighborCheck{
		CheckBase: core.NewCheckBase(CheckName),
		config:    &NoisyNeighborConfig{},
		tagger:    tagger,
	}
}

func (n *NoisyNeighborCheck) Interval() time.Duration {
	return 5 * time.Second
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

	var totalCgroups, starvedCgroups uint64

	for _, stat := range stats {
		if stat.EventCount == 0 && stat.PreemptionCount > 0 {
			starvedCgroups++
			continue
		}

		totalCgroups++
		tags := n.buildTags(stat)
		n.submitPrimaryMetrics(sender, stat, tags)
		n.submitRawCounters(sender, stat, tags)
		n.submitLegacyMetrics(sender, stat, tags)
	}

	sender.Gauge("noisy_neighbor.system.cgroups_tracked", float64(totalCgroups), "", nil)
	if starvedCgroups > 0 {
		sender.Gauge("noisy_neighbor.system.cgroups_starved", float64(starvedCgroups), "", nil)
		log.Warnf("[noisy_neighbor] Detected %d starved cgroups (preempted but never scheduled)", starvedCgroups)
	}

	sender.Commit()
	return nil
}

func (n *NoisyNeighborCheck) buildTags(stat model.NoisyNeighborStats) []string {
	cgroupName := stat.CgroupName
	if cgroupName == "" {
		cgroupName = "unknown"
	}

	var tags []string
	containerID := getContainerID(cgroupName)

	if containerID != "" {
		entityID := types.NewEntityID(types.ContainerID, containerID)
		if !entityID.Empty() {
			containerTags, err := n.tagger.Tag(entityID, types.ChecksConfigCardinality)
			if err != nil {
				log.Errorf("Error collecting tags for container %s: %s", containerID, err)
			} else {
				tags = containerTags
			}
		}
	}

	tags = append(tags, "cgroup_name:"+cgroupName)
	tags = append(tags, fmt.Sprintf("cgroup_id:%d", stat.CgroupID))

	if containerID != "" && containerID != "host" {
		tags = append(tags, "container_id:"+containerID)
	}

	return tags
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

	eventsPerProcess := float64(stat.EventCount) / float64(stat.UniquePidCount)
	sender.Gauge("noisy_neighbor.events.per_process", eventsPerProcess, "", tags)
}

func (n *NoisyNeighborCheck) submitRawCounters(sender sender.Sender, stat model.NoisyNeighborStats, tags []string) {
	sender.Count("noisy_neighbor.events.total", float64(stat.EventCount), "", tags)
	sender.Count("noisy_neighbor.process_scheduler_preemptions.total", float64(stat.PreemptionCount), "", tags)
	sender.Count("noisy_neighbor.process_scheduling_latency.total", float64(stat.SumLatenciesNs), "", tags)
	sender.Gauge("noisy_neighbor.unique_processes", float64(stat.UniquePidCount), "", tags)
}

func (n *NoisyNeighborCheck) submitLegacyMetrics(sender sender.Sender, stat model.NoisyNeighborStats, tags []string) {
	if stat.RunqLatencyNs == 0 {
		return
	}

	sender.Distribution("noisy_neighbor.runq.latency", float64(stat.RunqLatencyNs), "", tags)

	prevCgroupName := stat.PrevCgroupName
	if prevCgroupName == "" {
		prevCgroupName = "unknown"
	}
	prevContainerID := getContainerID(prevCgroupName)

	switchTags := append(tags, "prev_cgroup_name:"+prevCgroupName)
	switchTags = append(switchTags, fmt.Sprintf("prev_cgroup_id:%d", stat.PrevCgroupID))

	if prevContainerID != "" && prevContainerID != "host" {
		switchTags = append(switchTags, "prev_container_id:"+prevContainerID)

		entityID := types.NewEntityID(types.ContainerID, prevContainerID)
		if !entityID.Empty() {
			prevTags, err := n.tagger.Tag(entityID, types.ChecksConfigCardinality)
			if err != nil {
				log.Errorf("Error collecting tags for prev container %s: %s", prevContainerID, err)
			} else {
				for _, tag := range prevTags {
					switchTags = append(switchTags, "prev_"+tag)
				}
			}
		}
	}

	sender.Count("noisy_neighbor.sched.switch.out", 1, "", switchTags)
}

func getContainerID(cgroupName string) string {
	containerID, err := cgroups.ContainerFilter("", cgroupName)
	if err != nil {
		log.Debugf("Unable to extract containerID from cgroup name: %s, err: %v", cgroupName, err)
	}
	return containerID
}

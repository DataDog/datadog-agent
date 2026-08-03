// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package numamonitoring

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/numamonitoring/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type numaMonitoringCheck struct {
	core.CheckBase
	tagger         tagger.Component
	sysProbeClient *sysprobeclient.CheckClient
	cgroupReader   *cgroups.Reader
}

// Factory creates a NUMA monitoring check.
func Factory(tagger tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return &numaMonitoringCheck{
			CheckBase: core.NewCheckBaseWithInterval(CheckName, 10*time.Second),
			tagger:    tagger,
		}
	})
}

func (check *numaMonitoringCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source, provider string) error {
	if err := check.CommonConfigure(senderManager, initConfig, config, source, provider); err != nil {
		return err
	}
	check.sysProbeClient = sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")))
	reader, err := cgroups.NewReader(cgroups.WithReaderFilter(cgroups.ContainerFilter))
	if err != nil {
		return fmt.Errorf("initialize NUMA monitoring cgroup reader: %w", err)
	}
	check.cgroupReader = reader
	return nil
}

func (check *numaMonitoringCheck) Run() error {
	response, err := sysprobeclient.GetCheck[model.Response](check.sysProbeClient, sysconfig.NUMAMonitoringModule)
	if err != nil {
		return sysprobeclient.IgnoreStartupError(err)
	}
	sender, err := check.GetSender()
	if err != nil {
		return err
	}
	if err := check.cgroupReader.RefreshCgroups(0); err != nil {
		return fmt.Errorf("refresh NUMA monitoring cgroups: %w", err)
	}
	for _, stats := range response.Containers {
		submitStats(sender, stats, check.containerTags(stats.CgroupID))
	}
	sender.Commit()
	return nil
}

func submitStats(metricSender sender.Sender, stats model.ContainerStats, tags []string) {
	for node, share := range stats.RuntimeShares {
		metricSender.Gauge("system.numa.cpu.runtime_share", share, "", appendTag(tags, fmt.Sprintf("numa_node:%d", node)))
	}
	for node, bytes := range stats.ResidentBytes {
		metricSender.Gauge("system.numa.memory.resident", float64(bytes), "", appendTag(tags, fmt.Sprintf("numa_node:%d", node)))
	}
	for _, domain := range stats.Domains {
		domainTags := appendTag(tags, "resctrl_domain:"+domain.Domain)
		submitOptional(metricSender, "system.numa.cache.llc_occupancy", domain.LLCOccupancy, domainTags)
		submitOptional(metricSender, "system.numa.memory.bandwidth.total", domain.TotalBandwidth, domainTags)
		submitOptional(metricSender, "system.numa.memory.bandwidth.local", domain.LocalBandwidth, domainTags)
		submitOptional(metricSender, "system.numa.memory.bandwidth.remote_estimated", domain.RemoteBandwidth, domainTags)
	}
	submitOptional(metricSender, "system.numa.remote_ratio", stats.RemoteRatio, tags)
	submitOptional(metricSender, "system.numa.placement_mismatch", stats.PlacementMismatch, tags)
	submitOptional(metricSender, "system.numa.badness_score", stats.BadnessScore, tags)
}

func (check *numaMonitoringCheck) containerTags(cgroupID uint64) []string {
	cgroup := check.cgroupReader.GetCgroupByInode(cgroupID)
	if cgroup == nil || cgroup.Identifier() == "" {
		return nil
	}
	entityID := types.NewEntityID(types.ContainerID, cgroup.Identifier())
	tags, err := check.tagger.Tag(entityID, types.HighCardinality)
	if err != nil {
		log.Warnf("NUMA monitoring tagger error for container %s: %v", cgroup.Identifier(), err)
		return nil
	}
	return tags
}

func appendTag(tags []string, tag string) []string {
	result := make([]string, len(tags), len(tags)+1)
	copy(result, tags)
	return append(result, tag)
}

func submitOptional(metricSender sender.Sender, name string, value *float64, tags []string) {
	if value != nil {
		metricSender.Gauge(name, *value, "", tags)
	}
}

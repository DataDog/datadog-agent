// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package syscalllatencycheck

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/syscalllatency/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// SyscallLatencyCheck collects per-syscall latency metrics via system-probe.
type SyscallLatencyCheck struct {
	core.CheckBase
	sysProbeClient *sysprobeclient.CheckClient
}

// Factory creates a new check factory.
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &SyscallLatencyCheck{
		CheckBase: core.NewCheckBase(CheckName),
	}
}

// Configure parses the check configuration and initialises the system-probe client.
func (c *SyscallLatencyCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if err := c.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}
	c.sysProbeClient = sysprobeclient.GetCheckClient(
		sysprobeclient.WithSocketPath(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
	)
	return nil
}

// Run executes one check interval.
func (c *SyscallLatencyCheck) Run() error {
	stats, err := sysprobeclient.GetCheck[[]model.SyscallLatencyStats](c.sysProbeClient, sysconfig.SyscallLatencyCheckModule)
	if err != nil {
		return sysprobeclient.IgnoreStartupError(err)
	}

	s, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %w", err)
	}

	for _, stat := range stats {
		tags := []string{"syscall:" + stat.Syscall}
		if stat.ContainerID != "" {
			tags = append(tags, "container_id:"+stat.ContainerID)
		}
		s.MonotonicCount("system.syscall.latency.total", float64(stat.TotalTimeNs), "", tags)
		s.MonotonicCount("system.syscall.latency.count", float64(stat.Count), "", tags)
		s.Gauge("system.syscall.latency.max", float64(stat.MaxTimeNs), "", tags)
		s.MonotonicCount("system.syscall.latency.slow_count", float64(stat.SlowCount), "", tags)
	}

	s.Commit()
	return nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package gpu

import (
	"fmt"

	"gopkg.in/yaml.v2"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	processnet "github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	gpuMetricsNs     = "gpu."
	metricNameMemory = gpuMetricsNs + "memory"
	metricNameUtil   = gpuMetricsNs + "utilization"
	metricNameMaxMem = gpuMetricsNs + "memory.max"
)

// Check represents the GPU check that will be periodically executed via the Run() function
type Check struct {
	core.CheckBase
	config       *CheckConfig            // config for the check
	sysProbeUtil processnet.SysProbeUtil // sysProbeUtil is used to communicate with system probe
	activePIDs   map[uint32]bool         // activePIDs is a set of PIDs that have been seen in the current check run
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase:  core.NewCheckBase(CheckName),
		config:     &CheckConfig{},
		activePIDs: make(map[uint32]bool),
	}
}

// Configure parses the check configuration and init the check
func (m *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if err := m.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}

	if err := yaml.Unmarshal(config, m.config); err != nil {
		return fmt.Errorf("invalid gpu check config: %w", err)
	}

	return nil
}

func (m *Check) ensureInitialized() error {
	var err error

	if m.sysProbeUtil == nil {
		m.sysProbeUtil, err = processnet.GetRemoteSystemProbeUtil(
			pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket"),
		)
		if err != nil {
			return fmt.Errorf("sysprobe connection: %w", err)
		}
	}

	return nil
}

// Run executes the check
func (m *Check) Run() error {
	if err := m.ensureInitialized(); err != nil {
		return err
	}

	snd, err := m.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %w", err)
	}
	// Commit the metrics even in case of an error
	defer snd.Commit()

	sysprobeData, err := m.sysProbeUtil.GetCheck(sysconfig.GPUMonitoringModule)
	if err != nil {
		return fmt.Errorf("cannot get data from system-probe: %w", err)
	}

	stats, ok := sysprobeData.(model.GPUStats)
	if !ok {
		return log.Errorf("gpu check raw data has incorrect type: %T", stats)
	}

	// Set all PIDs to inactive, so we can remove the ones that we don't see
	// and send the final metrics
	for pid := range m.activePIDs {
		m.activePIDs[pid] = false
	}

	for pid, pidStats := range stats.ProcessStats {
		// Per-PID metrics are subject to change due to high cardinality
		tags := []string{fmt.Sprintf("pid:%d", pid)}
		snd.Gauge(metricNameMemory, float64(pidStats.CurrentMemoryBytes), "", tags)
		snd.Gauge(metricNameMaxMem, float64(pidStats.MaxMemoryBytes), "", tags)
		snd.Gauge(metricNameUtil, pidStats.UtilizationPercentage, "", tags)

		m.activePIDs[pid] = true
	}

	// Remove the PIDs that we didn't see in this check
	for pid, active := range m.activePIDs {
		if !active {
			tags := []string{fmt.Sprintf("pid:%d", pid)}
			snd.Gauge(metricNameMemory, 0, "", tags)
			snd.Gauge(metricNameMaxMem, 0, "", tags)
			snd.Gauge(metricNameUtil, 0, "", tags)

			delete(m.activePIDs, pid)
		}
	}

	return nil
}

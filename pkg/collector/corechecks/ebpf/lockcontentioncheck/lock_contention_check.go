// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package lockcontentioncheck

import (
	"fmt"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/lockcontentioncheck/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// LockContentionConfig is the config of the lock contention check
type LockContentionConfig struct{}

// LockContentionCheck collects kernel lock contention metrics via system-probe
type LockContentionCheck struct {
	core.CheckBase
	config         *LockContentionConfig
	sysProbeClient *sysprobeclient.CheckClient
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &LockContentionCheck{
		CheckBase: core.NewCheckBase(CheckName),
		config:    &LockContentionConfig{},
	}
}

// Parse parses the check configuration
func (c *LockContentionConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

// Configure parses the check configuration and init the check
func (m *LockContentionCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if err := m.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}
	if err := m.config.Parse(config); err != nil {
		return fmt.Errorf("lock_contention check config: %s", err)
	}
	m.sysProbeClient = sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")))
	return nil
}

// Run executes the check
func (m *LockContentionCheck) Run() error {
	stats, err := sysprobeclient.GetCheck[[]model.LockContentionStats](m.sysProbeClient, sysconfig.LockContentionCheckModule)
	if err != nil {
		return fmt.Errorf("get lock_contention check: %s", err)
	}

	s, err := m.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %s", err)
	}

	for _, stat := range stats {
		tags := []string{"lock_type:" + stat.LockType}
		s.MonotonicCount("system.lock_contention.wait_time", float64(stat.TotalTimeNs), "", tags)
		s.MonotonicCount("system.lock_contention.count", float64(stat.Count), "", tags)
		s.Gauge("system.lock_contention.max_wait", float64(stat.MaxTimeNs), "", tags)
	}

	s.Commit()
	return nil
}

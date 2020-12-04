// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// FIXME: we require the `cgo` build tag because of this dep relationship:
// github.com/DataDog/datadog-agent/pkg/process/net depends on `github.com/DataDog/agent-payload/process`,
// which has a hard dependency on `github.com/DataDog/zstd`, which requires CGO.
// Should be removed once `github.com/DataDog/agent-payload/process` can be imported with CGO disabled.
// +build cgo
// +build linux

package linuxaudit

import (
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	dd_config "github.com/DataDog/datadog-agent/pkg/config"
	process_net "github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/elastic/go-libaudit"
)

const (
	linuxAuditCheckName = "linux_audit"
)

type linuxAuditConfig struct {
	CollectLinuxAudit bool `yaml:"collect_linux_audit"`
}

type linuxAuditCheck struct {
	core.CheckBase
	instance *linuxAuditConfig
}

func (c *linuxAuditCheck) Run() error {
	if !c.instance.CollectLinuxAudit {
		return nil
	}

	sysProbeUtil, err := process_net.GetRemoteSystemProbeUtil()
	if err != nil {
		return err
	}

	data, err := sysProbeUtil.GetCheck("linux_audit")
	if err != nil {
		return err
	}

	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	statuses, ok := data.([]libaudit.AuditStatus)
	if !ok {
		return log.Errorf("Raw data has incorrect type")
	}

	for _, status := range statuses {
		sender.Gauge("linux_audit.enabled", float64(status.Enabled), "", nil)
		sender.Gauge("linux_audit.failure", float64(status.Failure), "", nil)
		sender.Gauge("linux_audit.rate_limit", float64(status.RateLimit), "", nil)
		sender.Gauge("linux_audit.backlog_limit", float64(status.BacklogLimit), "", nil)
		sender.Gauge("linux_audit.lost", float64(status.Lost), "", nil)
		sender.Gauge("linux_audit.backlog", float64(status.Backlog), "", nil)
	}

	sender.Commit()
	return nil
}

func init() {
	core.RegisterCheck(linuxAuditCheckName, linuxAuditFactory)
}

func linuxAuditFactory() check.Check {
	return &linuxAuditCheck{
		CheckBase: core.NewCheckBase(linuxAuditCheckName),
		instance:  &linuxAuditConfig{},
	}
}

// Parse parses the check configuration and init the check
func (c *linuxAuditConfig) Parse(data []byte) error {
	// default values
	c.CollectLinuxAudit = true

	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}
	return nil
}

// Configure parses the check configuration and init the check
func (c *linuxAuditCheck) Configure(config, initConfig integration.Data, source string) error {
	// TODO: Remove that hard-code and put it somewhere else
	process_net.SetSystemProbePath(dd_config.Datadog.GetString("system_probe_config.sysprobe_socket"))

	err := c.CommonConfigure(config, source)
	if err != nil {
		return err
	}

	return c.instance.Parse(config)
}

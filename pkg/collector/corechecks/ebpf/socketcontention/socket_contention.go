// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package socketcontention

import (
	"fmt"
	"time"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/socketcontention/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Config is the config of the socket contention check.
type Config struct{}

// Check fetches socket contention stats from system-probe.
type Check struct {
	core.CheckBase
	config         *Config
	sysProbeClient *sysprobeclient.CheckClient
}

// Factory creates a new check factory.
func Factory(_ tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck()
	})
}

func newCheck() check.Check {
	return &Check{
		CheckBase: core.NewCheckBaseWithInterval(CheckName, 10*time.Second),
		config:    &Config{},
	}
}

// Parse parses the check configuration.
func (c *Config) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

// Configure parses the check configuration and initializes the check.
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string, provider string) error {
	if err := c.CommonConfigure(senderManager, initConfig, config, source, provider); err != nil {
		return err
	}
	if err := c.config.Parse(config); err != nil {
		return fmt.Errorf("socket_contention check config: %w", err)
	}

	c.sysProbeClient = sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")))
	return nil
}

// Run executes the check.
func (c *Check) Run() error {
	stats, err := sysprobeclient.GetCheck[model.SocketContentionStats](c.sysProbeClient, sysconfig.SocketContentionModule)
	if err != nil {
		return sysprobeclient.IgnoreStartupError(err)
	}

	s, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %w", err)
	}

	s.Gauge("socket_contention.sockets_initialized", float64(stats.SocketInits), "", nil)
	s.Commit()
	return nil
}

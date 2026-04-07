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
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/socketcontention/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Config is the config of the socket contention check.
type Config struct{}

// Check fetches socket contention stats from system-probe.
type Check struct {
	core.CheckBase
	config         *Config
	tagger         tagger.Component
	sysProbeClient *sysprobeclient.CheckClient
	cgroupReader   *cgroups.Reader
}

// Factory creates a new check factory.
func Factory(tagger tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(tagger)
	})
}

func newCheck(tagger tagger.Component) check.Check {
	return &Check{
		CheckBase: core.NewCheckBaseWithInterval(CheckName, 10*time.Second),
		config:    &Config{},
		tagger:    tagger,
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
	reader, err := cgroups.NewReader(cgroups.WithReaderFilter(cgroups.ContainerFilter))
	if err != nil {
		return fmt.Errorf("socket_contention cgroup reader init: %w", err)
	}
	c.cgroupReader = reader
	return nil
}

// Run executes the check.
func (c *Check) Run() error {
	stats, err := sysprobeclient.GetCheck[model.SocketContentionStats](c.sysProbeClient, sysconfig.SocketContentionModule)
	if err != nil {
		return sysprobeclient.IgnoreStartupError(err)
	}
	if c.cgroupReader != nil {
		if err := c.cgroupReader.RefreshCgroups(0); err != nil {
			return fmt.Errorf("refresh cgroups: %w", err)
		}
	}

	s, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %w", err)
	}

	for _, stat := range stats {
		if stat.Count == 0 {
			continue
		}

		tags := []string{
			"object_kind:" + stat.ObjectKind,
			"socket_type:" + stat.SocketType,
			"socket_family:" + stat.Family,
			"protocol:" + stat.Protocol,
			"lock_subtype:" + stat.LockSubtype,
		}
		tags = append(tags, c.getContainerTags(stat.CgroupID)...)

		s.Gauge("socket_contention.contention_count", float64(stat.Count), "", tags)
		s.Gauge("socket_contention.contention_total_ns", float64(stat.TotalTimeNS), "", tags)
		s.Gauge("socket_contention.contention_max_ns", float64(stat.MaxTimeNS), "", tags)
		s.Gauge("socket_contention.contention_min_ns", float64(stat.MinTimeNS), "", tags)
	}
	s.Commit()
	return nil
}

func (c *Check) getContainerTags(cgroupID uint64) []string {
	if c.cgroupReader == nil || cgroupID == 0 {
		return nil
	}

	cg := c.cgroupReader.GetCgroupByInode(cgroupID)
	if cg == nil {
		return nil
	}

	containerID := cg.Identifier()
	if containerID == "" {
		return nil
	}

	entityID := types.NewEntityID(types.ContainerID, containerID)
	if entityID.Empty() {
		return nil
	}

	tags, err := c.tagger.Tag(entityID, types.HighCardinality)
	if err != nil {
		log.Warnf("socket_contention: tagger error for container %s: %v", containerID, err)
		return nil
	}

	return tags
}

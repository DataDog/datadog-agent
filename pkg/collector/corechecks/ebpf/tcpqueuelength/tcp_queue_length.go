// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package tcpqueuelength contains the TCP Queue Length check
package tcpqueuelength

import (
	"fmt"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/tcpqueuelength/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "tcp_queue_length"
)

// TCPQueueLengthConfig is the config of the TCP Queue Length check
type TCPQueueLengthConfig struct {
	CollectTCPQueueLength bool `yaml:"collect_tcp_queue_length"`
}

// TCPQueueLengthCheck grabs TCP queue length metrics
type TCPQueueLengthCheck struct {
	core.CheckBase
	instance       *TCPQueueLengthConfig
	tagger         tagger.Component
	sysProbeClient *sysprobeclient.CheckClient
}

// Factory creates a new check factory
func Factory(tagger tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(tagger)
	})
}

func newCheck(tagger tagger.Component) check.Check {
	return &TCPQueueLengthCheck{
		CheckBase: core.NewCheckBase(CheckName),
		instance:  &TCPQueueLengthConfig{},
		tagger:    tagger,
	}
}

// Parse parses the check configuration and init the check
func (t *TCPQueueLengthConfig) Parse(data []byte) error {
	// default values
	t.CollectTCPQueueLength = true

	return yaml.Unmarshal(data, t)
}

// Configure parses the check configuration and init the check
func (t *TCPQueueLengthCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if !pkgconfigsetup.SystemProbe().GetBool("system_probe_config.enable_tcp_queue_length") {
		return fmt.Errorf("tcp_queue_length check requires system_probe_config.enable_tcp_queue_length to be set to true")
	}

	err := t.CommonConfigure(senderManager, initConfig, config, source)
	if err != nil {
		return err
	}
	t.sysProbeClient = sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")))

	return t.instance.Parse(config)
}

// Run executes the check
func (t *TCPQueueLengthCheck) Run() error {
	if !t.instance.CollectTCPQueueLength {
		return nil
	}

	stats, err := sysprobeclient.GetCheck[model.TCPQueueLengthStats](t.sysProbeClient, sysconfig.TCPQueueLengthTracerModule)
	if err != nil {
		return sysprobeclient.IgnoreStartupError(err)
	}

	sender, err := t.GetSender()
	if err != nil {
		return err
	}

	for k, v := range stats {
		containerID, err := cgroups.ContainerFilter("", k)
		if err != nil || containerID == "" {
			log.Debugf("Unable to extract containerID from cgroup name: %s, err: %v", k, err)
			continue
		}

		entityID := types.NewEntityID(types.ContainerID, containerID)
		var tags []string
		if !entityID.Empty() {
			tags, err = t.tagger.Tag(entityID, types.HighCardinality)
			if err != nil {
				log.Errorf("Error collecting tags for container %s: %s", k, err)
			}
		}

		sender.Gauge("tcp_queue.read_buffer_max_usage_pct", float64(v.ReadBufferMaxUsage)/1000.0, "", tags)
		sender.Gauge("tcp_queue.write_buffer_max_usage_pct", float64(v.WriteBufferMaxUsage)/1000.0, "", tags)
	}

	sender.Commit()
	return nil
}

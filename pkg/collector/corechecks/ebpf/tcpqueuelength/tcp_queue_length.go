// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// FIXME: we require the `cgo` build tag because of this dep relationship:
// github.com/DataDog/datadog-agent/pkg/process/net depends on `github.com/DataDog/agent-payload/v5/process`,
// which has a hard dependency on `github.com/DataDog/zstd_0`, which requires CGO.
// Should be removed once `github.com/DataDog/agent-payload/v5/process` can be imported with CGO disabled.
//go:build cgo && linux

//nolint:revive // TODO(PLINT) Fix revive linter
package tcpqueuelength

import (
	yaml "gopkg.in/yaml.v2"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/tcpqueuelength/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	process_net "github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
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
	instance *TCPQueueLengthConfig
	tagger   tagger.Component
}

// Factory creates a new check factory
func Factory(tagger tagger.Component) optional.Option[func() check.Check] {
	return optional.NewOption(func() check.Check {
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
	err := t.CommonConfigure(senderManager, initConfig, config, source)
	if err != nil {
		return err
	}

	return t.instance.Parse(config)
}

// Run executes the check
func (t *TCPQueueLengthCheck) Run() error {
	if !t.instance.CollectTCPQueueLength {
		return nil
	}

	sysProbeUtil, err := process_net.GetRemoteSystemProbeUtil(
		pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket"))
	if err != nil {
		return err
	}

	data, err := sysProbeUtil.GetCheck(sysconfig.TCPQueueLengthTracerModule)
	if err != nil {
		return err
	}

	sender, err := t.GetSender()
	if err != nil {
		return err
	}

	stats, ok := data.(model.TCPQueueLengthStats)
	if !ok {
		return log.Errorf("Raw data has incorrect type")
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

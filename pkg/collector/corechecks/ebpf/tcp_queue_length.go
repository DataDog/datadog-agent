// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// FIXME: we require the `cgo` build tag because of this dep relationship:
// github.com/DataDog/datadog-agent/pkg/process/net depends on `github.com/DataDog/agent-payload/process`,
// which has a hard dependency on `github.com/DataDog/zstd`, which requires CGO.
// Should be removed once `github.com/DataDog/agent-payload/process` can be imported with CGO disabled.
// +build cgo
// +build linux

package ebpf

import (
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	dd_config "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/ebpf/tcpqueuelength"
	process_net "github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	tcpQueueLengthCheckName = "tcp_queue_length"
)

// TCPQueueLengthConfig is the config of the TCP Queue Length check
type TCPQueueLengthConfig struct {
	CollectTCPQueueLength bool `yaml:"collect_tcp_queue_length"`
}

// TCPQueueLengthCheck grabs TCP queue length metrics
type TCPQueueLengthCheck struct {
	core.CheckBase
	instance *TCPQueueLengthConfig
}

func init() {
	core.RegisterCheck(tcpQueueLengthCheckName, TCPQueueLengthFactory)
}

// TCPQueueLengthFactory is exported for integration testing
func TCPQueueLengthFactory() check.Check {
	return &TCPQueueLengthCheck{
		CheckBase: core.NewCheckBase(tcpQueueLengthCheckName),
		instance:  &TCPQueueLengthConfig{},
	}
}

// Parse parses the check configuration and init the check
func (t *TCPQueueLengthConfig) Parse(data []byte) error {
	// default values
	t.CollectTCPQueueLength = true

	if err := yaml.Unmarshal(data, t); err != nil {
		return err
	}
	return nil
}

//Configure parses the check configuration and init the check
func (t *TCPQueueLengthCheck) Configure(config, initConfig integration.Data, source string) error {
	// TODO: Remove that hard-code and put it somewhere else
	process_net.SetSystemProbePath(dd_config.Datadog.GetString("system_probe_config.sysprobe_socket"))

	err := t.CommonConfigure(config, source)
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

	sysProbeUtil, err := process_net.GetRemoteSystemProbeUtil()
	if err != nil {
		return err
	}

	data, err := sysProbeUtil.GetCheck("tcp_queue_length")
	if err != nil {
		return err
	}

	sender, err := aggregator.GetSender(t.ID())
	if err != nil {
		return err
	}

	stats, ok := data.(tcpqueuelength.Stats)
	if !ok {
		return log.Errorf("Raw data has incorrect type")
	}

	for k, v := range stats {
		entityID := containers.BuildTaggerEntityName(k)
		var tags []string
		if entityID != "" {
			tags, err = tagger.Tag(entityID, collectors.HighCardinality)
			if err != nil {
				log.Errorf("Error collecting tags for container %s: %s", k, err)
			}
		}

		sender.Gauge("tcp_queue.read_buffer_max_fill_rate", float64(v.ReadBufferMaxFillRate)/1000.0, "", tags)
		sender.Gauge("tcp_queue.write_buffer_max_fill_rate", float64(v.WriteBufferMaxFillRate)/1000.0, "", tags)
	}

	sender.Commit()
	return nil
}

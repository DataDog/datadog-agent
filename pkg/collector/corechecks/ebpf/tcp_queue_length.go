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
	"sort"
	"strconv"
	"strings"

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
	OnlyCountNbContexts   bool `yaml:"only_count_nb_contexts"` // For impact analysis only. To be removed after
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
	t.OnlyCountNbContexts = true

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

var tagsSet = make(map[string]struct{})

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

	for _, lineRaw := range data {
		line, ok := lineRaw.(tcpqueuelength.Stats)
		if !ok {
			log.Error("Raw data has incorrect type")
			continue
		}
		entityID := containers.BuildTaggerEntityName(line.ContainerID)
		var tags []string
		if entityID != "" {
			tags, err = tagger.Tag(entityID, collectors.OrchestratorCardinality)
			if err != nil {
				log.Errorf("Could not collect tags for container %s: %s", line.ContainerID, err)
			}
		}

		tags = append(tags,
			"saddr:"+line.Conn.Saddr.String(),
			"daddr:"+line.Conn.Daddr.String(),
			"sport:"+strconv.Itoa(int(line.Conn.Sport)),
			"dport:"+strconv.Itoa(int(line.Conn.Dport)),
			"pid:"+strconv.Itoa(int(line.Pid)))

		if t.instance.OnlyCountNbContexts {
			sort.Strings(tags)
			tagsSet[strings.Join(tags, ",")] = struct{}{}
		} else {
			sender.Gauge("tcp_queue.rqueue.size", float64(line.Rqueue.Size), "", tags)
			sender.Gauge("tcp_queue.rqueue.min", float64(line.Rqueue.Min), "", tags)
			sender.Gauge("tcp_queue.rqueue.max", float64(line.Rqueue.Max), "", tags)
			sender.Gauge("tcp_queue.wqueue.size", float64(line.Wqueue.Size), "", tags)
			sender.Gauge("tcp_queue.wqueue.min", float64(line.Wqueue.Min), "", tags)
			sender.Gauge("tcp_queue.wqueue.max", float64(line.Wqueue.Max), "", tags)
		}
	}

	if t.instance.OnlyCountNbContexts {
		sender.Gauge("tcp_queue.nb_contexts", float64(len(tagsSet)), "", []string{})
	}

	sender.Commit()
	return nil
}

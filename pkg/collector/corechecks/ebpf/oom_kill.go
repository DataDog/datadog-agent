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
	"fmt"
	"strings"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	dd_config "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/ebpf/oomkill"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	process_net "github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	oomKillCheckName = "oom_kill"
)

// OOMKillConfig is the config of the OOMKill check
type OOMKillConfig struct {
	CollectOOMKill bool `yaml:"collect_oom_kill"`
}

// OOMKillCheck grabs OOM Kill metrics
type OOMKillCheck struct {
	core.CheckBase
	instance *OOMKillConfig
}

// OOMKillFactory is exported for integration testing
func OOMKillFactory() check.Check {
	return &OOMKillCheck{
		CheckBase: core.NewCheckBase(oomKillCheckName),
		instance:  &OOMKillConfig{},
	}
}

func init() {
	core.RegisterCheck(oomKillCheckName, OOMKillFactory)
}

// Parse parses the check configuration
func (c *OOMKillConfig) Parse(data []byte) error {
	// default values
	c.CollectOOMKill = true

	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}
	return nil
}

// Configure parses the check configuration and init the check
func (m *OOMKillCheck) Configure(config, initConfig integration.Data, source string) error {
	// TODO: Remove that hard-code and put it somewhere else
	process_net.SetSystemProbePath(dd_config.Datadog.GetString("system_probe_config.sysprobe_socket"))

	err := m.CommonConfigure(config, source)
	if err != nil {
		return err
	}

	return m.instance.Parse(config)
}

// Run executes the check
func (m *OOMKillCheck) Run() error {
	if !m.instance.CollectOOMKill {
		return nil
	}

	sysProbeUtil, err := process_net.GetRemoteSystemProbeUtil()
	if err != nil {
		return err
	}

	data, err := sysProbeUtil.GetCheck("oom_kill")
	if err != nil {
		return err
	}

	// sender is just what is used to submit the data
	sender, err := aggregator.GetSender(m.ID())
	if err != nil {
		return err
	}

	triggerType := ""
	triggerTypeText := ""
	oomkillStats, ok := data.([]oomkill.Stats)
	if !ok {
		return log.Errorf("Raw data has incorrect type")
	}
	for _, line := range oomkillStats {
		entityID := containers.BuildTaggerEntityName(line.ContainerID)
		var tags []string
		if entityID != "" {
			tags, err = tagger.Tag(entityID, tagger.ChecksCardinality)
			if err != nil {
				log.Errorf("Error collecting tags for container %s: %s", line.ContainerID, err)
			}
		}

		if line.MemCgOOM == 1 {
			triggerType = "cgroup"
			triggerTypeText = fmt.Sprintf("This OOM kill was invoked by a cgroup, containerID: %s.", line.ContainerID)
		} else {
			triggerType = "system"
			triggerTypeText = "This OOM kill was invoked by the system."
		}
		tags = append(tags, "trigger_type:"+triggerType)

		tags = append(tags, "trigger_process_name:"+line.FComm)
		tags = append(tags, "process_name:"+line.TComm)

		// submit counter metric
		sender.Count("oom_kill.oom_process.count", 1, "", tags)

		// submit event with a few more details
		event := metrics.Event{
			Priority:       metrics.EventPriorityNormal,
			SourceTypeName: oomKillCheckName,
			EventType:      oomKillCheckName,
			AggregationKey: line.ContainerID,
			Title:          fmt.Sprintf("Process OOM Killed: oom_kill_process called on %s (pid: %d)", line.TComm, line.TPid),
			Tags:           tags,
		}

		var b strings.Builder
		b.WriteString("%%% \n")
		if line.Pid == line.TPid {
			fmt.Fprintf(&b, "Process `%s` (pid: %d) triggered an OOM kill on itself.", line.FComm, line.Pid)
		} else {
			fmt.Fprintf(&b, "Process `%s` (pid: %d) triggered an OOM kill on process `%s` (pid: %d).", line.FComm, line.Pid, line.TComm, line.TPid)
		}
		fmt.Fprintf(&b, "\n The process had reached %d pages in size. \n\n", line.Pages)
		b.WriteString(triggerTypeText)
		b.WriteString("\n %%%")

		event.Text = b.String()
		sender.Event(event)
	}

	sender.Commit()
	return nil
}

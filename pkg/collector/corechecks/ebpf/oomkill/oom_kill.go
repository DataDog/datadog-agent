// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// FIXME: we require the `cgo` build tag because of this dep relationship:
// github.com/DataDog/datadog-agent/pkg/process/net depends on `github.com/DataDog/agent-payload/v5/process`,
// which has a hard dependency on `github.com/DataDog/zstd_0`, which requires CGO.
// Should be removed once `github.com/DataDog/agent-payload/v5/process` can be imported with CGO disabled.
//go:build cgo && linux

// Package oomkill contains the OOMKill check.
package oomkill

import (
	"fmt"
	"strings"

	yaml "gopkg.in/yaml.v2"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/oomkill/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	process_net "github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	// CheckName is the name of the check
	CheckName = "oom_kill"
)

// OOMKillConfig is the config of the OOMKill check
type OOMKillConfig struct {
	CollectOOMKill bool `yaml:"collect_oom_kill"`
}

// OOMKillCheck grabs OOM Kill metrics
type OOMKillCheck struct {
	core.CheckBase
	instance *OOMKillConfig
	tagger   tagger.Component
}

// Factory creates a new check factory
func Factory(tagger tagger.Component) optional.Option[func() check.Check] {
	return optional.NewOption(func() check.Check {
		return newCheck(tagger)
	})
}

func newCheck(tagger tagger.Component) check.Check {
	return &OOMKillCheck{
		CheckBase: core.NewCheckBase(CheckName),
		instance:  &OOMKillConfig{},
		tagger:    tagger,
	}
}

// Parse parses the check configuration
func (c *OOMKillConfig) Parse(data []byte) error {
	// default values
	c.CollectOOMKill = true

	return yaml.Unmarshal(data, c)
}

// Configure parses the check configuration and init the check
func (m *OOMKillCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	err := m.CommonConfigure(senderManager, initConfig, config, source)
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

	sysProbeUtil, err := process_net.GetRemoteSystemProbeUtil(
		pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket"))
	if err != nil {
		return err
	}

	data, err := sysProbeUtil.GetCheck(sysconfig.OOMKillProbeModule)
	if err != nil {
		return err
	}

	// sender is just what is used to submit the data
	sender, err := m.GetSender()
	if err != nil {
		return err
	}

	triggerType := ""
	triggerTypeText := ""
	oomkillStats, ok := data.([]model.OOMKillStats)
	if !ok {
		return log.Errorf("Raw data has incorrect type")
	}
	for _, line := range oomkillStats {
		containerID, err := cgroups.ContainerFilter("", line.CgroupName)
		if err != nil || containerID == "" {
			log.Debugf("Unable to extract containerID from cgroup name: %s, err: %v", line.CgroupName, err)
		}

		entityID := types.NewEntityID(types.ContainerID, containerID)
		var tags []string
		if !entityID.Empty() {
			tags, err = m.tagger.Tag(entityID, m.tagger.ChecksCardinality())
			if err != nil {
				log.Errorf("Error collecting tags for container %s: %s", containerID, err)
			}
		}

		if line.MemCgOOM == 1 {
			triggerType = "cgroup"
			triggerTypeText = fmt.Sprintf("This OOM kill was invoked by a cgroup, containerID: %s.", containerID)
		} else {
			triggerType = "system"
			triggerTypeText = "This OOM kill was invoked by the system."
		}
		tags = append(tags, "trigger_type:"+triggerType)
		tags = append(tags, "trigger_process_name:"+line.TriggerComm)
		tags = append(tags, "process_name:"+line.VictimComm)

		// submit counter metric
		sender.Count("oom_kill.oom_process.count", 1, "", tags)

		// submit event with a few more details
		event := event.Event{
			AlertType:      event.AlertTypeError,
			Priority:       event.PriorityNormal,
			SourceTypeName: CheckName,
			EventType:      CheckName,
			AggregationKey: containerID,
			Title:          fmt.Sprintf("Process OOM Killed: oom_kill_process called on %s (pid: %d)", line.VictimComm, line.VictimPid),
			Tags:           tags,
		}

		var b strings.Builder
		b.WriteString("%%% \n")
		var oomScoreAdj string
		if line.ScoreAdj != 0 {
			oomScoreAdj = fmt.Sprintf(", oom_score_adj: %d", line.ScoreAdj)
		}
		if line.VictimPid == line.TriggerPid {
			fmt.Fprintf(&b, "Process `%s` (pid: %d, oom_score: %d%s) triggered an OOM kill on itself.", line.VictimComm, line.VictimPid, line.Score, oomScoreAdj)
		} else {
			fmt.Fprintf(&b, "Process `%s` (pid: %d) triggered an OOM kill on process `%s` (pid: %d, oom_score: %d%s).", line.TriggerComm, line.TriggerPid, line.VictimComm, line.VictimPid, line.Score, oomScoreAdj)
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

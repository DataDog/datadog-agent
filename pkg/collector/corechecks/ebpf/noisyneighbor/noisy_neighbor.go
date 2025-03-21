// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package noisyneighbor

import (
	"fmt"
	"net/http"

	"gopkg.in/yaml.v2"

	sysprobeclient "github.com/DataDog/datadog-agent/cmd/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// NoisyNeighborConfig is the config of the noisy neighbor check
type NoisyNeighborConfig struct{}

// NoisyNeighborCheck grabs noisy neighbor metrics
type NoisyNeighborCheck struct {
	core.CheckBase
	config         *NoisyNeighborConfig
	tagger         tagger.Component
	sysProbeClient *http.Client
}

// Factory creates a new check factory
func Factory(tagger tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(tagger)
	})
}

func newCheck(tagger tagger.Component) check.Check {
	return &NoisyNeighborCheck{
		CheckBase: core.NewCheckBase(CheckName),
		config:    &NoisyNeighborConfig{},
		tagger:    tagger,
	}
}

// Parse parses the check configuration
func (c *NoisyNeighborConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

// Configure parses the check configuration and init the check
func (n *NoisyNeighborCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if err := n.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}
	if err := n.config.Parse(config); err != nil {
		return fmt.Errorf("noisy_neighbor check config: %s", err)
	}
	n.sysProbeClient = sysprobeclient.Get(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket"))
	return nil
}

// Run executes the check
func (n *NoisyNeighborCheck) Run() error {
	// TODO noisy: use the stats returned here
	stats, err := sysprobeclient.GetCheck[[]model.NoisyNeighborStats](n.sysProbeClient, sysconfig.NoisyNeighborModule)
	if err != nil {
		return fmt.Errorf("get noisy neighbor check: %s", err)
	}

	sender, err := n.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %s", err)
	}

	// TODO noisy: emit your metrics here using `sender`
	for _, stat := range stats {
		containerID := getContainerID(stat.CgroupName)
		prevContainerID := getContainerID(stat.PrevCgroupName)

		var tags []string
		if containerID != "host" {
			entityID := types.NewEntityID(types.ContainerID, containerID)
			if !entityID.Empty() {
				tags, err = n.tagger.Tag(entityID, n.tagger.ChecksCardinality())
				if err != nil {
					log.Errorf("Error collecting tags for container %s: %s", containerID, err)
				}
			}
		}

		tags = append(tags, "container_id:"+containerID)
		sender.Distribution("noisy_neighbor.runq.latency", float64(stat.RunqLatencyNs), "", tags)
		tags = append(tags, "prev_container_id:"+prevContainerID)
		sender.Count("noisy_neighbor.sched.switch.out", 1, "", tags)
	}

	sender.Commit()
	return nil
}

// getContainerID attempts to get container id using the cgroup name.
// If no match is found, container id is set to `host`.
func getContainerID(cgroupName string) string {
	containerID, err := cgroups.ContainerFilter("", cgroupName)
	if err != nil {
		log.Debugf("Unable to extract containerID from cgroup name: %s, err: %v", cgroupName, err)
	}
	if containerID == "" {
		containerID = "host"
	}
	return containerID
}

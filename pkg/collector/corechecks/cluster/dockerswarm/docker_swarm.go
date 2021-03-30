// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package dockerswarm

import (
	"github.com/StackVista/stackstate-agent/pkg/config"
	yaml "gopkg.in/yaml.v2"

	"github.com/StackVista/stackstate-agent/pkg/aggregator"
	"github.com/StackVista/stackstate-agent/pkg/autodiscovery/integration"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	core "github.com/StackVista/stackstate-agent/pkg/collector/corechecks"
	"github.com/StackVista/stackstate-agent/pkg/metrics"
	"github.com/StackVista/stackstate-agent/pkg/util"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
)

// const for swarm check
const (
	SwarmCheckName    = "docker_swarm"
	SwarmServiceCheck = "swarm.service"
)

// SwarmConfig have boolean flag to collect topology
type SwarmConfig struct {
	// sts
	CollectSwarmTopology bool `yaml:"collect_swarm_topology"`
}

// SwarmCheck grabs Swarm topology and replica metrics
type SwarmCheck struct {
	core.CheckBase
	instance       *SwarmConfig
	dockerHostname string
	// sts
	topologyCollector *SwarmTopologyCollector
}

// Run executes the check
func (s *SwarmCheck) Run() error {
	//sts
	// Collect Swarm topology
	if s.instance.CollectSwarmTopology {
		sender, err := aggregator.GetSender(s.ID())
		if err != nil {
			return err
		}

		// try to get the agent hostname to use in the host component
		hostname, err := util.GetHostname()
		if err != nil {
			log.Warnf("Can't get hostname for host running the docker-swarm integration: %s", err)
		}

		log.Infof("Swarm check is enabled and running it")
		err = s.topologyCollector.BuildSwarmTopology(hostname, sender)
		if err != nil {
			sender.ServiceCheck(SwarmServiceCheck, metrics.ServiceCheckCritical, "", nil, err.Error())
			log.Errorf("Could not collect swarm topology: %s", err)
			return err
		}
		sender.Commit()
	} else {
		log.Infof("Swarm check is not enabled to collect topology")
	}

	return nil

}

// Parse the config
func (c *SwarmConfig) Parse(data []byte) error {
	// use STS_COLLECT_SWARM_TOPOLOGY to set the config
	if config.Datadog.IsSet("collect_swarm_topology") {
		c.CollectSwarmTopology = config.Datadog.GetBool("collect_swarm_topology")
	}

	return yaml.Unmarshal(data, c)
}

// Configure parses the check configuration and init the check
func (s *SwarmCheck) Configure(config, initConfig integration.Data) error {
	err := s.CommonConfigure(config)
	if err != nil {
		return err
	}

	err = s.instance.Parse(config)
	if err != nil {
		_ = log.Error("could not parse the config for the Docker Swarm topology check")
		return err
	}
	return nil
}

// SwarmFactory is exported for integration testing
func SwarmFactory() check.Check {
	return &SwarmCheck{
		CheckBase:         core.NewCheckBase(SwarmCheckName),
		instance:          &SwarmConfig{},
		topologyCollector: MakeSwarmTopologyCollector(),
	}
}

func init() {
	core.RegisterCheck(SwarmCheckName, SwarmFactory)
}

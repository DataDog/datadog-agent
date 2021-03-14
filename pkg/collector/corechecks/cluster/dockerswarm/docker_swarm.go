package dockerswarm

import (
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
	sender, err := aggregator.GetSender(s.ID())
	if err != nil {
		return err
	}

	// try to get the agent hostname to use in the host component
	hostname, err := util.GetHostname()
	if err != nil  {
		log.Warnf("Can't get hostname for host running the docker-swarm integration: %s", err)
	}

	//sts
	// Collect Swarm topology
	if s.instance.CollectSwarmTopology {
		log.Infof("Swarm check is enabled and running it")
		err := s.topologyCollector.BuildSwarmTopology(hostname, sender)
		if err != nil {
			sender.ServiceCheck(SwarmServiceCheck, metrics.ServiceCheckCritical, "", nil, err.Error())
			log.Errorf("Could not collect swarm topology: %s", err)
			return err
		}
	} else {
		log.Infof("Swarm check is not enabled to collect topology")
	}

	sender.Commit()
	return nil

}

// Parse the config
func (c *SwarmConfig) Parse(data []byte) error {
	// default values
	c.CollectSwarmTopology = false

	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}
	return nil
}

// Configure parses the check configuration and init the check
func (s *SwarmCheck) Configure(config, initConfig integration.Data) error {
	err := s.CommonConfigure(config)
	if err != nil {
		return err
	}

	s.instance.Parse(config)

	// Use the same hostname as the agent so that host tags (like `availability-zone:us-east-1b`)
	// are attached to Docker events from this host. The hostname from the docker api may be
	// different than the agent hostname depending on the environment (like EC2 or GCE).
	s.dockerHostname, err = util.GetHostname()
	if err != nil {
		log.Warnf("Can't get hostname from docker: %s", err)
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

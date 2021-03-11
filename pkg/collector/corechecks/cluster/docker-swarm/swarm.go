package docker_swarm

import (
	yaml "gopkg.in/yaml.v2"

	"github.com/StackVista/stackstate-agent/pkg/aggregator"
	"github.com/StackVista/stackstate-agent/pkg/autodiscovery/integration"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	core "github.com/StackVista/stackstate-agent/pkg/collector/corechecks"
	"github.com/StackVista/stackstate-agent/pkg/collector/corechecks/cluster/topologycollectors"
	"github.com/StackVista/stackstate-agent/pkg/metrics"
	"github.com/StackVista/stackstate-agent/pkg/util"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
)

const (
	SwarmCheckName = "docker-swarm"
	SwarmServiceCheck = "swarm.service"
)

type SwarmConfig struct {
	// sts
	CollectSwarmTopology     bool `yaml:"collect_swarm_topology"`
}

// DockerCheck grabs docker metrics
type SwarmCheck struct {
	core.CheckBase
	instance                    *SwarmConfig
	dockerHostname              string
	// sts
	topologyCollector *topologycollectors.SwarmTopologyCollector
}

func (s *SwarmCheck) Run() error {
	sender, err := aggregator.GetSender(s.ID())
	if err != nil {
		return err
	}

	//sts
	// Collect Swarm topology
	if s.instance.CollectSwarmTopology {
		log.Infof("Swarm check is enabled and running it")
		err := s.topologyCollector.BuildSwarmTopology(sender)
		if err != nil {
			sender.ServiceCheck(SwarmServiceCheck, metrics.ServiceCheckCritical, "", nil, err.Error())
			log.Errorf("Could not collect swarm topology: %s", err)
			return err
		}
	}

	sender.Commit()
	return nil

}

func (c *SwarmConfig) Parse(data []byte) error {
	// default values
	c.CollectSwarmTopology = true

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
		topologyCollector: topologycollectors.MakeSwarmTopologyCollector(),
	}
}

func init() {
	core.RegisterCheck(SwarmCheckName, SwarmFactory)
}

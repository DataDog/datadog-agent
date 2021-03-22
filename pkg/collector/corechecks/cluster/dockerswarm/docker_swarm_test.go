package dockerswarm

import (
	"github.com/StackVista/stackstate-agent/pkg/aggregator/mocksender"
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	"os"
	"testing"
)

func TestDockerSwarmCheck_True(t *testing.T) {

	swarmcheck := MockSwarmFactory()
	// set mock hostname
	testHostname := "mock-host"
	config.Datadog.Set("hostname", testHostname)
	// set up the mock batcher
	mockBatcher := batcher.NewMockBatcher()
	// Setup mock sender
	sender := mocksender.NewMockSender(swarmcheck.ID())
	sender.On("Gauge", "swarm.service.running_replicas", 2.0, "", []string{"serviceName:agent_stackstate-agent"}).Return().Times(1)
	sender.On("Gauge", "swarm.service.desired_replicas", 2.0, "", []string{"serviceName:agent_stackstate-agent"}).Return().Times(1)
	sender.On("Commit").Return().Times(1)

	// set test configuration
	testConfig := map[string]interface{}{
		"collect_swarm_topology": true,
	}
	config, err := yaml.Marshal(testConfig)
	assert.NoError(t, err)
	swarmcheck.Configure(config, nil)
	swarmcheck.Run()

	producedTopology := mockBatcher.CollectedTopology.Flush()
	expectedTopology := batcher.Topologies{
		"swarm_topology": {
			StartSnapshot: false,
			StopSnapshot:  false,
			Instance:      topology.Instance{Type: "docker-swarm", URL: "agents"},
			Components: []topology.Component{
				*serviceComponent,
				*containerComponent,
			},
			Relations: []topology.Relation{
				*serviceRelation,
			},
		},
	}
	assert.EqualValues(t, expectedTopology, producedTopology)
	sender.AssertExpectations(t)
}

func TestDockerSwarmCheck_FromEnv(t *testing.T) {

	swarmcheck := MockSwarmFactory().(*SwarmCheck)
	// force CollectSwarmTopology to false
	swarmcheck.instance.CollectSwarmTopology = false

	// set environment for STS_COLLECT_SWARM_TOPOLOGY
	os.Setenv("DD_COLLECT_SWARM_TOPOLOGY", "true")

	// set mock hostname
	testHostname := "mock-host"
	config.Datadog.Set("hostname", testHostname)
	// set up the mock batcher
	mockBatcher := batcher.NewMockBatcher()
	// Setup mock sender
	sender := mocksender.NewMockSender(swarmcheck.ID())
	sender.On("Gauge", "swarm.service.running_replicas", 2.0, "", []string{"serviceName:agent_stackstate-agent"}).Return().Times(1)
	sender.On("Gauge", "swarm.service.desired_replicas", 2.0, "", []string{"serviceName:agent_stackstate-agent"}).Return().Times(1)
	sender.On("Commit").Return().Times(1)

	swarmcheck.Configure(nil, nil)
	swarmcheck.Run()

	producedTopology := mockBatcher.CollectedTopology.Flush()
	expectedTopology := batcher.Topologies{
		"swarm_topology": {
			StartSnapshot: false,
			StopSnapshot:  false,
			Instance:      topology.Instance{Type: "docker-swarm", URL: "agents"},
			Components: []topology.Component{
				*serviceComponent,
				*containerComponent,
			},
			Relations: []topology.Relation{
				*serviceRelation,
			},
		},
	}
	assert.EqualValues(t, expectedTopology, producedTopology)
	sender.AssertExpectations(t)

	os.Unsetenv("DD_COLLECT_SWARM_TOPOLOGY")
}

func TestDockerSwarmCheck_False(t *testing.T) {

	swarmcheck := SwarmFactory().(*SwarmCheck)
	swarmcheck.Configure(nil, nil)

	// set up the mock batcher
	mockBatcher := batcher.NewMockBatcher()
	// set mock hostname
	testHostname := "mock-host"
	config.Datadog.Set("hostname", testHostname)
	// Setup mock sender
	sender := mocksender.NewMockSender(swarmcheck.ID())
	sender.On("Commit").Return().Times(1)

	swarmcheck.Run()

	producedTopology := mockBatcher.CollectedTopology.Flush()
	expectedTopology := batcher.Topologies{}
	// since instance flag is not true, no topology will be collected by default
	assert.EqualValues(t, expectedTopology, producedTopology)
}

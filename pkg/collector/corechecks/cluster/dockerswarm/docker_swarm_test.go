package dockerswarm

import (
	"github.com/StackVista/stackstate-agent/pkg/aggregator/mocksender"
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDockerSwarmCheck_True(t *testing.T) {

	swarmcheck := SwarmFactory().(*SwarmCheck)
	swarmcheck.instance.CollectSwarmTopology = true
	swarmcheck.topologyCollector = makeSwarmTopologyCollector(&MockSwarmClient{})
	swarmcheck.Configure(nil, nil)

	// set up the mock batcher
	mockBatcher := batcher.NewMockBatcher()
	// set mock hostname
	testHostname := "mock-host"
	config.Datadog.Set("hostname", testHostname)
	// Setup mock sender
	sender := mocksender.NewMockSender(swarmcheck.ID())
	sender.On("Gauge", "swarm.service.running_replicas", 2.0, "", []string{"serviceName:agent_stackstate-agent"}).Return().Times(1)
	sender.On("Gauge", "swarm.service.desired_replicas", 2.0, "", []string{"serviceName:agent_stackstate-agent"}).Return().Times(1)
	sender.On("Commit").Return().Times(1)
	swarmcheck.Run()

	producedTopology := mockBatcher.CollectedTopology.Flush()
	expectedTopology := batcher.Topologies{
		"docker_swarm": {
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

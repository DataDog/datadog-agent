package dockerswarm

import (
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDockerSwarmCheck(t *testing.T) {

	swarmCheck := SwarmFactory()
	swarmCheck.Configure(nil, nil)

	// set up the mock batcher
	mockBatcher := batcher.NewMockBatcher()
	// set mock hostname
	testHostname := "mock-host"
	config.Datadog.Set("hostname", testHostname)

	swarmCheck.Run()

	producedTopology := mockBatcher.CollectedTopology.Flush()
	expectedTopology := batcher.Topologies{
		"docker_swarm": {
			StartSnapshot: false,
			StopSnapshot:  false,
			Instance:      topology.Instance{Type: "docker-swarm", URL: "agents"},
			Components: []topology.Component{
				serviceComponent,
				containerComponent,
			},
			Relations: []topology.Relation{
				serviceRelation,
			},
		},
	}

	assert.Equal(t, expectedTopology, producedTopology)
}

//func TestDockerSwarm(t *testing.T) {
//	swarmCheck := SwarmFactory()
//
//	// Setup mock sender
//	sender := mocksender.NewMockSender(swarmCheck.ID())
//	sender.SetupAcceptAll()
//
//	// Setup mock batcher
//	_ = batcher.NewMockBatcher()
//
//
//	swarmTopology := makeSwarmTopologyCollector(&MockSwarmClient{})
//
//	components, relations, err := swarmTopology.collectSwarmServices(sender)
//	assert.NoError(t, err)
//
//
//	expectedComponents := []*topology.Component{}
//	expectedRelations := []*topology.Relation{}
//	assert.EqualValues(t, expectedComponents, components)
//	assert.EqualValues(t, expectedRelations, relations)
//}

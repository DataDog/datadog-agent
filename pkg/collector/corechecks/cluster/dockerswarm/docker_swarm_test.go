package dockerswarm

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/stretchr/testify/mock"
	"testing"
)

func TestDockerSwarmCheck(t *testing.T)  {

	swarmCheck := SwarmFactory().(*SwarmCheck)
	swarmCheck.Configure(nil, nil)

	// set up the mock batcher
	mockBatcher := batcher.NewMockBatcher()

	swarmCheck.Run()

	producedTopology := mockBatcher.CollectedTopology.Flush()
	expectedTopology := batcher.Topologies{
		"dock_swarm_topology": {
			StartSnapshot: false,
			StopSnapshot:  false,
			Instance:      topology.Instance{Type: "disk", URL: "agents"},
			Components: []topology.Component{
				{
					ExternalID: fmt.Sprintf("urn:swarm-service:/%s", ),
					Type: topology.Type{
						Name: "swarm-service",
					},
					Data: topology.Data{
					},
				},
			},
			Relations: []topology.Relation{},
		},
	}

	assert.Equal(t, expectedTopology, producedTopology)

}

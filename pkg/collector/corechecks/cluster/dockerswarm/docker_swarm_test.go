package dockerswarm

import (
	"github.com/StackVista/stackstate-agent/pkg/aggregator/mocksender"
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDockerSwarm(t *testing.T) {
	swarmCheck := SwarmFactory()

	// Setup mock sender
	sender := mocksender.NewMockSender(swarmCheck.ID())
	sender.SetupAcceptAll()

	// Setup mock batcher
	_ = batcher.NewMockBatcher()


	swarmTopology := makeSwarmTopologyCollector(&MockSwarmClient{})

	components, relations, err := swarmTopology.collectSwarmServices(sender)
	assert.NoError(t, err)


	expectedComponents := []*topology.Component{}
	expectedRelations := []*topology.Relation{}
	assert.EqualValues(t, expectedComponents, components)
	assert.EqualValues(t, expectedRelations, relations)
}

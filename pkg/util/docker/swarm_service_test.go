package docker

import (
	"github.com/docker/docker/api/types/swarm"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDockerUtil_getActiveNodes(t *testing.T) {

	mockSwarmServiceClient := &mockSwarmServiceAPIClient{
		nodeList: func() ([]swarm.Node, error) {
			swarmNodes := []swarm.Node{
				{
					ID:            "Node-NodeStateDown",
					Status:        swarm.NodeStatus{
						State:   swarm.NodeStateDown,
					},
				},
				{
					ID:            "Node-NodeStateUnknown",
					Status:        swarm.NodeStatus{
						State:   swarm.NodeStateUnknown,
					},
				},
				{
					ID:            "Node-NodeStateReady",
					Status:        swarm.NodeStatus{
						State:   swarm.NodeStateReady,
					},
				},
				{
					ID:            "Node-NodeStateDisconnected",
					Status:        swarm.NodeStatus{
						State:   swarm.NodeStateDisconnected,
					},
				},
			}
			return swarmNodes, nil
		},
	}

	nodeMap, err := getActiveNodes(nil, mockSwarmServiceClient)
	assert.NoError(t, err)

	expectedNodeMap := map[string]bool{
		"Node-NodeStateReady":true,
	}
	assert.EqualValues(t, expectedNodeMap, nodeMap)

}

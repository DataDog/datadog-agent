package dockerswarm

import (
	"github.com/StackVista/stackstate-agent/pkg/aggregator/mocksender"
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/stretchr/testify/assert"
	"testing"
)

var (
	serviceComponent = topology.Component{
		ExternalID: "urn:swarm-service:/klbo61rrhksdmc9ho3pq97t6e",
		Type: topology.Type{
			Name: swarmServiceType,
		},
		Data: topology.Data{
			"name":    swarmService.Name,
			"image":   swarmService.ContainerImage,
			"tags":    swarmService.Labels,
			"version": swarmService.Version,
			"created": swarmService.CreatedAt,
		},
	}
	containerComponent = topology.Component{
		ExternalID: "urn:container:/a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406",
		Type:       topology.Type{Name: "container"},
		Data: topology.Data{
			"TaskID": swarmService.TaskContainers[0].ID,
			"name":   swarmService.TaskContainers[0].Name,
			"image":  swarmService.TaskContainers[0].ContainerImage,
			"status": swarmService.TaskContainers[0].ContainerStatus,
		},
	}
	serviceRelation = topology.Relation{
		ExternalID: "urn:swarm-service:/klbo61rrhksdmc9ho3pq97t6e-urn:container:/a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406",
		SourceID:   "urn:swarm-service:/klbo61rrhksdmc9ho3pq97t6e",
		TargetID:   "urn:container:/a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406",
		Type:       topology.Type{Name: "creates"},
		Data:       topology.Data{},
	}
)

func TestMakeSwarmTopologyCollector(t *testing.T) {
	st := makeSwarmTopologyCollector(&MockSwarmClient{})
	assert.Equal(t, check.ID("swarm_topology"), st.CheckID)
	expectedInstance := topology.Instance{
		Type: "docker-swarm",
		URL:  "agents",
	}
	assert.Equal(t, expectedInstance, st.TopologyInstance)
}

func TestSwarmTopologyCollector_CollectSwarmServices(t *testing.T) {
	st := makeSwarmTopologyCollector(&MockSwarmClient{})

	// Setup mock sender
	sender := mocksender.NewMockSender(st.CheckID)
	sender.SetupAcceptAll()

	comps, relations, err := st.collectSwarmServices(sender)
	serviceComponents := []topology.Component{
		{
			ExternalID: "urn:swarm-service:/klbo61rrhksdmc9ho3pq97t6e",
			Type: topology.Type{
				Name: swarmServiceType,
			},
			Data: topology.Data{
				"name":    swarmService.Name,
				"image":   swarmService.ContainerImage,
				"tags":    swarmService.Labels,
				"version": swarmService.Version,
				"created": swarmService.CreatedAt,
			},
		},
	}
	containerComponents := []topology.Component{
		{
			ExternalID: "urn:container:/a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406",
			Type:       topology.Type{Name: "container"},
			Data: topology.Data{
				"TaskID": swarmService.TaskContainers[0].ID,
				"name":   swarmService.TaskContainers[0].Name,
				"image":  swarmService.TaskContainers[0].ContainerImage,
				"status": swarmService.TaskContainers[0].ContainerStatus,
			},
		},
	}
	serviceRelations := []topology.Relation{
		{
			ExternalID: "urn:swarm-service:/klbo61rrhksdmc9ho3pq97t6e-urn:container:/a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406",
			SourceID:   "urn:swarm-service:/klbo61rrhksdmc9ho3pq97t6e",
			TargetID:   "urn:container:/a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406",
			Type:       topology.Type{Name: "creates"},
			Data:       topology.Data{},
		},
	}
	// append container components to service components
	serviceComponents = append(serviceComponents, containerComponents...)
	// error should be nil
	assert.Equal(t, err, nil)
	// components should be serviceComponents
	assert.Equal(t, comps, serviceComponents)
	// relations should be serviceRelations
	assert.Equal(t, relations, serviceRelations)
	// check for produced metrics
	sender.On("Gauge", "swarm.service.running_replicas", 2.0, "", []string{"serviceName:agent_stackstate-agent"}).Return().Times(1)
	sender.On("Gauge", "swarm.service.desired_replicas", 2.0, "", []string{"serviceName:agent_stackstate-agent"}).Return().Times(1)
	sender.AssertExpectations(t)
}

func TestSwarmTopologyCollector_BuildSwarmTopology(t *testing.T) {
	st := makeSwarmTopologyCollector(&MockSwarmClient{})
	// Setup mock sender
	sender := mocksender.NewMockSender(st.CheckID)
	sender.SetupAcceptAll()
	// set up the mock batcher
	mockBatcher := batcher.NewMockBatcher()

	err := st.BuildSwarmTopology(sender)
	assert.NoError(t, err)

	producedTopology := mockBatcher.CollectedTopology.Flush()
	expectedTopology := batcher.Topologies{
		"swarm_topology": {
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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package dockerswarm

import (
	"github.com/StackVista/stackstate-agent/pkg/aggregator/mocksender"
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/stretchr/testify/assert"
	"testing"
)

var (
	serviceComponent = &topology.Component{
		ExternalID: "urn:swarm-service:/klbo61rrhksdmc9ho3pq97t6e",
		Type: topology.Type{
			Name: swarmServiceType,
		},
		Data: topology.Data{
			"name":         swarmService.Name,
			"image":        swarmService.ContainerImage,
			"tags":         swarmService.Labels,
			"version":      swarmService.Version.Index,
			"created":      swarmService.CreatedAt,
			"spec":         swarmService.Spec,
			"endpoint":     swarmService.Endpoint,
			"updateStatus": swarmService.UpdateStatus,
			"updated":      swarmService.UpdatedAt,
			"clusterName":  "agent-swarm",
		},
	}
	containerComponent = &topology.Component{
		ExternalID: "urn:container:/a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406",
		Type:       topology.Type{Name: "docker-container"},
		Data: topology.Data{
			"TaskID":      swarmService.TaskContainers[0].ID,
			"name":        swarmService.TaskContainers[0].Name,
			"image":       swarmService.TaskContainers[0].ContainerImage,
			"status":      swarmService.TaskContainers[0].ContainerStatus,
			"spec":        swarmService.TaskContainers[0].ContainerSpec,
			"state":       swarmService.TaskContainers[0].DesiredState,
			"identifiers": []string{"urn:container:/mock-host:a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406"},
		},
	}
	serviceRelation = &topology.Relation{
		ExternalID: "urn:swarm-service:/klbo61rrhksdmc9ho3pq97t6e->urn:container:/a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406",
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
	// set mock hostname
	testHostname := "mock-host"
	config.Datadog.Set("hostname", testHostname)
	// set mock cluster name
	config.Datadog.Set("cluster_name", "agent-swarm")
	expectedTags := []string{"serviceName:agent_stackstate-agent", "clusterName:agent-swarm"}
	// check for produced metrics
	sender.On("Gauge", "swarm.service.running_replicas", 2.0, "", expectedTags).Return().Times(1)
	sender.On("Gauge", "swarm.service.desired_replicas", 2.0, "", expectedTags).Return().Times(1)
	comps, relations, err := st.collectSwarmServices(testHostname, sender)

	// list of swamr service components
	serviceComponents := []*topology.Component{
		serviceComponent,
	}
	// list of swamr task container components
	containerComponents := []*topology.Component{
		containerComponent,
	}
	// list of swamr service and task container relation
	serviceRelations := []*topology.Relation{
		serviceRelation,
	}
	// append container components to service components
	serviceComponents = append(serviceComponents, containerComponents...)
	// error should be nil
	assert.Equal(t, err, nil)
	// components should be serviceComponents
	assert.EqualValues(t, comps, serviceComponents)
	// relations should be serviceRelations
	assert.EqualValues(t, relations, serviceRelations)
	// metrics assertion
	sender.AssertExpectations(t)
	sender.AssertNumberOfCalls(t, "Gauge", 2)

}

func TestSwarmTopologyCollector_BuildSwarmTopology(t *testing.T) {
	st := makeSwarmTopologyCollector(&MockSwarmClient{})
	// Setup mock sender
	sender := mocksender.NewMockSender(st.CheckID)
	// set up the mock batcher
	mockBatcher := batcher.NewMockBatcher()
	// set mock hostname
	testHostname := "mock-host"
	config.Datadog.Set("hostname", testHostname)
	// set mock cluster name
	config.Datadog.Set("cluster_name", "agent-swarm")
	expectedTags := []string{"serviceName:agent_stackstate-agent", "clusterName:agent-swarm"}
	// check for produced metrics
	sender.On("Gauge", "swarm.service.running_replicas", 2.0, "", expectedTags).Return().Times(1)
	sender.On("Gauge", "swarm.service.desired_replicas", 2.0, "", expectedTags).Return().Times(1)

	err := st.BuildSwarmTopology(testHostname, sender)
	assert.NoError(t, err)

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
	assert.EqualValues(t, producedTopology, expectedTopology)
	// metrics assertion
	sender.AssertExpectations(t)
	sender.AssertNumberOfCalls(t, "Gauge", 2)
}

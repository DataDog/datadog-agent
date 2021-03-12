package dockerswarm

import (
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/containers"
	"github.com/StackVista/stackstate-agent/pkg/util/docker"
	"github.com/docker/docker/api/types/swarm"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestMakeSwarmTopologyCollector(t *testing.T) {
	st := MakeSwarmTopologyCollector()
	assert.Equal(t, check.ID("swarm_topology"), st.CheckID)
	expectedInstance := topology.Instance{
		Type: "docker-swarm",
		URL:  "agents",
	}
	assert.Equal(t, expectedInstance, st.TopologyInstance)
}

func TestSwarmTopologyCollector_SwarmServices(t *testing.T) {
	st := MakeSwarmTopologyCollector()
	du := docker.GetDockerUtil()
	swarmServices := []containers.SwarmService{
		{
			ID:             "klbo61rrhksdmc9ho3pq97t6e",
			Name:           "agent_stackstate-agent",
			ContainerImage: "stackstate/stackstate-agent-2-test:stac-12057-swarm-topology@sha256:1d463af3e8c407e08bff9f6127e4959d5286a25018ec5269bfad5324815eb367",
			Labels: 		map[string]string{
								"com.docker.stack.image": "docker.io/stackstate/stackstate-agent-2-test:stac-12057-swarm-topology",
								"com.docker.stack.namespace": "agent",
							},
			Version:        swarm.Version{Index: uint64(136),},
			//CreatedAt:      time.Time{"2021-03-11T08:01:46.718350483Z"},
			CreatedAt:      time.Date(2021, time.March, 10, 23, 0, 0, 0, time.UTC),
			UpdatedAt:      time.Date(2021, time.March, 10, 45, 0, 0, 0, time.UTC),
			TaskContainers: []*containers.SwarmTask{
				{
					ID: "qwerty12345",
					Name: "/agent_stackstate-agent.1.skz8sp5d1y4f64qykw37mf3k2",
					ContainerImage: "stackstate/stackstate-agent-2-test",
					ContainerStatus: swarm.ContainerStatus{
						ContainerID: "a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406",
						ExitCode: 0,
						PID: 341,
					},
				},
			},
			DesiredTasks: 	2,
			RunningTasks: 	2,
		},
	}
	serviceComponents := []topology.Component{
		{
			ExternalID: "urn:swarm-service:/klbo61rrhksdmc9ho3pq97t6e",
			Type: topology.Type{
				Name: swarmServiceType,
			},
			Data: topology.Data{
				"name": swarmServices[0].Name,
				"image": swarmServices[0].ContainerImage,
				"tags": swarmServices[0].Labels,
				"version": swarmServices[0].Version,
				"created":      swarmServices[0].CreatedAt,
				"spec":         nil,
				"endpoint":     nil,
				"updateStatus": nil,
			},
		},
	}
	containerComponents := []topology.Component{
		{
			ExternalID: "urn:container:/a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406",
			Type:       topology.Type{Name: "container"},
			Data: topology.Data{
				"TaskID": 		swarmServices[0].TaskContainers[0].ID,
				"name":         swarmServices[0].TaskContainers[0].Name,
				"image":        swarmServices[0].TaskContainers[0].ContainerImage,
				"spec":			nil,
				"status":     	swarmServices[0].TaskContainers[0].ContainerStatus,
			},
		},
	}
	serviceRelations := []topology.Relation{
		{
			ExternalID: "urn:swarm-service:/klbo61rrhksdmc9ho3pq97t6e-urn:container:/a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406",
			SourceID: "urn:swarm-service:/klbo61rrhksdmc9ho3pq97t6e",
			TargetID: "urn:container:/a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406",
			Type: topology.Type{
				Name: "creates",
			},
			Data: topology.Data{},
		},
	}
	// To DO How to assign responses for a function for mocking
	du.ListSwarmServices := swarmServices


}

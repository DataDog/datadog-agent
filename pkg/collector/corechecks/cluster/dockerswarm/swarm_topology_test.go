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
	//serviceComponent1 = topology.Component{
	//	ExternalID: "urn:swarm-service:/klbo61rrhksdmc9ho3pq97t6e",
	//	Type: topology.Type{
	//		Name: swarmServiceType,
	//	},
	//	Data: topology.Data{
	//		"image":"stackstate/stackstate-agent-2-test:stac-12057-swarm-topology@sha256:1d463af3e8c407e08bff9f6127e4959d5286a25018ec5269bfad5324815eb367",
	//		"version":0x88,
	//		"spec":swarm.ServiceSpec{
	//			Annotations:swarm.Annotations{
	//				Name:"",
	//				Labels:map[string]string(nil),
	//			},
	//			TaskTemplate:swarm.TaskSpec{
	//				ContainerSpec:swarm.ContainerSpec{
	//					Image:"",
	//					Labels:map[string]string(nil),
	//					Command:[]string(nil),
	//					Args:[]string(nil),
	//					Hostname:"",
	//					Env:[]string(nil),
	//					Dir:"",
	//					User:"",
	//					Groups:[]string(nil),
	//					TTY:false,
	//					OpenStdin:false,
	//					Mounts:[]mount.Mount(nil),
	//					StopGracePeriod:(*time.Duration)(nil),
	//					Healthcheck:(*container.HealthConfig)(nil),
	//					Hosts:[]string(nil),
	//					DNSConfig:(*swarm.DNSConfig)(nil),
	//					Secrets:[]*swarm.SecretReference(nil),
	//				},
	//				Resources:(*swarm.ResourceRequirements)(nil),
	//				RestartPolicy:(*swarm.RestartPolicy)(nil),
	//				Placement:(*swarm.Placement)(nil),
	//				Networks:[]swarm.NetworkAttachmentConfig(nil),
	//				LogDriver:(*swarm.Driver)(nil),
	//				ForceUpdate:0x0,
	//			},
	//			Mode:swarm.ServiceMode{
	//				Replicated:(*swarm.ReplicatedService)(nil),
	//				Global:(*swarm.GlobalService)(nil),
	//			},
	//			UpdateConfig:(*swarm.UpdateConfig)(nil),
	//			Networks:[]swarm.NetworkAttachmentConfig(nil),
	//			EndpointSpec:(*swarm.EndpointSpec)(nil),
	//		},
	//		"endpoint":swarm.Endpoint{
	//			Spec:swarm.EndpointSpec{
	//				Mode:"",
	//				Ports:[]swarm.PortConfig(nil),
	//			},
	//			Ports:[]swarm.PortConfig(nil),
	//			VirtualIPs:[]swarm.EndpointVirtualIP(nil),
	//		},
	//		"name":"agent_stackstate-agent",
	//		"tags":map[string]string{
	//			"com.docker.stack.image":"docker.io/stackstate/stackstate-agent-2-test:stac-12057-swarm-topology",
	//			"com.docker.stack.namespace":"agent",
	//		},
	//		"updateStatus":swarm.UpdateStatus{
	//			State:"",
	//			Message:"",
	//		},
	//	},
	//}
	containerComponent = topology.Component{
		ExternalID: "urn:container:/a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406",
		Type:       topology.Type{Name: "docker container"},
		Data: topology.Data{
			"TaskID": swarmService.TaskContainers[0].ID,
			"name":   swarmService.TaskContainers[0].Name,
			"image":  swarmService.TaskContainers[0].ContainerImage,
			"status": swarmService.TaskContainers[0].ContainerStatus,
			"identifiers": []string{"urn:container:/mock-host:a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406"},
		},
	}
	serviceRelation = topology.Relation{
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
	// check for produced metrics
	sender.On("Gauge", "swarm.service.running_replicas", 2.0, "", []string{"serviceName:agent_stackstate-agent"}).Return().Times(1)
	sender.On("Gauge", "swarm.service.desired_replicas", 2.0, "", []string{"serviceName:agent_stackstate-agent"}).Return().Times(1)
	sender.On("Commit").Return().Times(1)
	comps, relations, err := st.collectSwarmServices(testHostname, sender)
	serviceComponents := []*topology.Component{
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
	containerComponents := []*topology.Component{
		{
			ExternalID: "urn:container:/a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406",
			Type:       topology.Type{Name: "docker container"},
			Data: topology.Data{
				"TaskID": swarmService.TaskContainers[0].ID,
				"name":   swarmService.TaskContainers[0].Name,
				"image":  swarmService.TaskContainers[0].ContainerImage,
				"status": swarmService.TaskContainers[0].ContainerStatus,
				"identifiers": []string{"urn:container:/mock-host:a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406"},
			},
		},
	}
	serviceRelations := []*topology.Relation{
		{
			ExternalID: "urn:swarm-service:/klbo61rrhksdmc9ho3pq97t6e->urn:container:/a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406",
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

	err := st.BuildSwarmTopology(testHostname, sender)
	assert.NoError(t, err)

	producedTopology := mockBatcher.CollectedTopology.Flush()
	//expectedTopology := batcher.Topologies{
	//	"swarm_topology": {
	//		StartSnapshot: false,
	//		StopSnapshot:  false,
	//		Instance:      topology.Instance{Type: "docker-swarm", URL: "agents"},
	//		Components: []topology.Component{
	//			serviceComponent,
	//			containerComponent,
	//		},
	//		Relations: []topology.Relation{
	//			serviceRelation,
	//		},
	//	},
	//}


	assert.Equal(t, producedTopology, 1)

}

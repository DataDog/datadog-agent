package topologycollectors

import (
	"errors"
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/aggregator"
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/collector/corechecks"
	docker_swarm "github.com/StackVista/stackstate-agent/pkg/collector/corechecks/cluster/docker-swarm"
	"github.com/StackVista/stackstate-agent/pkg/metrics"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/docker"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
)

const (
	SwarmTopologyCheckName = "swarm_topology"
	swarmServiceType        = "swarm-service"
)

// DockerTopologyCollector contains the checkID and topology instance for the docker topology check
type SwarmTopologyCollector struct {
	corechecks.CheckTopologyCollector
}

// MakeDockerTopologyCollector returns a new instance of DockerTopologyCollector
func MakeSwarmTopologyCollector() *SwarmTopologyCollector {
	return &SwarmTopologyCollector{
		corechecks.MakeCheckTopologyCollector(SwarmTopologyCheckName, topology.Instance{
			Type: "docker-swarm",
			URL:  "agents",
		}),
	}
}

// BuildSwarmTopology collects and produces all docker swarm topology
func (dt *SwarmTopologyCollector) BuildSwarmTopology(metrics aggregator.Sender) error {
	sender := batcher.GetBatcher()
	if sender == nil {
		return errors.New("no batcher instance available, skipping BuildSwarmTopology")
	}

	// collect all swarm services as topology components
	swarmComponents, swarmRelations, err := dt.collectSwarmServices(metrics)
	if err != nil {
		return err
	}

	// submit all collected topology components
	for _, component := range swarmComponents {
		sender.SubmitComponent(dt.CheckID, dt.TopologyInstance, *component)
	}
	// submit all collected topology relations
	for _, relation := range swarmRelations {
		sender.SubmitRelation(dt.CheckID, dt.TopologyInstance, *relation)
	}

	sender.SubmitComplete(dt.CheckID)

	return nil
}

// collectSwarmServices collects swarm services from the docker util and produces topology.Component
func (dt *SwarmTopologyCollector) collectSwarmServices(sender aggregator.Sender) ([]*topology.Component, []*topology.Relation, error) {
	du, err := docker.GetDockerUtil()
	if err != nil {
		sender.ServiceCheck(docker_swarm.SwarmServiceCheck, metrics.ServiceCheckCritical, "", nil, err.Error())
		log.Warnf("Error initialising check: %s", err)
		return nil, nil, err
	}
	sList, err := du.ListSwarmServices()
	if err != nil {
		return nil, nil, err
	}

	taskContainerComponents := make([]*topology.Component, 0)
	swarmServiceComponents := make([]*topology.Component, 0)
	swarmServiceRelations := make([]*topology.Relation, 0)
	for _, s := range sList {
		tags := make([]string, 0)
		// ------------ Create a component structure for Swarm Service
		sourceExternalID := fmt.Sprintf("urn:%s:/%s", swarmServiceType, s.ID)
		swarmServiceComponent := &topology.Component{
			ExternalID: sourceExternalID,
			Type:       topology.Type{Name: swarmServiceType},
			Data: topology.Data{
				"name":         s.Name,
				"image":        s.ContainerImage,
				"tags":         s.Labels,
				"version":      s.Version.Index,
				"created":      s.CreatedAt,
				"spec":         s.Spec,
				"endpoint":     s.Endpoint,
				"updateStatus": s.UpdateStatus,
			},
		}

		// add updated time when it's present
		if !s.UpdatedAt.IsZero() {
			swarmServiceComponent.Data["updated"] = s.UpdatedAt
		}

		// add previous spec if there is one
		if s.PreviousSpec != nil {
			swarmServiceComponent.Data["previousSpec"] = s.PreviousSpec
		}

		swarmServiceComponents = append(swarmServiceComponents, swarmServiceComponent)

		// ------------ Create a component structure for Swarm Task Container
		for _, taskContainer := range s.TaskContainers {
			targetExternalID := fmt.Sprintf("urn:container:/%s", taskContainer.ContainerStatus.ContainerID)
			taskContainerComponent := &topology.Component{
				ExternalID: targetExternalID,
				Type:       topology.Type{Name: "container"},
				Data: topology.Data{
					"TaskID": 		taskContainer.ID,
					"name":         taskContainer.Name,
					"image":        taskContainer.ContainerImage,
					"spec":         taskContainer.ContainerSpec,
					"status":     	taskContainer.ContainerStatus,
				},
			}
			taskContainerComponents = append(taskContainerComponents, taskContainerComponent)
			log.Infof("Creating a relation for service %s with container %s", s.Name, taskContainer.Name)
			swarmServiceRelation := &topology.Relation{
				ExternalID: fmt.Sprintf("%s->%s", sourceExternalID, targetExternalID),
				SourceID:   sourceExternalID,
				TargetID:   targetExternalID,
				Type: 		topology.Type{Name: "creates"},
				Data: 		topology.Data{},
			}
			swarmServiceRelations = append(swarmServiceRelations, swarmServiceRelation)
		}


		sender.Gauge("docker.service.running_replicas", float64(s.RunningTasks), "", append(tags, "serviceName:"+s.Name))
		sender.Gauge("docker.service.desired_replicas", float64(s.DesiredTasks), "", append(tags, "serviceName:"+s.Name))

	}
	// Append TaskContainer components to same Service Component list
	swarmServiceComponents = append(swarmServiceComponents, taskContainerComponents...)

	return swarmServiceComponents, swarmServiceRelations, nil
}

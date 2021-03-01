package topology

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/collector/corechecks"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/docker"
	"github.com/docker/docker/api/types"
)

const (
	dockerTopologyCheckName = "docker_topology"
	containerType           = "container"
	swarmServiceType        = "swarm-service"
)

// DockerTopologyCollector contains the checkID and topology instance for the docker topology check
type DockerTopologyCollector struct {
	corechecks.CheckTopologyCollector
}

// Container represents a single container on a machine.
type Container struct {
	Type   string
	ID     string
	Name   string
	Mounts []types.MountPoint
}

// MakeDockerTopologyCollector returns a new instance of DockerTopologyCollector
func MakeDockerTopologyCollector() *DockerTopologyCollector {
	return &DockerTopologyCollector{
		corechecks.MakeCheckTopologyCollector(dockerTopologyCheckName, topology.Instance{
			Type: "docker",
			URL:  "agents",
		}),
	}
}

// BuildTopology collects all docker topology
func (dt *DockerTopologyCollector) BuildContainerTopology(du *docker.DockerUtil) error {
	sender := batcher.GetBatcher()

	// collect all containers as topology components
	containerComponents, err := dt.collectContainers(du)
	if err != nil {
		return err
	}

	// submit all collected topology components
	for _, component := range containerComponents {
		sender.SubmitComponent(dt.CheckID, dt.TopologyInstance, *component)
	}

	sender.SubmitComplete(dt.CheckID)

	return nil
}

// BuildSwarmTopology collects and produces all docker swarm topology
func (dt *DockerTopologyCollector) BuildSwarmTopology(du *docker.DockerUtil) error {
	sender := batcher.GetBatcher()

	// collect all swarm services as topology components
	swarmServices, err := dt.collectSwarmServices(du)
	if err != nil {
		return err
	}

	// submit all collected topology components
	for _, component := range swarmServices {
		sender.SubmitComponent(dt.CheckID, dt.TopologyInstance, *component)
	}

	sender.SubmitComplete(dt.CheckID)

	return nil
}

// collectContainers collects containers from the docker util and produces topology.Component
func (dt *DockerTopologyCollector) collectContainers(du *docker.DockerUtil) ([]*topology.Component, error) {
	cList, err := du.ListContainers(&docker.ContainerListConfig{IncludeExited: false, FlagExcluded: true})
	if err != nil {
		return nil, err
	}

	containerComponents := make([]*topology.Component, 0)
	for _, ctr := range cList {
		containerComponent := &topology.Component{
			ExternalID: fmt.Sprintf("urn:%s:/%s", containerType, ctr.ID),
			Type:       topology.Type{Name: containerType},
			Data: topology.Data{
				"type":        ctr.Type,
				"containerID": ctr.ID,
				"name":        ctr.Name,
				"image":       ctr.Image,
				"mounts":      ctr.Mounts,
				"state":       ctr.State,
				"health":      ctr.Health,
			},
		}

		containerComponents = append(containerComponents, containerComponent)
	}

	return containerComponents, nil
}

// collectSwarmServices collects swarm services from the docker util and produces topology.Component
func (dt *DockerTopologyCollector) collectSwarmServices(du *docker.DockerUtil) ([]*topology.Component, error) {
	sList, err := du.ListSwarmServices()
	if err != nil {
		return nil, err
	}

	containerComponents := make([]*topology.Component, 0)
	for _, s := range sList {
		containerComponent := &topology.Component{
			ExternalID: fmt.Sprintf("urn:%s:/%s", swarmServiceType, s.ID),
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
			containerComponent.Data["updated"] = s.UpdatedAt
		}

		// add previous spec if there is one
		if s.PreviousSpec != nil {
			containerComponent.Data["previousSpec"] = s.PreviousSpec
		}

		containerComponents = append(containerComponents, containerComponent)
	}

	return containerComponents, nil
}

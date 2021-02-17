package topology

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/docker"
	"github.com/docker/docker/api/types"
)

const (
	dockerTopologyCheckName = "docker_topology"
	containerType           = "container"
)

// DockerTopologyCollector contains the checkID and topology instance for the docker topology check
type DockerTopologyCollector struct {
	CheckID check.ID
	TopologyInstance topology.Instance
}

// Container represents a single container on a machine.
type Container struct {
	Type     string
	ID       string
	Name     string
	Mounts          []types.MountPoint
}

// MakeDockerTopologyCollector returns a new instance of DockerTopologyCollector
func MakeDockerTopologyCollector() *DockerTopologyCollector {
	return &DockerTopologyCollector{
		CheckID: dockerTopologyCheckName,
		TopologyInstance:  topology.Instance{Type: "docker", URL: "agents"},
	}
}

// CollectTopology collects all docker topology
func (dt *DockerTopologyCollector) CollectTopology(du *docker.DockerUtil) error {
	// set up the topology instance to match the docker synchronization for all agents

	err := dt.collectContainers(du)
	if err != nil {
		return err
	}

	return nil
}

//
func (dt *DockerTopologyCollector) collectContainers(du *docker.DockerUtil) error {
	cList, err := du.ListContainers(&docker.ContainerListConfig{IncludeExited: true, FlagExcluded: true})
	if err != nil {
		return err
	}

	// get the batcher to produce topology data to StackState
	topologySender := batcher.GetBatcher()

	for _, ctr := range cList {
		//
		topologySender.SubmitComponent(dt.CheckID, dt.TopologyInstance,
			topology.Component{
				ExternalID: fmt.Sprintf("urn:%s:/%s", containerType, ctr.ID),
				Type:       topology.Type{Name: containerType},
				Data: topology.Data{
					"containerID": ctr.ID,
					"name":        ctr.Name,
					"type":        ctr.Type,
					"mounts":      ctr.Mounts,
				},
			},
		)
	}

	return nil
}

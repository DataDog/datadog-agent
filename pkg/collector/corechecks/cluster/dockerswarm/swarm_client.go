package dockerswarm

import (
	"github.com/StackVista/stackstate-agent/pkg/util/containers"
	"github.com/docker/docker/api/types/swarm"
	"time"
)

// SwarmClient represents a docker client that can retrieve docker swarm information from the docker API
type SwarmClient interface {
	ListSwarmServices() ([]*containers.SwarmService, error)
}


// MockSwarmClient - used in testing
type MockSwarmClient struct {

}

// ListSwarmServices returns a mock list of services
func (m *MockSwarmClient) ListSwarmServices() ([]*containers.SwarmService, error) {
	swarmServices := []*containers.SwarmService {
		{
			ID:             "Mock Service",
			Name:           "",
			ContainerImage: "",
			Labels:         nil,
			Version:        swarm.Version{},
			CreatedAt:      time.Time{},
			UpdatedAt:      time.Time{},
			Spec:           swarm.ServiceSpec{},
			PreviousSpec:   nil,
			Endpoint:       swarm.Endpoint{},
			UpdateStatus:   swarm.UpdateStatus{},
			TaskContainers: nil,
			DesiredTasks:   0,
			RunningTasks:   0,
		},
	}
	return swarmServices, nil
}

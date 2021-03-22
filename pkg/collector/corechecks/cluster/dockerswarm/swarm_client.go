package dockerswarm

import (
	"github.com/StackVista/stackstate-agent/pkg/util/containers"
)

// SwarmClient represents a docker client that can retrieve docker swarm information from the docker API
type SwarmClient interface {
	ListSwarmServices() ([]*containers.SwarmService, error)
}

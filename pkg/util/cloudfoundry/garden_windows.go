package cloudfoundry

import (
	"fmt"

	"code.cloudfoundry.org/garden"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// GetGardenContainers returns the list of running containers from the local garden API
func (gu *GardenUtil) GetGardenContainers() ([]garden.Container, error) {
	return []garden.Container{}, fmt.Errorf("garden containers collection not yet available on windows")
}

// ListContainers returns the list of running containers and populates their metrics and metadata
func (gu *GardenUtil) ListContainers() ([]*containers.Container, error) {
	return []*containers.Container{}, fmt.Errorf("garden containers collection not yet available on windows")
}

// UpdateContainerMetrics updates the metric for a list of containers
func (gu *GardenUtil) UpdateContainerMetrics(cList []*containers.Container) error {
	return fmt.Errorf("garden containers collection not yet available on windows")
}

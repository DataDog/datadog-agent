package environments

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/test-infra-definitions/resources/local/docker"
)

type DockerLocal struct {
	Local *docker.Environment
	// Components
	RemoteHost *components.RemoteHost
}

var _ e2e.Initializable = &DockerLocal{}

// Init initializes the environment
func (e *DockerLocal) Init(_ e2e.Context) error {
	return nil
}

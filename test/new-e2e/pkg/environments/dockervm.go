package environments

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
)

type DockerVM struct {
	// Components
	Host       *components.Host
	FakeIntake *components.FakeIntake
	Agent      *components.DockerAgent

	// Other clients
	Docker *client.Docker
}

var _ e2e.Initializable = &DockerVM{}

func (e *DockerVM) Init(ctx e2e.Context) error {
	var err error
	e.Docker, err = client.NewDocker(ctx.T(), e.Host.HostOutput)
	if err != nil {
		return err
	}

	if e.Agent != nil {
		agent, err := client.NewDockerAgentClient(ctx.T(), e.Docker, e.Agent.ContainerName, true)
		if err != nil {
			return err
		}
		e.Agent.Client = agent
	}

	return nil
}

package environments

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
)

type VM struct {
	// Components
	Host       *components.Host
	FakeIntake *components.FakeIntake
	Agent      *components.HostAgent
}

var _ e2e.Initializable = &VM{}

func (e *VM) Init(ctx e2e.Context) error {
	if e.Agent != nil {
		agent, err := client.NewHostAgentClient(ctx.T(), e.Host, true)
		if err != nil {
			return err
		}
		e.Agent.Client = agent
	}

	return nil
}

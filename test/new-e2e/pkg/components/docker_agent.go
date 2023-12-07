package components

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
)

type DockerAgent struct {
	agent.DockerAgentOutput

	// Client cannot be initialized inline as it requires other information to create client
	Client agentclient.Agent
}

package client

import (
	"github.com/DataDog/test-infra-definitions/datadog/agent"
)

var _ stackInitializer = (*Agent)(nil)

// A client Agent that is connected to an agent.Installer defined in test-infra-definition.
type Agent struct {
	*UpResultDeserializer[agent.InstallerData]
	*sshClient
}

// Create a new instance of Agent
func NewAgent(installer *agent.Installer) *Agent {
	agent := &Agent{}
	agent.UpResultDeserializer = NewUpResultDeserializer(installer.GetClientDataDeserializer(), agent.init)
	return agent
}

func (agent *Agent) init(auth *Authentification, data *agent.InstallerData) error {
	var err error
	agent.sshClient, err = newSSHClient(auth, &data.Connection)
	return err
}

func (agent *Agent) Status() (string, error) {
	return agent.sshClient.Execute("sudo datadog-agent status")
}

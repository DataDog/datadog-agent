// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"strings"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/os"
)

var _ clientService[agent.ClientData] = (*Agent)(nil)

// An Agent that is connected to an [agent.Installer].
//
// [agent.Installer]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/datadog/agent#Installer
type Agent struct {
	*UpResultDeserializer[agent.ClientData]
	os os.OS
	*AgentCommandRunner
	vmClient *vmClient
}

// Create a new instance of an Agent connected to an [agent.Installer].
//
// [agent.Installer]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/datadog/agent#Installer
func NewAgent(installer *agent.Installer) *Agent {
	agentInstance := &Agent{os: installer.VM().GetOS()}
	agentInstance.UpResultDeserializer = NewUpResultDeserializer[agent.ClientData](installer, agentInstance)
	return agentInstance
}

//lint:ignore U1000 Ignore unused function as this function is called using reflection
func (agent *Agent) initService(t *testing.T, data *agent.ClientData) error {
	vmClient, err := newVMClient(t, "", &data.Connection)

	agent.vmClient = vmClient
	agent.AgentCommandRunner = newAgentCommandRunner(t, agent.executeAgentCmdWithError)
	return err
}

func (agent *Agent) executeAgentCmdWithError(arguments []string) (string, error) {
	parameters := ""
	if len(arguments) > 0 {
		parameters = `"` + strings.Join(arguments, `" "`) + `"`
	}
	cmd := agent.os.GetRunAgentCmd(parameters)
	return agent.vmClient.ExecuteWithError(cmd)
}

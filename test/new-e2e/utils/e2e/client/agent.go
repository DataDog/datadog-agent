// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"github.com/DataDog/test-infra-definitions/datadog/agent"
)

var _ clientService[agent.ClientData] = (*Agent)(nil)

// A client Agent that is connected to an agent.Installer defined in test-infra-definition.
type Agent struct {
	*UpResultDeserializer[agent.ClientData]
	*sshClient
}

// Create a new instance of Agent
func NewAgent(installer *agent.Installer) *Agent {
	agentInstance := &Agent{}
	agentInstance.UpResultDeserializer = NewUpResultDeserializer[agent.ClientData](installer, agentInstance)
	return agentInstance
}

//lint:ignore U1000 Ignore unused function as this function is call using reflection
func (agent *Agent) initService(data *agent.ClientData) error {
	var err error
	agent.sshClient, err = newSSHClient("", &data.Connection)
	return err
}

func (agent *Agent) Status() (string, error) {
	return agent.sshClient.Execute("sudo datadog-agent status")
}

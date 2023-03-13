// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"github.com/DataDog/test-infra-definitions/datadog/agent"
)

var _ stackInitializer = (*Agent)(nil)

// A client Agent that is connected to an agent.Installer defined in test-infra-definition.
type Agent struct {
	*UpResultDeserializer[agent.ClientData]
	*sshClient
}

// Create a new instance of Agent
func NewAgent(installer *agent.Installer) *Agent {
	agent := &Agent{}
	agent.UpResultDeserializer = NewUpResultDeserializer(installer.GetClientDataDeserializer(), agent.init)
	return agent
}

func (agent *Agent) init(auth *Authentification, data *agent.ClientData) error {
	var err error
	agent.sshClient, err = newSSHClient(auth, &data.Connection)
	return err
}

func (agent *Agent) Status() (string, error) {
	return agent.sshClient.Execute("sudo datadog-agent status")
}

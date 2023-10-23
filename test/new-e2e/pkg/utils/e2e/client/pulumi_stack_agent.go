// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	e2eOs "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

var _ pulumiStackInitializer = (*PulumiStackAgent)(nil)
var _ Agent = (*PulumiStackAgent)(nil)

// PulumiStackAgent is a type that implements [Agent] and uses the pulumi stack filled by
// [agent.Installer] to setup the connection with the Agent.
//
// [agent.Installer]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/datadog/agent#Installer
type PulumiStackAgent struct {
	deserializer utils.RemoteServiceDeserializer[agent.ClientData]
	os           e2eOs.OS
	Agent
	shouldWaitForReady bool
}

// NewPulumiStackAgent creates a new instance of an Agent connected to an [agent.Installer].
//
// [agent.Installer]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/datadog/agent#Installer
func NewPulumiStackAgent(installer *agent.Installer, agentClientOptions ...agentclientparams.Option) *PulumiStackAgent {
	agentClientParams := agentclientparams.NewParams(agentClientOptions...)
	agentInstance := &PulumiStackAgent{
		os:                 installer.VM().GetOS(),
		shouldWaitForReady: agentClientParams.ShouldWaitForReady,
		deserializer:       installer,
	}
	return agentInstance
}

// initFromPulumiStack initializes the instance from the data stored in the pulumi stack.
// This method is called by [CallStackInitializers] using reflection.
//
//lint:ignore U1000 Ignore unused function as this function is called using reflection
func (agent *PulumiStackAgent) initFromPulumiStack(t *testing.T, stackResult auto.UpResult) error {
	clientData, err := agent.deserializer.Deserialize(stackResult)
	if err != nil {
		return err
	}

	vm, err := NewVMClient(t, &clientData.Connection, agent.os.GetType())
	if err != nil {
		return err
	}
	agent.Agent, err = NewAgentClient(t, vm, agent.os, agent.shouldWaitForReady)
	return err
}

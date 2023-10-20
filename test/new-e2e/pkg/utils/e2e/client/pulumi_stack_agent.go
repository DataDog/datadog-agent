// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	e2eOs "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

var _ stackInitializer = (*PulumiStackAgent)(nil)

// PulumiStackAgent is an agent connected to [agent.Installer] which is created from a pulumi stack.
//
// [agent.Installer]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/datadog/agent#Installer
type PulumiStackAgent struct {
	deserializer utils.RemoteServiceDeserializer[agent.ClientData]
	os           e2eOs.OS
	*AgentCommandRunner
	vmClient           *VMClient
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

//lint:ignore U1000 Ignore unused function as this function is called using reflection
func (agent *PulumiStackAgent) setStack(t *testing.T, stackResult auto.UpResult) error {
	clientData, err := agent.deserializer.Deserialize(stackResult)
	if err != nil {
		return err
	}

	agent.vmClient, err = NewVMClient(t, &clientData.Connection, agent.os.GetType())
	if err != nil {
		return err
	}
	agent.AgentCommandRunner = newAgentCommandRunner(t, agent.executeAgentCmdWithError)
	if !agent.shouldWaitForReady {
		return nil
	}
	return agent.waitForReadyTimeout(1 * time.Minute)
}

func (agent *PulumiStackAgent) executeAgentCmdWithError(arguments []string) (string, error) {
	parameters := ""
	if len(arguments) > 0 {
		parameters = `"` + strings.Join(arguments, `" "`) + `"`
	}
	cmd := agent.os.GetRunAgentCmd(parameters)
	return agent.vmClient.ExecuteWithError(cmd)
}

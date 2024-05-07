// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package environments

import (
	"github.com/DataDog/test-infra-definitions/resources/aws"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
)

// WindowsHost is an environment based on environments.Host but that is specific to Windows.
type WindowsHost struct {
	AwsEnvironment *aws.Environment
	// Components
	RemoteHost      *components.RemoteHost
	FakeIntake      *components.FakeIntake
	Agent           *components.RemoteHostAgent
	ActiveDirectory *components.RemoteActiveDirectory

	// WARN: do not use outside of the Init method
	// Agent Client options are stored here as a workaround to make it easier to customize the agent client,
	// but they should not be used for anything else as it should eventually be refactored differently
	AgentClientOptions []agentclientparams.Option
}

var _ e2e.Initializable = &WindowsHost{}

// Init initializes the environment
func (e *WindowsHost) Init(ctx e2e.Context) error {
	if e.Agent != nil {
		agent, err := client.NewHostAgentClientWithParams(ctx, e.RemoteHost.HostOutput, e.AgentClientOptions...)
		if err != nil {
			return err
		}
		e.Agent.Client = agent
	}

	return nil
}

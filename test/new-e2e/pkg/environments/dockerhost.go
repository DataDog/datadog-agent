// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package environments

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
)

// DockerHost is an environment that contains a Docker VM, FakeIntake and Agent configured to talk to each other.
type DockerHost struct {
	// Components
	Host       *components.RemoteHost
	FakeIntake *components.FakeIntake
	Agent      *components.DockerAgent

	// Other clients
	Docker *client.Docker
}

var _ e2e.Initializable = &DockerHost{}

// Init initializes the environment
func (e *DockerHost) Init(ctx e2e.Context) error {
	privateKeyPath, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.PrivateKeyPath, "")
	if err != nil {
		return err
	}

	e.Docker, err = client.NewDocker(ctx.T(), e.Host.HostOutput, privateKeyPath)
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

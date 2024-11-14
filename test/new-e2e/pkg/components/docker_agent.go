// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
)

// DockerAgent represents an Agent running in a Docker container
type DockerAgent struct {
	agent.DockerAgentOutput

	// Client cannot be initialized inline as it requires other information to create client
	Client        agentclient.Agent
	ClientOptions []agentclientparams.Option
}

var _ e2e.Initializable = (*DockerAgent)(nil)

// Init is called by e2e test Suite after the component is provisioned.
func (a *DockerAgent) Init(ctx e2e.Context) (err error) {
	a.Client, err = client.NewDockerAgentClient(ctx, a.DockerAgentOutput, a.ClientOptions...)
	return err
}

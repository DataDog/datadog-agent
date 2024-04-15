// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
)

const (
	agentReadyTimeout = 1 * time.Minute
)

// NewHostAgentClient creates an Agent client for host install
func NewHostAgentClient(t *testing.T, host *components.RemoteHost, waitForAgentReady bool) (agentclient.Agent, error) {
	if !waitForAgentReady {
		return NewHostAgentClientWithParams(t, host, agentclientparams.WithSkipWaitForAgentReady())
	}

	return NewHostAgentClientWithParams(t, host)
}

// NewHostAgentClientWithParams creates an Agent client for host install with custom parameters
func NewHostAgentClientWithParams(t *testing.T, host *components.RemoteHost, options ...agentclientparams.Option) (agentclient.Agent, error) {
	params := agentclientparams.NewParams(options...)
	ae := newAgentHostExecutor(host, params)

	return newAgentClient(t, params, ae)
}

// NewDockerAgentClient creates an Agent client for a Docker install
func NewDockerAgentClient(t *testing.T, docker *Docker, agentContainerName string, waitForAgentReady bool) (agentclient.Agent, error) {
	options := []agentclientparams.Option{}
	if !waitForAgentReady {
		options = append(options, agentclientparams.WithSkipWaitForAgentReady())
	}

	params := agentclientparams.NewParams(options...)
	ae := newAgentDockerExecutor(docker, agentContainerName)

	return newAgentClient(t, params, ae)
}

func newAgentClient(t *testing.T, params *agentclientparams.Params, executor agentCommandExecutor) (agentclient.Agent, error) {
	commandRunner := newAgentCommandRunner(t, executor)

	if params.ShouldWaitForReady {
		if err := commandRunner.waitForReadyTimeout(agentReadyTimeout); err != nil {
			return nil, err
		}
	}

	return commandRunner, nil
}

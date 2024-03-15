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
)

const (
	agentReadyTimeout = 1 * time.Minute
)

// NewHostAgentClient creates an Agent client for host install
func NewHostAgentClient(t *testing.T, host *components.RemoteHost, waitForAgentReady bool) (agentclient.Agent, error) {
	commandRunner := newAgentCommandRunner(t, newAgentHostExecutor(host))

	if waitForAgentReady {
		if err := commandRunner.waitForReadyTimeout(agentReadyTimeout); err != nil {
			return nil, err
		}
	}

	return commandRunner, nil
}

// NewDockerAgentClient creates an Agent client for a Docker install
func NewDockerAgentClient(t *testing.T, docker *Docker, agentContainerName string, waitForAgentReady bool) (agentclient.Agent, error) {
	commandRunner := newAgentCommandRunner(t, newAgentDockerExecutor(docker, agentContainerName))

	if waitForAgentReady {
		if err := commandRunner.waitForReadyTimeout(agentReadyTimeout); err != nil {
			return nil, err
		}
	}

	return commandRunner, nil
}

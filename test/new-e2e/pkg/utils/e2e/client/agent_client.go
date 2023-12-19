// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"strings"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/os"
)

// AgentClient is a type that provides methods to run remote commands on a test-infra-definition Agent.
type AgentClient struct {
	*agentCommandRunner
}

// NewAgentClient creates a new instance of AgentClient
func NewAgentClient(t *testing.T, vm VM, os os.OS, shouldWaitForReady bool) (*AgentClient, error) {
	agent := &AgentClient{
		agentCommandRunner: newAgentCommandRunner(t, func(arguments []string) (string, error) {
			parameters := ""
			if len(arguments) > 0 {
				parameters = `"` + strings.Join(arguments, `" "`) + `"`
			}
			cmd := os.GetRunAgentCmd(parameters)
			return vm.ExecuteWithError(cmd)
		}),
	}

	if shouldWaitForReady {
		if err := agent.waitForReadyTimeout(1 * time.Minute); err != nil {
			return nil, err
		}
	}
	return agent, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/stretchr/testify/require"
)

// executeAgentCmdWithError is a function to run a command on the Agent.
type executeAgentCmdWithError func(arguments []string) (string, error)

// AgentCommandRunner provides high level methods to run commands on the Agent.
type AgentCommandRunner struct {
	t                        *testing.T
	executeAgentCmdWithError executeAgentCmdWithError
	isReady                  bool
}

// Create a new instance of AgentCommandRunner
func newAgentCommandRunner(t *testing.T, executeAgentCmdWithError executeAgentCmdWithError) *AgentCommandRunner {
	agent := &AgentCommandRunner{
		t:                        t,
		executeAgentCmdWithError: executeAgentCmdWithError,
		isReady:                  false,
	}
	return agent
}

func (agent *AgentCommandRunner) executeCommand(command string, commandArgs ...AgentArgsOption) string {
	if !agent.isReady {
		err := agent.waitForReadyTimeout(1 * time.Minute)
		require.NoErrorf(agent.t, err, "the agent is not ready")
		agent.isReady = true
	}
	args := newAgentArgs(commandArgs...)
	arguments := []string{command}
	arguments = append(arguments, args.Args...)
	output, err := agent.executeAgentCmdWithError(arguments)
	require.NoError(agent.t, err)
	return output
}

func (agent *AgentCommandRunner) Version(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("version", commandArgs...)
}

func (agent *AgentCommandRunner) Hostname(commandArgs ...AgentArgsOption) string {
	output := agent.executeCommand("hostname", commandArgs...)
	return strings.Trim(output, "\n")
}

func (agent *AgentCommandRunner) Config(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("config", commandArgs...)
}

func (agent *AgentCommandRunner) Health() (string, error) {
	arguments := []string{"health"}
	output, err := agent.executeAgentCmdWithError(arguments)
	return output, err
}

func (agent *AgentCommandRunner) ConfigCheck(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("configcheck", commandArgs...)
}

// IsReady runs status command and returns true if the agent is ready.
// This function should rarely be used.
func (a *Agent) IsReady() (bool, error) {
	return a.Status().isReady()
}

type Status struct {
	Content string
}

func newStatus(s string) *Status {
	return &Status{Content: s}
}

// isReady true if status contains a valid version
func (s *Status) isReady() (bool, error) {
	return regexp.MatchString("={15}\nAgent \\(v6|7\\.\\d{2}\\..*\n={15}", s.Content)
}

func (agent *AgentCommandRunner) Status(commandArgs ...AgentArgsOption) *Status {
	return newStatus(agent.executeCommand("status", commandArgs...))
}

// WaitForReady blocks up to timeout waiting for agent to be ready.
// Retries every 100 ms up to timeout.
// Returns error on failure.
func (a *AgentCommandRunner) waitForReadyTimeout(timeout time.Duration) error {
	interval := 100 * time.Millisecond
	maxRetries := timeout.Milliseconds() / interval.Milliseconds()
	err := backoff.Retry(func() error {
		statusOutput, err := a.executeAgentCmdWithError([]string{"status"})
		if err != nil {
			return err
		}

		isReady, err := newStatus(statusOutput).isReady()
		if err != nil {
			return err
		}
		if !isReady {
			return errors.New("agent not ready")
		}

		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(interval), uint64(maxRetries)))
	return err
}

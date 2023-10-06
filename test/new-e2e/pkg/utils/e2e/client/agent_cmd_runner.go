// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"errors"
	"fmt"
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
	args := newAgentArgs(commandArgs...)
	arguments := []string{command}
	arguments = append(arguments, args.Args...)
	output, err := agent.executeAgentCmdWithError(arguments)
	require.NoError(agent.t, err)
	return output
}

// Version runs version command returns the runtime Agent version
func (agent *AgentCommandRunner) Version(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("version", commandArgs...)
}

// Hostname runs hostname command and returns the runtime Agent hostname
func (agent *AgentCommandRunner) Hostname(commandArgs ...AgentArgsOption) string {
	output := agent.executeCommand("hostname", commandArgs...)
	return strings.Trim(output, "\n")
}

// Config runs config command and returns the runtime agent config
func (agent *AgentCommandRunner) Config(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("config", commandArgs...)
}

// Flare runs flare command and returns the output. You should use the FakeIntake client to fetch the flare archive
func (agent *AgentCommandRunner) Flare(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("flare", commandArgs...)
}

// Health runs health command and returns the runtime agent health
func (agent *AgentCommandRunner) Health() (string, error) {
	arguments := []string{"health"}
	output, err := agent.executeAgentCmdWithError(arguments)
	return output, err
}

// ConfigCheck runs configcheck command and returns the runtime agent configcheck
func (agent *AgentCommandRunner) ConfigCheck(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("configcheck", commandArgs...)
}

// Secret runs the secret command
func (agent *AgentCommandRunner) Secret(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("secret", commandArgs...)
}

// IsReady runs status command and returns true if the command returns a zero exit code.
// This function should rarely be used.
func (a *Agent) IsReady() bool {
	_, err := a.executeAgentCmdWithError([]string{"status"})
	return err == nil
}

// Status contains the Agent status content
type Status struct {
	Content string
}

func newStatus(s string) *Status {
	return &Status{Content: s}
}

// Status runs status command and returns a Status struct
func (agent *AgentCommandRunner) Status(commandArgs ...AgentArgsOption) *Status {

	return newStatus(agent.executeCommand("status", commandArgs...))
}

// waitForReadyTimeout blocks up to timeout waiting for agent to be ready.
// Retries every 100 ms up to timeout.
// Returns error on failure.
func (agent *AgentCommandRunner) waitForReadyTimeout(timeout time.Duration) error {
	interval := 100 * time.Millisecond
	maxRetries := timeout.Milliseconds() / interval.Milliseconds()
	err := backoff.Retry(func() error {
		_, err := agent.executeAgentCmdWithError([]string{"status"})
		if err != nil {
			return errors.New("agent not ready")
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(interval), uint64(maxRetries)))
	return err
}

// WaitAgentLogs waits for the agent log corresponding to the pattern
// agent-name can be: datadog-agent, system-probe, security-agent
// pattern: is the log that we are looking for
// Retries every 500 ms up to timeout.
// Returns error on failure.
func (a *Agent) WaitAgentLogs(agentName string, pattern string) error {
	err := backoff.Retry(func() error {
		output, err := a.vmClient.ExecuteWithError(fmt.Sprintf("cat /var/log/datadog/%s.log", agentName))
		if err != nil {
			return err
		}
		if strings.Contains(output, pattern) {
			return nil
		}
		return errors.New("no log found")
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(500*time.Millisecond), 60))
	return err
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/stretchr/testify/require"
)

// executeAgentCmdWithError is a function to run an Agent command.
type executeAgentCmdWithError func(arguments []string) (string, error)

// agentCommandRunner is an internal type that provides methods to run Agent commands.
// It is used by both [VMClient] and [Docker]
type agentCommandRunner struct {
	t                        *testing.T
	executeAgentCmdWithError executeAgentCmdWithError
	isReady                  bool
}

// Create a new instance of agentCommandRunner
func newAgentCommandRunner(t *testing.T, executeAgentCmdWithError executeAgentCmdWithError) *agentCommandRunner {
	agent := &agentCommandRunner{
		t:                        t,
		executeAgentCmdWithError: executeAgentCmdWithError,
		isReady:                  false,
	}
	return agent
}

func (agent *agentCommandRunner) executeCommand(command string, commandArgs ...AgentArgsOption) string {
	output, err := agent.executeCommandWithError(command, commandArgs...)
	require.NoError(agent.t, err)
	return output
}

func (agent *agentCommandRunner) executeCommandWithError(command string, commandArgs ...AgentArgsOption) (string, error) {
	if !agent.isReady {
		err := agent.waitForReadyTimeout(1 * time.Minute)
		require.NoErrorf(agent.t, err, "the agent is not ready")
		agent.isReady = true
	}
	args := newAgentArgs(commandArgs...)
	arguments := []string{command}
	arguments = append(arguments, args.Args...)
	output, err := agent.executeAgentCmdWithError(arguments)
	return output, err
}

// Version runs version command returns the runtime Agent version
func (agent *agentCommandRunner) Version(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("version", commandArgs...)
}

// Hostname runs hostname command and returns the runtime Agent hostname
func (agent *agentCommandRunner) Hostname(commandArgs ...AgentArgsOption) string {
	output := agent.executeCommand("hostname", commandArgs...)
	return strings.Trim(output, "\n")
}

// Config runs config command and returns the runtime agent config
func (agent *agentCommandRunner) Config(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("config", commandArgs...)
}

// ConfigWithError runs config command and returns the runtime agent config or an error
func (agent *agentCommandRunner) ConfigWithError(commandArgs ...AgentArgsOption) (string, error) {
	arguments := append([]string{"config"}, newAgentArgs(commandArgs...).Args...)
	return agent.executeAgentCmdWithError(arguments)
}

// Diagnose runs diagnose command and returns its output
func (agent *agentCommandRunner) Diagnose(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("diagnose", commandArgs...)
}

// Flare runs flare command and returns the output. You should use the FakeIntake client to fetch the flare archive
func (agent *agentCommandRunner) Flare(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("flare", commandArgs...)
}

// Health runs health command and returns the runtime agent health
func (agent *agentCommandRunner) Health() (string, error) {
	arguments := []string{"health"}
	output, err := agent.executeAgentCmdWithError(arguments)
	return output, err
}

// ConfigCheck runs configcheck command and returns the runtime agent configcheck
func (agent *agentCommandRunner) ConfigCheck(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("configcheck", commandArgs...)
}

// Integration run integration command and returns the output
func (agent *agentCommandRunner) Integration(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("integration", commandArgs...)
}

// IntegrationWithError run integration command and returns the output
func (agent *agentCommandRunner) IntegrationWithError(commandArgs ...AgentArgsOption) (string, error) {
	return agent.executeCommandWithError("integration", commandArgs...)
}

// Secret runs the secret command
func (agent *agentCommandRunner) Secret(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("secret", commandArgs...)
}

// IsReady runs status command and returns true if the command returns a zero exit code.
// This function should rarely be used.
func (agent *agentCommandRunner) IsReady() bool {
	_, err := agent.executeAgentCmdWithError([]string{"status"})
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
func (agent *agentCommandRunner) Status(commandArgs ...AgentArgsOption) *Status {

	return newStatus(agent.executeCommand("status", commandArgs...))
}

// StatusWithError runs status command and returns a Status struct and error
func (agent *agentCommandRunner) StatusWithError(commandArgs ...AgentArgsOption) (*Status, error) {
	status, err := agent.executeCommandWithError("status", commandArgs...)
	return newStatus(status), err
}

// waitForReadyTimeout blocks up to timeout waiting for agent to be ready.
// Retries every 100 ms up to timeout.
// Returns error on failure.
func (agent *agentCommandRunner) waitForReadyTimeout(timeout time.Duration) error {
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

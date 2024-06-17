// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"

	"github.com/cenkalti/backoff"
	"github.com/stretchr/testify/require"
)

type agentCommandExecutor interface {
	execute(arguments []string) (string, error)
}

// agentCommandRunner is an internal type that provides methods to run Agent commands.
// It is used by both [VMClient] and [Docker]
type agentCommandRunner struct {
	t        *testing.T
	executor agentCommandExecutor
	isReady  bool
}

// Create a new instance of agentCommandRunner
func newAgentCommandRunner(t *testing.T, executor agentCommandExecutor) *agentCommandRunner {
	agent := &agentCommandRunner{
		t:        t,
		executor: executor,
		isReady:  false,
	}
	return agent
}

func (agent *agentCommandRunner) executeCommand(command string, commandArgs ...agentclient.AgentArgsOption) string {
	output, err := agent.executeCommandWithError(command, commandArgs...)
	require.NoError(agent.t, err)
	return output
}

func (agent *agentCommandRunner) executeCommandWithError(command string, commandArgs ...agentclient.AgentArgsOption) (string, error) {
	if !agent.isReady {
		err := agent.waitForReadyTimeout(1 * time.Minute)
		require.NoErrorf(agent.t, err, "the agent is not ready")
		agent.isReady = true
	}

	args, err := optional.MakeParams(commandArgs...)
	require.NoError(agent.t, err)

	arguments := []string{command}
	arguments = append(arguments, args.Args...)
	agent.t.Logf("Running agent command: %+q", arguments)
	output, err := agent.executor.execute(arguments)
	return output, err
}

// Version runs version command returns the runtime Agent version
func (agent *agentCommandRunner) Version(commandArgs ...agentclient.AgentArgsOption) string {
	return agent.executeCommand("version", commandArgs...)
}

// Hostname runs hostname command and returns the runtime Agent hostname
func (agent *agentCommandRunner) Hostname(commandArgs ...agentclient.AgentArgsOption) string {
	output := agent.executeCommand("hostname", commandArgs...)
	return strings.Trim(output, "\n")
}

// Check runs check command and returns the runtime Agent check
func (agent *agentCommandRunner) Check(commandArgs ...agentclient.AgentArgsOption) string {
	return agent.executeCommand("check", commandArgs...)
}

// Check runs check command and returns the runtime Agent check or an error
func (agent *agentCommandRunner) CheckWithError(commandArgs ...agentclient.AgentArgsOption) (string, error) {
	args, err := optional.MakeParams(commandArgs...)
	require.NoError(agent.t, err)

	arguments := append([]string{"check"}, args.Args...)
	return agent.executor.execute(arguments)
}

// Config runs config command and returns the runtime agent config
func (agent *agentCommandRunner) Config(commandArgs ...agentclient.AgentArgsOption) string {
	return agent.executeCommand("config", commandArgs...)
}

// ConfigWithError runs config command and returns the runtime agent config or an error
func (agent *agentCommandRunner) ConfigWithError(commandArgs ...agentclient.AgentArgsOption) (string, error) {
	args, err := optional.MakeParams(commandArgs...)
	require.NoError(agent.t, err)

	arguments := append([]string{"config"}, args.Args...)
	return agent.executor.execute(arguments)
}

// Diagnose runs diagnose command and returns its output
func (agent *agentCommandRunner) Diagnose(commandArgs ...agentclient.AgentArgsOption) string {
	return agent.executeCommand("diagnose", commandArgs...)
}

// Flare runs flare command and returns the output. You should use the FakeIntake client to fetch the flare archive
func (agent *agentCommandRunner) Flare(commandArgs ...agentclient.AgentArgsOption) string {
	return agent.executeCommand("flare", commandArgs...)
}

// FlareWithError runs flare command and returns the output or an error. You should use the FakeIntake client to fetch the flare archive
func (agent *agentCommandRunner) FlareWithError(commandArgs ...agentclient.AgentArgsOption) (string, error) {
	args, err := optional.MakeParams(commandArgs...)
	require.NoError(agent.t, err)

	arguments := append([]string{"flare"}, args.Args...)
	return agent.executor.execute(arguments)
}

// Health runs health command and returns the runtime agent health
func (agent *agentCommandRunner) Health() (string, error) {
	arguments := []string{"health"}
	output, err := agent.executor.execute(arguments)
	return output, err
}

// ConfigCheck runs configcheck command and returns the runtime agent configcheck
func (agent *agentCommandRunner) ConfigCheck(commandArgs ...agentclient.AgentArgsOption) string {
	return agent.executeCommand("configcheck", commandArgs...)
}

// Integration run integration command and returns the output
func (agent *agentCommandRunner) Integration(commandArgs ...agentclient.AgentArgsOption) string {
	return agent.executeCommand("integration", commandArgs...)
}

// IntegrationWithError run integration command and returns the output
func (agent *agentCommandRunner) IntegrationWithError(commandArgs ...agentclient.AgentArgsOption) (string, error) {
	return agent.executeCommandWithError("integration", commandArgs...)
}

// Secret runs the secret command
func (agent *agentCommandRunner) Secret(commandArgs ...agentclient.AgentArgsOption) string {
	return agent.executeCommand("secret", commandArgs...)
}

// IsReady runs status command and returns true if the command returns a zero exit code.
// This function should rarely be used.
func (agent *agentCommandRunner) IsReady() bool {
	_, err := agent.executor.execute([]string{"status"})
	return err == nil
}

// RemoteConfig runs remote-config command and returns the output
func (agent *agentCommandRunner) RemoteConfig(commandArgs ...agentclient.AgentArgsOption) string {
	return agent.executeCommand("remote-config", commandArgs...)
}

// Status runs status command and returns a Status struct
func (agent *agentCommandRunner) Status(commandArgs ...agentclient.AgentArgsOption) *agentclient.Status {
	return &agentclient.Status{
		Content: agent.executeCommand("status", commandArgs...),
	}
}

// StatusWithError runs status command and returns a Status struct and error
func (agent *agentCommandRunner) StatusWithError(commandArgs ...agentclient.AgentArgsOption) (*agentclient.Status, error) {
	status, err := agent.executeCommandWithError("status", commandArgs...)

	return &agentclient.Status{
		Content: status,
	}, err
}

// waitForReadyTimeout blocks up to timeout waiting for agent to be ready.
// Retries every 100 ms up to timeout.
// Returns error on failure.
func (agent *agentCommandRunner) waitForReadyTimeout(timeout time.Duration) error {
	interval := 100 * time.Millisecond
	maxRetries := timeout.Milliseconds() / interval.Milliseconds()
	agent.t.Log("Waiting for the agent to be ready")
	err := backoff.Retry(func() error {
		_, err := agent.executor.execute([]string{"status"})
		if err != nil {
			return fmt.Errorf("agent not ready: %w", err)
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(interval), uint64(maxRetries)))
	return err
}

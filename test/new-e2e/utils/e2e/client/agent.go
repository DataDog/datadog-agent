// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/runner/parameters"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	e2eOs "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/cenkalti/backoff"
)

var _ clientService[agent.ClientData] = (*Agent)(nil)

// A client Agent that is connected to an [agent.Installer].
//
// [agent.Installer]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/datadog/agent#Installer
type Agent struct {
	*UpResultDeserializer[agent.ClientData]
	*vmClient
	os e2eOs.OS
}

// Create a new instance of Agent
func NewAgent(installer *agent.Installer) *Agent {
	agentInstance := &Agent{os: installer.VM().GetOS()}
	agentInstance.UpResultDeserializer = NewUpResultDeserializer[agent.ClientData](installer, agentInstance)
	return agentInstance
}

//lint:ignore U1000 Ignore unused function as this function is call using reflection
func (agent *Agent) initService(t *testing.T, data *agent.ClientData) error {
	var err error
	var privateSshKey []byte

	privateKeyPath, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.PrivateKeyPath, "")
	if err != nil {
		return err
	}

	if privateKeyPath != "" {
		privateSshKey, err = os.ReadFile(privateKeyPath)
		if err != nil {
			return err
		}
	}

	agent.vmClient, err = newVMClient(t, privateSshKey, &data.Connection)
	return err
}

func (agent *Agent) GetCommand(parameters string) string {
	return agent.os.GetRunAgentCmd(parameters)
}

func (agent *Agent) executeCommand(command string, commandArgs ...AgentArgsOption) string {
	args := newAgentArgs(commandArgs...)
	return agent.vmClient.Execute(agent.GetCommand(command) + " " + args.Args)
}

func (agent *Agent) Version(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("version", commandArgs...)
}

func (agent *Agent) Config(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("config", commandArgs...)
}

func (agent *Agent) Hostname(commandArgs ...AgentArgsOption) string {
	output := agent.executeCommand("hostname", commandArgs...)
	return strings.Trim(output, "\n")
}

type Status struct {
	Content string
}

func newStatus(s string) *Status {
	return &Status{Content: s}
}

// isReady true if status contains a valid version
func (s *Status) isReady() (bool, error) {
	return regexp.MatchString("={15}\nAgent \\(v7\\.\\d{2}\\..*\n={15}", s.Content)
}

func (agent *Agent) Status(commandArgs ...AgentArgsOption) *Status {
	return newStatus(agent.executeCommand("status", commandArgs...))
}

// IsReady runs status command and returns true if the agent is ready.
// Use this to wait for agent to be ready before running any command.
func (a *Agent) IsReady() (bool, error) {
	return a.Status().isReady()
}

// WaitForReady blocks up to one minute waiting for agent to be ready.
// Retries every 100 ms up to one minute.
// Returns error on failure.
func (a *Agent) WaitForReady() error {
	return a.WaitForReadyTimeout(1 * time.Minute)
}

// WaitForReady blocks up to timeout waiting for agent to be ready.
// Retries every 100 ms up to timeout.
// Returns error on failure.
func (a *Agent) WaitForReadyTimeout(timeout time.Duration) error {
	interval := 100 * time.Millisecond
	maxRetries := timeout.Milliseconds() / interval.Milliseconds()
	err := backoff.Retry(func() error {
		statusOutput, err := a.vmClient.ExecuteWithError(a.GetCommand("status"))
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

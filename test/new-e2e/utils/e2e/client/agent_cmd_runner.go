// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"errors"
	"regexp"
	"time"

	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/cenkalti/backoff"
)

// AgentCommandRunner provides methods to run commands on the Agent.
type AgentCommandRunner struct {
	cmdRunner *vmClient
	os        os.OS
}

// Create a new instance of Agent
func newAgentCommandRunner(client *vmClient, os os.OS) *AgentCommandRunner {
	agent := &AgentCommandRunner{
		cmdRunner: client, os: os,
	}
	agent.waitForReadyTimeout(1 * time.Minute)
	return agent
}

func (agent *AgentCommandRunner) getCommand(parameters string) string {
	return agent.os.GetRunAgentCmd(parameters)
}

func (agent *AgentCommandRunner) executeCommand(command string, commandArgs ...AgentArgsOption) string {
	args := newAgentArgs(commandArgs...)
	return agent.cmdRunner.Execute(agent.getCommand(command) + " " + args.Args)
}

func (agent *AgentCommandRunner) Version(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("version", commandArgs...)
}

func (agent *AgentCommandRunner) Config(commandArgs ...AgentArgsOption) string {
	return agent.executeCommand("config", commandArgs...)
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
		statusOutput, err := a.cmdRunner.ExecuteWithError(a.getCommand("status"))
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

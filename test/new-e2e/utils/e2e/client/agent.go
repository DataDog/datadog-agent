// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"errors"
	"regexp"
	"time"

	"github.com/DataDog/test-infra-definitions/datadog/agent"
	"github.com/cenkalti/backoff"
)

var _ stackInitializer = (*Agent)(nil)

// A client Agent that is connected to an agent.Installer defined in test-infra-definition.
type Agent struct {
	*UpResultDeserializer[agent.ClientData]
	*sshClient
}

// Create a new instance of Agent
func NewAgent(installer *agent.Installer) *Agent {
	agent := &Agent{}
	agent.UpResultDeserializer = NewUpResultDeserializer(installer.GetClientDataDeserializer(), agent.init)
	return agent
}

func (agent *Agent) init(auth *Authentification, data *agent.ClientData) error {
	var err error
	agent.sshClient, err = newSSHClient(auth, &data.Connection)
	return err
}

func (agent *Agent) Version() (string, error) {
	return agent.sshClient.Execute("datadog-agent version")
}

type Status struct {
	rawString string
}

func NewStatus(s string) *Status {
	return &Status{rawString: s}
}

func (agent *Agent) Status() (*Status, error) {
	s, err := agent.sshClient.Execute("sudo datadog-agent status")
	if err != nil {
		return nil, err
	}
	return NewStatus(s), nil
}

// IsReady true if status contains a valid version
func (s *Status) IsReady() (bool, error) {
	return regexp.MatchString("={15}\nAgent \\(v7\\.\\d{2}\\..*\n={15}", s.rawString)
}

// IsReady runs status command and returns true if the status is ready
// Use this to wait for agent to be ready before running any command
func (a *Agent) IsReady() (bool, error) {
	status, err := a.Status()
	if err != nil {
		return false, err
	}
	return status.IsReady()
}

// WaitForReady blocks up for timeout waiting for agent to be ready
// Retries every 100 ms up to timeout
// Returns error on failure
func (a *Agent) WaitForReady(timeout time.Duration) error {
	interval := 100 * time.Millisecond
	maxRetries := timeout.Milliseconds() / interval.Milliseconds()
	err := backoff.Retry(func() error {
		isReady, err := a.IsReady()
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

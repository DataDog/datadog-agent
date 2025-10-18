// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent contains a wrapper around the agent commands for use in tests.
package agent

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// Agent is a cross platform wrapper around the agent commands for use in tests.
type Agent struct {
	t      func() *testing.T
	remote *components.RemoteHost
}

// New creates a new instance of Agent.
func New(t func() *testing.T, remote *components.RemoteHost) *Agent {
	return &Agent{t: t, remote: remote}
}

// Configuration returns the configuration of the agent.
func (a *Agent) Configuration() (map[string]any, error) {
	rawConfig, err := a.runCommand("config", "--all")
	if err != nil {
		return nil, err
	}
	config := make(map[string]any)
	err = yaml.Unmarshal([]byte(rawConfig), &config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

// runCommand runs a command on the remote host.
func (a *Agent) runCommand(command string, args ...string) (string, error) {
	var err error
	for range 30 {
		_, err = a.remote.Execute("sudo -u dd-agent datadog-agent config --all")
		if err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		return "", err
	}
	return a.remote.Execute(fmt.Sprintf("sudo -u dd-agent datadog-agent %s %s", command, strings.Join(args, " ")))
}

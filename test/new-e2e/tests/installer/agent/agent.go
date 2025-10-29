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

	"github.com/avast/retry-go/v4"
	"github.com/goccy/go-yaml"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
)

// Agent is a cross platform wrapper around the agent commands for use in tests.
type Agent struct {
	t    func() *testing.T
	host *environments.Host
}

// New creates a new instance of Agent.
func New(t func() *testing.T, host *environments.Host) *Agent {
	return &Agent{t: t, host: host}
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
	var baseCommand string
	switch a.host.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		baseCommand = "sudo -u dd-agent datadog-agent"
	case e2eos.WindowsFamily:
		baseCommand = `& "C:\Program Files\Datadog\Datadog Agent\bin\agent.exe"`
	default:
		return "", fmt.Errorf("unsupported OS family: %v", a.host.RemoteHost.OSFamily)
	}

	err := retry.Do(func() error {
		_, err := a.host.RemoteHost.Execute(fmt.Sprintf("%s config --all", baseCommand))
		return err
	}, retry.Attempts(10), retry.Delay(1*time.Second), retry.DelayType(retry.FixedDelay))
	if err != nil {
		return "", fmt.Errorf("error waiting for agent to be ready: %w", err)
	}
	return a.host.RemoteHost.Execute(fmt.Sprintf("%s %s %s", baseCommand, command, strings.Join(args, " ")))
}

// MustSetExperimentTimeout sets the agent experiment timeout for config and upgrades.
func (a *Agent) MustSetExperimentTimeout(timeout time.Duration) {
	switch a.host.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		a.host.RemoteHost.MustExecute("sudo mkdir -p /etc/systemd/system/datadog-agent-exp.service.d")
		a.host.RemoteHost.MustExecute(fmt.Sprintf("sudo sh -c 'echo \"[Service]\nEnvironment=EXPERIMENT_TIMEOUT=%ds\" > /etc/systemd/system/datadog-agent-exp.service.d/experiment-timeout.conf'", int(timeout.Seconds())))
	case e2eos.WindowsFamily:

	default:
		a.t().Fatalf("Unsupported OS family: %v", a.host.RemoteHost.OSFamily)
	}
}

// MustUnsetExperimentTimeout unsets the agent experiment timeout for config and upgrades.
func (a *Agent) MustUnsetExperimentTimeout() {
	switch a.host.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		a.host.RemoteHost.MustExecute("sudo rm -f /etc/systemd/system/datadog-agent-exp.service.d/experiment-timeout.conf")
	case e2eos.WindowsFamily:
	default:
		a.t().Fatalf("Unsupported OS family: %v", a.host.RemoteHost.OSFamily)
	}
}

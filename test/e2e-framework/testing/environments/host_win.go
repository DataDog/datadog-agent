// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package environments

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/activedirectory"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
)

// WindowsHost is an environment based on environments.Host but that is specific to Windows.
type WindowsHost struct {
	Environment config.Env
	// Components
	RemoteHost      *components.RemoteHost
	FakeIntake      *components.FakeIntake
	Agent           *components.RemoteHostAgent
	ActiveDirectory *components.RemoteActiveDirectory
}

// Ensure WindowsHost implements the WindowsHostOutputs interface
var _ outputs.WindowsHostOutputs = (*WindowsHost)(nil)

var _ common.Initializable = &WindowsHost{}

// Init initializes the environment
func (e *WindowsHost) Init(_ common.Context) error {
	return nil
}

// RemoteHostOutput implements outputs.WindowsHostOutputs
func (e *WindowsHost) RemoteHostOutput() *remote.HostOutput {
	if e.RemoteHost == nil {
		e.RemoteHost = &components.RemoteHost{}
	}
	return &e.RemoteHost.HostOutput
}

// FakeIntakeOutput implements outputs.WindowsHostOutputs
func (e *WindowsHost) FakeIntakeOutput() *fakeintake.FakeintakeOutput {
	if e.FakeIntake == nil {
		e.FakeIntake = &components.FakeIntake{}
	}
	return &e.FakeIntake.FakeintakeOutput
}

// AgentOutput implements outputs.WindowsHostOutputs
func (e *WindowsHost) AgentOutput() *agent.HostAgentOutput {
	if e.Agent == nil {
		e.Agent = &components.RemoteHostAgent{}
	}
	return &e.Agent.HostAgentOutput
}

// ActiveDirectoryOutput implements outputs.WindowsHostOutputs
func (e *WindowsHost) ActiveDirectoryOutput() *activedirectory.Output {
	if e.ActiveDirectory == nil {
		e.ActiveDirectory = &components.RemoteActiveDirectory{}
	}
	return &e.ActiveDirectory.Output
}

// DisableFakeIntake implements outputs.WindowsHostOutputs
func (e *WindowsHost) DisableFakeIntake() {
	e.FakeIntake = nil
}

// DisableAgent implements outputs.WindowsHostOutputs
func (e *WindowsHost) DisableAgent() {
	e.Agent = nil
}

// DisableActiveDirectory implements outputs.WindowsHostOutputs
func (e *WindowsHost) DisableActiveDirectory() {
	e.ActiveDirectory = nil
}

func (e *WindowsHost) SetAgentClientOptions(options ...agentclientparams.Option) {
	e.Agent.ClientOptions = options
}

// SetEnvironment implements outputs.WindowsHostOutputs
func (e *WindowsHost) SetEnvironment(env config.Env) {
	e.Environment = env
}

// Diagnose returns a string containing the diagnosis of the environment
func (e *WindowsHost) Diagnose(outputDir string) (string, error) {
	diagnoses := []string{}
	if e.RemoteHost == nil {
		return "", errors.New("RemoteHost component is not initialized")
	}
	// add Agent diagnose
	if e.Agent != nil {
		diagnoses = append(diagnoses, "==== Agent ====")
		dstPath, err := generateAndDownloadAgentFlare(e.Agent, e.RemoteHost, outputDir)
		if err != nil {
			return "", fmt.Errorf("failed to generate and download agent flare: %w", err)
		}
		diagnoses = append(diagnoses, "Flare archive downloaded to "+dstPath)
		diagnoses = append(diagnoses, "\n")
	}

	return strings.Join(diagnoses, "\n"), nil
}

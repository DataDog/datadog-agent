// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package windowshost

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	compout "github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs/windowshost"
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
var _ windowshost.WindowsHostOutputs = (*WindowsHost)(nil)

var _ common.Initializable = &WindowsHost{}

// Init initializes the environment
func (e *WindowsHost) Init(_ common.Context) error {
	return nil
}

// RemoteHostOutput implements windowshost.WindowsHostOutputs
func (e *WindowsHost) RemoteHostOutput() *compout.HostOutput {
	if e.RemoteHost == nil {
		e.RemoteHost = &components.RemoteHost{}
	}
	return &e.RemoteHost.HostOutput
}

// FakeIntakeOutput implements windowshost.WindowsHostOutputs
func (e *WindowsHost) FakeIntakeOutput() *compout.FakeintakeOutput {
	if e.FakeIntake == nil {
		e.FakeIntake = &components.FakeIntake{}
	}
	return &e.FakeIntake.FakeintakeOutput
}

// AgentOutput implements windowshost.WindowsHostOutputs
func (e *WindowsHost) AgentOutput() *compout.HostAgentOutput {
	if e.Agent == nil {
		e.Agent = &components.RemoteHostAgent{}
	}
	return &e.Agent.HostAgentOutput
}

// ActiveDirectoryOutput implements windowshost.WindowsHostOutputs
func (e *WindowsHost) ActiveDirectoryOutput() *compout.ActiveDirectoryOutput {
	if e.ActiveDirectory == nil {
		e.ActiveDirectory = &components.RemoteActiveDirectory{}
	}
	return &e.ActiveDirectory.ActiveDirectoryOutput
}

// DisableFakeIntake implements windowshost.WindowsHostOutputs
func (e *WindowsHost) DisableFakeIntake() {
	e.FakeIntake = nil
}

// DisableAgent implements windowshost.WindowsHostOutputs
func (e *WindowsHost) DisableAgent() {
	e.Agent = nil
}

// DisableActiveDirectory implements windowshost.WindowsHostOutputs
func (e *WindowsHost) DisableActiveDirectory() {
	e.ActiveDirectory = nil
}

func (e *WindowsHost) SetAgentClientOptions(options ...agentclientparams.Option) {
	e.Agent.ClientOptions = options
}

// SetEnvironment implements windowshost.WindowsHostOutputs
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
		dstPath, err := components.GenerateAndDownloadAgentFlare(e.Agent, e.RemoteHost, outputDir)
		if err != nil {
			return "", fmt.Errorf("failed to generate and download agent flare: %w", err)
		}
		diagnoses = append(diagnoses, "Flare archive downloaded to "+dstPath)
		diagnoses = append(diagnoses, "\n")
	}

	return strings.Join(diagnoses, "\n"), nil
}

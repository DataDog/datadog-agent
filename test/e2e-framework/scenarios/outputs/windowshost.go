// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package outputs

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/activedirectory"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
)

// WindowsHostOutputs is the interface for WindowsHost environment outputs.
type WindowsHostOutputs interface {
	RemoteHostOutput() *remote.HostOutput
	FakeIntakeOutput() *fakeintake.FakeintakeOutput
	AgentOutput() *agent.HostAgentOutput
	ActiveDirectoryOutput() *activedirectory.Output
	DisableFakeIntake()
	DisableAgent()
	DisableActiveDirectory()
	SetAgentClientOptions(options ...agentclientparams.Option)
	SetEnvironment(env config.Env)
}

// WindowsHost contains the outputs for a WindowsHost environment.
type WindowsHost struct {
	RemoteHost      *remote.HostOutput
	FakeIntake      *fakeintake.FakeintakeOutput
	Agent           *agent.HostAgentOutput
	ActiveDirectory *activedirectory.Output
}

// NewWindowsHost creates a new WindowsHost output struct with all fields initialized.
func NewWindowsHost() *WindowsHost {
	return &WindowsHost{
		RemoteHost:      &remote.HostOutput{},
		FakeIntake:      &fakeintake.FakeintakeOutput{},
		Agent:           &agent.HostAgentOutput{},
		ActiveDirectory: &activedirectory.Output{},
	}
}

// RemoteHostOutput returns the remote host output for exporting
func (h *WindowsHost) RemoteHostOutput() *remote.HostOutput {
	return h.RemoteHost
}

// FakeIntakeOutput returns the fakeintake output for exporting (may be nil)
func (h *WindowsHost) FakeIntakeOutput() *fakeintake.FakeintakeOutput {
	return h.FakeIntake
}

// AgentOutput returns the agent output for exporting (may be nil)
func (h *WindowsHost) AgentOutput() *agent.HostAgentOutput {
	return h.Agent
}

// ActiveDirectoryOutput returns the ActiveDirectory output for exporting (may be nil)
func (h *WindowsHost) ActiveDirectoryOutput() *activedirectory.Output {
	return h.ActiveDirectory
}

// DisableFakeIntake marks FakeIntake as not provisioned (sets to nil)
func (h *WindowsHost) DisableFakeIntake() {
	h.FakeIntake = nil
}

// DisableAgent marks Agent as not provisioned (sets to nil)
func (h *WindowsHost) DisableAgent() {
	h.Agent = nil
}

// DisableActiveDirectory marks ActiveDirectory as not provisioned (sets to nil)
func (h *WindowsHost) DisableActiveDirectory() {
	h.ActiveDirectory = nil
}

// SetAgentClientOptions sets the agent client options
func (h *WindowsHost) SetAgentClientOptions(options ...agentclientparams.Option) {
	return
}

// SetEnvironment is a no-op for outputs (only used in test environments)
func (h *WindowsHost) SetEnvironment(env config.Env) {
	return
}

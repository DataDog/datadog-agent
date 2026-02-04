// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package outputs contains lightweight output structs for scenarios.
// These structs only contain *Output types and have no test client dependencies.
// Use these in Run functions to avoid pulling in heavy test dependencies.
//
// The testing layer (environments package) embeds these outputs and adds client
// functionality on top. This separation ensures that running scenarios via
// pulumi doesn't pull in test framework dependencies like testify, k8s client, etc.
package outputs

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/updater"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
)

// HostOutputs is the interface that both outputs.Host and environments.Host implement.
// This allows Run functions to accept either type, enabling:
// - Scenarios (main.go) to use lightweight outputs.Host without test dependencies
// - Provisioners (test code) to use environments.Host with full client support
type HostOutputs interface {
	RemoteHostOutput() *remote.HostOutput
	FakeIntakeOutput() *fakeintake.FakeintakeOutput
	AgentOutput() *agent.HostAgentOutput
	UpdaterOutput() *updater.HostUpdaterOutput
	DisableFakeIntake()
	DisableAgent()
	DisableUpdater()
	SetAgentClientOptions(options ...agentclientparams.Option)
}

// Host contains the outputs for a Host environment.
// This struct is lightweight and does not import test client dependencies.
type Host struct {
	RemoteHost *remote.HostOutput
	FakeIntake *fakeintake.FakeintakeOutput
	Agent      *agent.HostAgentOutput
	Updater    *updater.HostUpdaterOutput
}

// NewHost creates a new Host output struct with all fields initialized.
func NewHost() *Host {
	return &Host{
		RemoteHost: &remote.HostOutput{},
		FakeIntake: &fakeintake.FakeintakeOutput{},
		Agent:      &agent.HostAgentOutput{},
		Updater:    &updater.HostUpdaterOutput{},
	}
}

// RemoteHostOutput returns the remote host output for exporting
func (h *Host) RemoteHostOutput() *remote.HostOutput {
	return h.RemoteHost
}

// FakeIntakeOutput returns the fakeintake output for exporting (may be nil)
func (h *Host) FakeIntakeOutput() *fakeintake.FakeintakeOutput {
	return h.FakeIntake
}

// AgentOutput returns the agent output for exporting (may be nil)
func (h *Host) AgentOutput() *agent.HostAgentOutput {
	return h.Agent
}

// UpdaterOutput returns the updater output for exporting (may be nil)
func (h *Host) UpdaterOutput() *updater.HostUpdaterOutput {
	return h.Updater
}

// DisableFakeIntake marks FakeIntake as not provisioned (sets to nil)
func (h *Host) DisableFakeIntake() {
	h.FakeIntake = nil
}

// DisableAgent marks Agent as not provisioned (sets to nil)
func (h *Host) DisableAgent() {
	h.Agent = nil
}

// DisableUpdater marks Updater as not provisioned (sets to nil)
func (h *Host) DisableUpdater() {
	h.Updater = nil
}

// SetAgentClientOptions sets the agent client options
func (h *Host) SetAgentClientOptions(options ...agentclientparams.Option) {
	return
}

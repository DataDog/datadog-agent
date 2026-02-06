// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package outputs

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
)

// DockerHostOutputs is the interface for DockerHost environment outputs.
type DockerHostOutputs interface {
	RemoteHostOutput() *remote.HostOutput
	FakeIntakeOutput() *fakeintake.FakeintakeOutput
	DockerAgentOutput() *agent.DockerAgentOutput
	DockerOutput() *docker.ManagerOutput
	DisableFakeIntake()
	DisableAgent()
}

// DockerHost contains the outputs for a DockerHost environment.
type DockerHost struct {
	RemoteHost *remote.HostOutput
	FakeIntake *fakeintake.FakeintakeOutput
	Agent      *agent.DockerAgentOutput
	Docker     *docker.ManagerOutput
}

// NewDockerHost creates a new DockerHost output struct with all fields initialized.
func NewDockerHost() *DockerHost {
	return &DockerHost{
		RemoteHost: &remote.HostOutput{},
		FakeIntake: &fakeintake.FakeintakeOutput{},
		Agent:      &agent.DockerAgentOutput{},
		Docker:     &docker.ManagerOutput{},
	}
}

// RemoteHostOutput returns the remote host output for exporting
func (h *DockerHost) RemoteHostOutput() *remote.HostOutput {
	return h.RemoteHost
}

// FakeIntakeOutput returns the fakeintake output for exporting (may be nil)
func (h *DockerHost) FakeIntakeOutput() *fakeintake.FakeintakeOutput {
	return h.FakeIntake
}

// DockerAgentOutput returns the Docker agent output for exporting (may be nil)
func (h *DockerHost) DockerAgentOutput() *agent.DockerAgentOutput {
	return h.Agent
}

// DockerOutput returns the Docker manager output for exporting
func (h *DockerHost) DockerOutput() *docker.ManagerOutput {
	return h.Docker
}

// DisableFakeIntake marks FakeIntake as not provisioned (sets to nil)
func (h *DockerHost) DisableFakeIntake() {
	h.FakeIntake = nil
}

// DisableAgent marks Agent as not provisioned (sets to nil)
func (h *DockerHost) DisableAgent() {
	h.Agent = nil
}

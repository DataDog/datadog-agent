// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ec2host is the reference unified scenario: an AWS EC2 host with the agent.
package ec2host

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario/params"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
)

// EC2HostParams is the canonical, introspectable parameter set for the ec2-host scenario.
type EC2HostParams struct {
	OS   string `scenario:"name=os,default=ubuntu-22.04,help=Operating system,enum=ubuntu-22.04|debian-12|amazon-linux-2023"`
	Arch string `scenario:"name=arch,default=x86_64,help=CPU architecture,enum=x86_64|arm64"`

	// Agent holds agent-installation parameters.
	//
	// WARNING: Agent.Install defaults to false for Go struct-literal callers — the
	// schema default ("install-agent=true") is only applied by the CLI/service via
	// Decode. Use NewEC2HostParams to get a safe default with Install=true.
	Agent      params.AgentParams      // embedded → agent flags
	Fakeintake params.FakeintakeParams // embedded → fakeintake flags

	// InstanceOptions is a Go-only escape hatch for advanced VM tuning.
	InstanceOptions []ec2.VMOption `scenario:"-"`
}

// NewEC2HostParams returns EC2HostParams with safe defaults for Go callers
// (notably Agent.Install=true, matching the schema default that the CLI/service
// apply via Decode). Go struct-literal callers otherwise get Install=false.
func NewEC2HostParams(os, arch string) *EC2HostParams {
	return &EC2HostParams{OS: os, Arch: arch, Agent: params.AgentParams{Install: true}}
}

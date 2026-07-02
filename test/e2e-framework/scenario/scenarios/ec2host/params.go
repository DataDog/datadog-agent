// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ec2host is the reference unified scenario: an AWS EC2 host with the agent.
package ec2host

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario/params"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
)

// EC2HostParams is the canonical, introspectable parameter set for the ec2-host scenario.
type EC2HostParams struct {
	OS   string `scenario:"name=os,default=ubuntu-22.04,help=Operating system,enum=ubuntu-22.04|debian-12|amazon-linux-2023"`
	Arch string `scenario:"name=arch,default=x86_64,help=CPU architecture,enum=x86_64|arm64"`

	// Agent holds agent-installation parameters.
	Agent      params.AgentParams      // embedded → agent flags
	Fakeintake params.FakeintakeParams // embedded → fakeintake flags

	// InstanceOptions is a Go-only escape hatch for advanced VM tuning.
	InstanceOptions []ec2.VMOption `scenario:"-"`
}

// NewParams returns fully-defaulted EC2HostParams (os=ubuntu-22.04, arch=x86_64,
// install-agent=true, …). This is the blessed constructor: it yields the same
// values the CLI produces for empty input. Override fields as needed.
func NewParams() *EC2HostParams { return scenario.NewParams[EC2HostParams]() }

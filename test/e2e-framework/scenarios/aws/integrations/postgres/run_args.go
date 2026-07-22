// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package postgres contains the aws/postgres Agent E2E scenario: a single EC2
// host running the host-installed Datadog Agent plus a PostgreSQL 16 Docker
// workload that the postgres integration monitors over TCP.
package postgres

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	compos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

const (
	// defaultVMName is the resource name of the host. The AWS namer prefixes it
	// with "aws", so the exported host output key is dd-Host-aws-agent-host and
	// the invoke task role is "aws-agent-host".
	defaultVMName = "agent-host"

	// defaultInstanceType matches the capacity plan's selected_infra target.
	defaultInstanceType = "t3.large"
)

// Params contains the parameters for the aws/postgres scenario.
type Params struct {
	Name         string
	instanceType string
	vmOptions    []ec2.VMOption
	agentOptions []agentparams.Option
	installAgent bool
}

func newParams() *Params {
	return &Params{
		Name:         defaultVMName,
		instanceType: defaultInstanceType,
		vmOptions:    []ec2.VMOption{},
		agentOptions: []agentparams.Option{},
		installAgent: true,
	}
}

// GetParams returns Params from the option setters.
func GetParams(opts ...Option) (*Params, error) {
	params := newParams()
	if err := optional.ApplyOptions(params, opts); err != nil {
		return nil, fmt.Errorf("unable to apply Option: %w", err)
	}
	return params, nil
}

// ParamsFromEnvironment builds Params from the AWS environment ConfigMap flags,
// mirroring the conventions used by the aws/dockervm and aws/vm scenarios.
func ParamsFromEnvironment(e aws.Environment) *Params {
	p := newParams()

	osDesc := compos.DescriptorFromString(e.InfraOSDescriptor(), compos.UbuntuDefault)
	if img := e.InfraOSImageID(); img != "" {
		p.vmOptions = append(p.vmOptions, ec2.WithAMI(img, osDesc, osDesc.Architecture))
	} else if e.InfraOSDescriptor() != "" {
		p.vmOptions = append(p.vmOptions, ec2.WithOS(osDesc))
	}

	if !e.AgentDeploy() {
		p.installAgent = false
		p.agentOptions = nil
	} else {
		if v := e.AgentVersion(); v != "" {
			p.agentOptions = append(p.agentOptions, agentparams.WithVersion(v))
		}
	}

	return p
}

// Option mutates Params.
type Option func(*Params) error

// WithName sets the host resource name.
func WithName(name string) Option {
	return func(p *Params) error {
		p.Name = name
		return nil
	}
}

// WithInstanceType overrides the EC2 instance type.
func WithInstanceType(instanceType string) Option {
	return func(p *Params) error {
		p.instanceType = instanceType
		return nil
	}
}

// WithEC2VMOptions appends EC2 VM options.
func WithEC2VMOptions(opts ...ec2.VMOption) Option {
	return func(p *Params) error {
		p.vmOptions = append(p.vmOptions, opts...)
		return nil
	}
}

// WithAgentOptions appends host Agent options.
func WithAgentOptions(opts ...agentparams.Option) Option {
	return func(p *Params) error {
		p.agentOptions = append(p.agentOptions, opts...)
		return nil
	}
}

// WithoutAgent disables installing the host Agent.
func WithoutAgent() Option {
	return func(p *Params) error {
		p.installAgent = false
		p.agentOptions = nil
		return nil
	}
}

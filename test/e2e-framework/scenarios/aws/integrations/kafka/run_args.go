// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kafka

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

const (
	// defaultVMName is the VM resource name. The framework exports the host
	// outputs under "dd-Host-aws-<defaultVMName>" (the aws- prefix comes from
	// the AWS environment namer applied to the EC2 component name). The
	// generated task reads role "agent-host", so the host alias export below
	// must be keyed "dd-Host-agent-host".
	defaultVMName = "agent-host"

	// defaultInstanceType matches lab.json capacity_plan.roles[].selected_infra.target.
	defaultInstanceType = "t3.large"
)

// Params is the set of parameters for the Kafka lab scenario.
type Params struct {
	Name string

	instanceOptions   []ec2.VMOption
	agentOptions      []agentparams.Option
	fakeintakeOptions []fakeintake.Option
}

func newParams() *Params {
	return &Params{
		Name:              defaultVMName,
		instanceOptions:   []ec2.VMOption{},
		agentOptions:      []agentparams.Option{},
		fakeintakeOptions: []fakeintake.Option{},
	}
}

// GetParams returns Params from the given options.
func GetParams(opts ...Option) *Params {
	params := newParams()
	if err := optional.ApplyOptions(params, opts); err != nil {
		panic(fmt.Errorf("unable to apply Option, err: %w", err))
	}
	return params
}

// ParamsFromEnvironment builds Params by reading configuration from the AWS environment.
func ParamsFromEnvironment(e aws.Environment) *Params {
	p := newParams()

	// Pin the instance type to the capacity-planned target unless the
	// environment explicitly overrides it via ddinfra:instanceType.
	p.instanceOptions = append(p.instanceOptions, ec2.WithInstanceType(defaultInstanceType))

	// Agent installation toggle.
	if !e.AgentDeploy() {
		p.agentOptions = nil
	}

	// Fakeintake is opt-in only.
	if e.AgentUseFakeintake() {
		fiOpts := []fakeintake.Option{}
		if e.InfraShouldDeployFakeintakeWithLB() {
			fiOpts = append(fiOpts, fakeintake.WithLoadBalancer())
		}
		if retention := e.AgentFakeintakeRetentionPeriod(); retention != "" {
			fiOpts = append(fiOpts, fakeintake.WithRetentionPeriod(retention))
		}
		p.fakeintakeOptions = fiOpts
	} else {
		p.fakeintakeOptions = nil
	}

	return p
}

// Option is a scenario option.
type Option func(*Params) error

// WithName sets the scenario VM name.
func WithName(name string) Option {
	return func(params *Params) error {
		params.Name = name
		return nil
	}
}

// WithEC2InstanceOptions adds options to the EC2 VM.
func WithEC2InstanceOptions(opts ...ec2.VMOption) Option {
	return func(params *Params) error {
		params.instanceOptions = append(params.instanceOptions, opts...)
		return nil
	}
}

// WithAgentOptions adds options to the Agent.
func WithAgentOptions(opts ...agentparams.Option) Option {
	return func(params *Params) error {
		params.agentOptions = append(params.agentOptions, opts...)
		return nil
	}
}

// WithFakeIntakeOptions adds options to the FakeIntake.
func WithFakeIntakeOptions(opts ...fakeintake.Option) Option {
	return func(params *Params) error {
		params.fakeintakeOptions = append(params.fakeintakeOptions, opts...)
		return nil
	}
}

// WithoutFakeIntake disables the creation of the FakeIntake.
func WithoutFakeIntake() Option {
	return func(params *Params) error {
		params.fakeintakeOptions = nil
		return nil
	}
}

// WithoutAgent disables the creation of the Agent.
func WithoutAgent() Option {
	return func(params *Params) error {
		params.agentOptions = nil
		return nil
	}
}

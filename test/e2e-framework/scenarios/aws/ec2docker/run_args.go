// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ec2docker contains the definition of the AWS Docker environment.
package ec2docker

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	compos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	defaultVMName = "dockervm"
)

type preAgentInstallHook func(*aws.Environment, *remote.Host) (pulumi.Resource, error)

// Params contains all the parameters needed to create the environment
type Params struct {
	Name string

	vmOptions            []ec2.VMOption
	agentOptions         []dockeragentparams.Option
	fakeintakeOptions    []fakeintake.Option
	preAgentInstallHooks []preAgentInstallHook
	testingWorkload      bool
}

func newParams() *Params {
	// We use nil arrays to decide if we should create or not
	return &Params{
		Name:              defaultVMName,
		vmOptions:         []ec2.VMOption{},
		agentOptions:      []dockeragentparams.Option{},
		fakeintakeOptions: []fakeintake.Option{},
	}
}

// GetParams return Params from options opts setup
func GetParams(opts ...Option) *Params {
	params := newParams()
	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(fmt.Errorf("unable to apply Option, err: %w", err))
	}
	return params
}

// ParamsFromEnvironment builds Params by reading the configuration from the given AWS environment.
// It maps ConfigMap-driven flags to the Docker-on-EC2 scenario run parameters.
func ParamsFromEnvironment(e aws.Environment) *Params {
	p := newParams()

	// VM OS/AMI selection
	osDesc := compos.DescriptorFromString(e.InfraOSDescriptor(), compos.UbuntuDefault)
	if img := e.InfraOSImageID(); img != "" {
		p.vmOptions = append(p.vmOptions, ec2.WithAMI(img, osDesc, osDesc.Architecture))
	} else {
		if e.InfraOSDescriptor() != "" {
			p.vmOptions = append(p.vmOptions, ec2.WithOS(osDesc))
		}
		if e.InfraOSImageIDUseLatest() {
			p.vmOptions = append(p.vmOptions, ec2.WithLatestAMI())
		}
	}

	// Agent installation and image selection
	if !e.AgentDeploy() {
		p.agentOptions = nil
	} else {
		if full := e.AgentFullImagePath(); full != "" {
			p.agentOptions = append(p.agentOptions, dockeragentparams.WithFullImagePath(full))
		} else if v := e.AgentVersion(); v != "" {
			p.agentOptions = append(p.agentOptions, dockeragentparams.WithImageTag(v))
		}
		if e.AgentJMX() {
			p.agentOptions = append(p.agentOptions, dockeragentparams.WithJMX())
		}
		if e.AgentFIPS() {
			p.agentOptions = append(p.agentOptions, dockeragentparams.WithFIPS())
		}
	}

	// Fakeintake options
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

	// Testing workload toggle
	if e.TestingWorkloadDeploy() {
		p.testingWorkload = true
	}

	return p
}

// Option is a function that modifies the Params
type Option func(*Params) error

// WithName sets the name of the provisioner
func WithName(name string) Option {
	return func(params *Params) error {
		params.Name = name
		return nil
	}
}

// WithEC2VMOptions sets the options for the EC2 VM
func WithEC2VMOptions(opts ...ec2.VMOption) Option {
	return func(params *Params) error {
		params.vmOptions = append(params.vmOptions, opts...)
		return nil
	}
}

// WithAgentOptions sets the options for the Docker Agent
func WithAgentOptions(opts ...dockeragentparams.Option) Option {
	return func(params *Params) error {
		params.agentOptions = append(params.agentOptions, opts...)
		return nil
	}
}

// WithFakeIntakeOptions sets the options for the FakeIntake
func WithFakeIntakeOptions(opts ...fakeintake.Option) Option {
	return func(params *Params) error {
		params.fakeintakeOptions = append(params.fakeintakeOptions, opts...)
		return nil
	}
}

// WithoutFakeIntake deactivates the creation of the FakeIntake
func WithoutFakeIntake() Option {
	return func(params *Params) error {
		params.fakeintakeOptions = nil
		return nil
	}
}

// WithoutAgent deactivates the creation of the Docker Agent
func WithoutAgent() Option {
	return func(params *Params) error {
		params.agentOptions = nil
		return nil
	}
}

// WithTestingWorkload enables testing workload
func WithTestingWorkload() Option {
	return func(params *Params) error {
		params.testingWorkload = true
		return nil
	}
}

// WithPreAgentInstallHook adds a callback between host setup end and the agent starting up.
func WithPreAgentInstallHook(cb preAgentInstallHook) Option {
	return func(params *Params) error {
		params.preAgentInstallHooks = append(params.preAgentInstallHooks, cb)
		return nil
	}
}

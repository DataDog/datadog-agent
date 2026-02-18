// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ec2

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	compos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

const (
	defaultVMName = "vm"
)

// Params is a set of parameters for the Host environment.
type Params struct {
	Name string

	instanceOptions    []VMOption
	agentOptions       []agentparams.Option
	agentClientOptions []agentclientparams.Option
	fakeintakeOptions  []fakeintake.Option
	installDocker      bool
	installUpdater     bool
}

func newParams() *Params {
	// We use nil arrays to decide if we should create or not
	return &Params{
		Name:               defaultVMName,
		instanceOptions:    []VMOption{},
		agentOptions:       []agentparams.Option{},
		agentClientOptions: []agentclientparams.Option{},
		fakeintakeOptions:  []fakeintake.Option{},
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
// It maps ConfigMap-driven flags to the EC2 scenario run parameters, keeping sensible defaults.
func ParamsFromEnvironment(e aws.Environment) *Params {
	p := newParams()

	// VM OS/AMI selection
	osDesc := compos.DescriptorFromString(e.InfraOSDescriptor(), compos.UbuntuDefault)
	if img := e.InfraOSImageID(); img != "" {
		p.instanceOptions = append(p.instanceOptions, WithAMI(img, osDesc, osDesc.Architecture))
	} else {
		// Use provided descriptor if any
		if e.InfraOSDescriptor() != "" {
			p.instanceOptions = append(p.instanceOptions, WithOS(osDesc))
		}
		if e.InfraOSImageIDUseLatest() {
			p.instanceOptions = append(p.instanceOptions, WithLatestAMI())
		}
	}

	// Agent installation toggle
	if !e.AgentDeploy() {
		p.agentOptions = nil
	}

	// Updater toggle
	if e.UpdaterDeploy() {
		p.installUpdater = true
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

	// Nothing to set for installDocker from environment at the moment.

	return p
}

// Option is a provisioner option.
type Option func(*Params) error

// WithName sets the name of the provisioner.
func WithName(name string) Option {
	return func(params *Params) error {
		params.Name = name
		return nil
	}
}

// WithEC2InstanceOptions adds options to the EC2 VM.
func WithEC2InstanceOptions(opts ...VMOption) Option {
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

// WithAgentClientOptions adds options to the Agent client.
func WithAgentClientOptions(opts ...agentclientparams.Option) Option {
	return func(params *Params) error {
		params.agentClientOptions = append(params.agentClientOptions, opts...)
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

// WithUpdater installs the agent through the updater.
func WithUpdater() Option {
	return func(params *Params) error {
		params.installUpdater = true
		return nil
	}
}

// WithDocker installs docker on the VM
func WithDocker() Option {
	return func(params *Params) error {
		params.installDocker = true
		return nil
	}
}

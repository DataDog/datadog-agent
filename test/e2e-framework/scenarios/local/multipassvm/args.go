// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package localmultipassvm

import (
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/local"
	localmultipass "github.com/DataDog/datadog-agent/test/e2e-framework/resources/local/vm/multipass"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

// VMOption enables the options pattern with VMArgs
type VMOption = func(args *localmultipass.VMArgs) error

func WithCPUs(cpus string) VMOption {
	return func(args *localmultipass.VMArgs) error {
		args.CPUs = cpus
		return nil
	}
}

func WithMemory(mem string) VMOption {
	return func(args *localmultipass.VMArgs) error {
		args.Memory = mem
		return nil
	}
}

func WithDisk(disk string) VMOption {
	return func(args *localmultipass.VMArgs) error {
		args.Disk = disk
		return nil
	}
}

// WithPulumiResourceOptions adds Pulumi resource options to the VM (e.g. pulumi.DependsOn, pulumi.Provider).
func WithPulumiResourceOptions(opts ...pulumi.ResourceOption) VMOption {
	return func(args *localmultipass.VMArgs) error {
		args.PulumiResourceOptions = append(args.PulumiResourceOptions, opts...)
		return nil
	}
}

const defaultVMName = "vm"

// Params is a set of parameters for the local multipass VM environment.
type Params struct {
	Name             string
	vmOptions        []VMOption
	agentOptions     []agentparams.Option
	deployFakeIntake bool
}

func newParams() *Params {
	return &Params{
		Name:             defaultVMName,
		vmOptions:        []VMOption{},
		agentOptions:     []agentparams.Option{},
		deployFakeIntake: true,
	}
}

// GetParams builds Params from the given options.
func GetParams(opts ...Option) *Params {
	params := newParams()
	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(fmt.Errorf("unable to apply Option, err: %w", err))
	}
	return params
}

// Option is a provisioner option.
type Option func(*Params) error

// WithName sets the name of the VM.
func WithName(name string) Option {
	return func(p *Params) error {
		p.Name = name
		return nil
	}
}

// WithInstanceOptions adds options to the multipass VM.
func WithInstanceOptions(opts ...VMOption) Option {
	return func(p *Params) error {
		p.vmOptions = append(p.vmOptions, opts...)
		return nil
	}
}

// WithAgentOptions adds options to the Agent.
func WithAgentOptions(opts ...agentparams.Option) Option {
	return func(p *Params) error {
		p.agentOptions = append(p.agentOptions, opts...)
		return nil
	}
}

// WithoutFakeIntake disables the creation of the FakeIntake.
func WithoutFakeIntake() Option {
	return func(p *Params) error {
		p.deployFakeIntake = false
		return nil
	}
}

// WithoutAgent disables the creation of the Agent.
func WithoutAgent() Option {
	return func(p *Params) error {
		p.agentOptions = nil
		return nil
	}
}

// ParamsFromEnvironment builds Params by reading configuration from the given local environment.
// It maps ConfigMap-driven flags to the multipass VM run parameters, keeping sensible defaults.
func ParamsFromEnvironment(e local.Environment) *Params {
	p := newParams()

	p.Name = e.VMHostname()

	// VM sizing
	if cpus := e.VMCPUs(); cpus != "" {
		p.vmOptions = append(p.vmOptions, WithCPUs(cpus))
	}
	if mem := e.VMMemory(); mem != "" {
		p.vmOptions = append(p.vmOptions, WithMemory(mem))
	}
	if disk := e.VMDisk(); disk != "" {
		p.vmOptions = append(p.vmOptions, WithDisk(disk))
	}

	// Agent installation toggle
	if !e.AgentDeploy() {
		p.agentOptions = nil
	}

	// FakeIntake toggle
	p.deployFakeIntake = e.AgentUseFakeintake()

	return p
}

func buildArgs(options ...VMOption) (*localmultipass.VMArgs, error) {
	vmArgs := &localmultipass.VMArgs{}
	vmArgs.PulumiResourceOptions = []pulumi.ResourceOption{}
	return common.ApplyOption(vmArgs, options)
}

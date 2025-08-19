// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package winazurehost

import (
	"fmt"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/pkg/components/datadog-installer"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/components/defender"
	"github.com/DataDog/test-infra-definitions/components/activedirectory"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/azure/compute"
	"github.com/DataDog/test-infra-definitions/scenarios/azure/fakeintake"
)

// ProvisionerParams is a set of parameters for the Provisioner.
type ProvisionerParams struct {
	name string

	instanceOptions        []compute.VMOption
	agentOptions           []agentparams.Option
	agentClientOptions     []agentclientparams.Option
	fakeintakeOptions      []fakeintake.Option
	activeDirectoryOptions []activedirectory.Option
	defenderOptions        []defender.Option
	installerOptions       []installer.Option
}

// ProvisionerOption is a provisioner option.
type ProvisionerOption func(*ProvisionerParams) error

// WithName sets the name of the provisioner.
func WithName(name string) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.name = name
		return nil
	}
}

// WithInstanceOptions adds options to the VM.
func WithInstanceOptions(opts ...compute.VMOption) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.instanceOptions = append(params.instanceOptions, opts...)
		return nil
	}
}

// WithAgentOptions adds options to the Agent.
func WithAgentOptions(opts ...agentparams.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = append(params.agentOptions, opts...)
		return nil
	}
}

// WithoutAgent disables the creation of the Agent.
func WithoutAgent() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = nil
		return nil
	}
}

// WithAgentClientOptions adds options to the Agent client.
func WithAgentClientOptions(opts ...agentclientparams.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentClientOptions = append(params.agentClientOptions, opts...)
		return nil
	}
}

// WithFakeIntakeOptions adds options to the FakeIntake.
func WithFakeIntakeOptions(opts ...fakeintake.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = append(params.fakeintakeOptions, opts...)
		return nil
	}
}

// WithoutFakeIntake disables the creation of the FakeIntake.
func WithoutFakeIntake() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = nil
		return nil
	}
}

// WithActiveDirectoryOptions adds Active Directory to the EC2 VM.
func WithActiveDirectoryOptions(opts ...activedirectory.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.activeDirectoryOptions = append(params.activeDirectoryOptions, opts...)
		return nil
	}
}

// WithDefenderOptions configures Windows Defender on an EC2 VM.
func WithDefenderOptions(opts ...defender.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.defenderOptions = append(params.defenderOptions, opts...)
		return nil
	}
}

// WithInstaller configures Datadog Installer on an EC2 VM.
func WithInstaller(opts ...installer.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.installerOptions = []installer.Option{}
		params.installerOptions = append(params.installerOptions, opts...)
		return nil
	}
}

func getProvisionerParams(opts ...ProvisionerOption) *ProvisionerParams {
	params := &ProvisionerParams{
		name:               defaultVMName,
		instanceOptions:    []compute.VMOption{},
		agentOptions:       []agentparams.Option{},
		agentClientOptions: []agentclientparams.Option{},
		fakeintakeOptions:  []fakeintake.Option{},
		// Disable Windows Defender on VMs by default
		defenderOptions: []defender.Option{defender.WithDefenderDisabled()},
	}
	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(fmt.Errorf("unable to apply ProvisionerOption, err: %w", err))
	}
	return params
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package winazurehost contains the definition of the Azure Windows Host environment.
package winazurehost

import (
	installer "github.com/DataDog/datadog-agent/test/new-e2e/pkg/components/datadog-installer"
	"github.com/DataDog/test-infra-definitions/components/activedirectory"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/resources/azure"
	"github.com/DataDog/test-infra-definitions/scenarios/azure/compute"
	"github.com/DataDog/test-infra-definitions/scenarios/azure/fakeintake"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/components/defender"
)

const (
	provisionerBaseID = "azure-vm-"
	defaultVMName     = "vm"
)

// Provisioner creates a VM environment with a Windows VM, a FakeIntake and a Host Agent configured to talk to each other.
// FakeIntake and Agent creation can be deactivated by using [WithoutFakeIntake] and [WithoutAgent] options.
func Provisioner(opts ...ProvisionerOption) e2e.TypedProvisioner[environments.WindowsHost] {
	// We need to build params here to be able to use params.name in the provisioner name
	params := getProvisionerParams(opts...)
	provisioner := e2e.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.WindowsHost) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard-to-debug issues.
		params := getProvisionerParams(opts...)
		return Run(ctx, env, params)
	}, nil)

	return provisioner
}

// ProvisionerNoAgent wraps Provisioner with hardcoded WithoutAgent options.
func ProvisionerNoAgent(opts ...ProvisionerOption) e2e.TypedProvisioner[environments.WindowsHost] {
	mergedOpts := make([]ProvisionerOption, 0, len(opts)+1)
	mergedOpts = append(mergedOpts, opts...)
	mergedOpts = append(mergedOpts, WithoutAgent())

	return Provisioner(mergedOpts...)
}

// ProvisionerNoAgentNoFakeIntake wraps Provisioner with hardcoded WithoutAgent and WithoutFakeIntake options.
func ProvisionerNoAgentNoFakeIntake(opts ...ProvisionerOption) e2e.TypedProvisioner[environments.WindowsHost] {
	mergedOpts := make([]ProvisionerOption, 0, len(opts)+2)
	mergedOpts = append(mergedOpts, opts...)
	mergedOpts = append(mergedOpts, WithoutAgent(), WithoutFakeIntake())

	return Provisioner(mergedOpts...)
}

// ProvisionerNoFakeIntake wraps Provisioner with hardcoded WithoutFakeIntake option.
func ProvisionerNoFakeIntake(opts ...ProvisionerOption) e2e.TypedProvisioner[environments.WindowsHost] {
	mergedOpts := make([]ProvisionerOption, 0, len(opts)+1)
	mergedOpts = append(mergedOpts, opts...)
	mergedOpts = append(mergedOpts, WithoutFakeIntake())

	return Provisioner(mergedOpts...)
}

// Run deploys a Windows environment given a pulumi.Context
func Run(ctx *pulumi.Context, env *environments.WindowsHost, params *ProvisionerParams) error {
	azureEnv, err := azure.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	host, err := compute.NewVM(azureEnv, params.name, params.instanceOptions...)
	if err != nil {
		return err
	}
	err = host.Export(ctx, &env.RemoteHost.HostOutput)
	if err != nil {
		return err
	}

	if params.defenderOptions != nil {
		defender, err := defender.NewDefender(azureEnv.CommonEnvironment, host, params.defenderOptions...)
		if err != nil {
			return err
		}
		// Active Directory setup needs to happen after Windows Defender setup
		params.activeDirectoryOptions = append(params.activeDirectoryOptions,
			activedirectory.WithPulumiResourceOptions(
				pulumi.DependsOn(defender.Resources)))
	}

	if params.activeDirectoryOptions != nil {
		activeDirectoryComp, activeDirectoryResources, err := activedirectory.NewActiveDirectory(ctx, &azureEnv, host, params.activeDirectoryOptions...)
		if err != nil {
			return err
		}
		err = activeDirectoryComp.Export(ctx, &env.ActiveDirectory.Output)
		if err != nil {
			return err
		}

		if params.agentOptions != nil {
			// Agent install needs to happen after ActiveDirectory setup
			params.agentOptions = append(params.agentOptions,
				agentparams.WithPulumiResourceOptions(
					pulumi.DependsOn(activeDirectoryResources)))
		}
	} else {
		// Suite inits all fields by default, so we need to explicitly set it to nil
		env.ActiveDirectory = nil
	}

	// Create FakeIntake if required
	if params.fakeintakeOptions != nil {
		fakeIntake, err := fakeintake.NewVMInstance(azureEnv, params.fakeintakeOptions...)
		if err != nil {
			return err
		}
		err = fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)
		if err != nil {
			return err
		}
		// Normally if FakeIntake is enabled, Agent is enabled, but just in case
		if params.agentOptions != nil {
			// Prepend in case it's overridden by the user
			newOpts := []agentparams.Option{agentparams.WithFakeintake(fakeIntake)}
			params.agentOptions = append(newOpts, params.agentOptions...)
		}
	} else {
		env.FakeIntake = nil
	}

	if params.agentOptions != nil {
		agent, err := agent.NewHostAgent(&azureEnv, host, params.agentOptions...)
		if err != nil {
			return err
		}
		err = agent.Export(ctx, &env.Agent.HostAgentOutput)
		if err != nil {
			return err
		}
		env.Agent.ClientOptions = params.agentClientOptions
	} else {
		env.Agent = nil
	}

	if params.installerOptions != nil {
		installer, err := installer.NewInstaller(&azureEnv, host, params.installerOptions...)
		if err != nil {
			return err
		}
		err = installer.Export(ctx, &env.Installer.Output)
		if err != nil {
			return err
		}
	} else {
		env.Installer = nil
	}

	return nil
}

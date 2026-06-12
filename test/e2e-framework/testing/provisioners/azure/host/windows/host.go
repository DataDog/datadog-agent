// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package winazurehost contains the definition of the Azure Windows Host environment.
package winazurehost

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/activedirectory"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/azure"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/azure/compute"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/azure/fakeintake"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/windows/defender"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/hostagent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

const (
	provisionerBaseID = "azure-vm-"
	defaultVMName     = "vm"
)

// Provisioner creates a VM environment with a Windows Azure VM, a FakeIntake
// and a Host Agent configured to talk to each other.
//
// Agent installation is performed via MSI over SSH after Pulumi provisions the
// VM and FakeIntake (PostProvision). Active Directory is still set up by Pulumi
// before PostProvision runs, so ordering is preserved. FakeIntake and Agent
// creation can be deactivated by using [WithoutFakeIntake] and [WithoutAgent]
// options.
func Provisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.WindowsHost] {
	// Capture user-provided agent options outside the closure so PostProvision
	// receives clean options.
	params := getProvisionerParams(opts...)
	agentOpts := params.agentOptions
	usePostProvision := agentOpts != nil

	pulumiProv := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.WindowsHost) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard-to-debug issues.
		params := getProvisionerParams(opts...)
		if usePostProvision {
			params.agentOptions = nil
		}
		return Run(ctx, env, params)
	}, nil)

	if !usePostProvision {
		return pulumiProv
	}

	return provisioners.WithPostProvision(pulumiProv, func(t *testing.T, env *environments.WindowsHost) {
		hostagent.InstallOnWindowsHost(t, env, agentOpts...)
	})
}

// ProvisionerNoAgent wraps Provisioner with hardcoded WithoutAgent options.
func ProvisionerNoAgent(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.WindowsHost] {
	mergedOpts := make([]ProvisionerOption, 0, len(opts)+1)
	mergedOpts = append(mergedOpts, opts...)
	mergedOpts = append(mergedOpts, WithoutAgent())

	return Provisioner(mergedOpts...)
}

// ProvisionerNoAgentNoFakeIntake wraps Provisioner with hardcoded WithoutAgent and WithoutFakeIntake options.
func ProvisionerNoAgentNoFakeIntake(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.WindowsHost] {
	mergedOpts := make([]ProvisionerOption, 0, len(opts)+2)
	mergedOpts = append(mergedOpts, opts...)
	mergedOpts = append(mergedOpts, WithoutAgent(), WithoutFakeIntake())

	return Provisioner(mergedOpts...)
}

// ProvisionerNoFakeIntake wraps Provisioner with hardcoded WithoutFakeIntake option.
func ProvisionerNoFakeIntake(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.WindowsHost] {
	mergedOpts := make([]ProvisionerOption, 0, len(opts)+1)
	mergedOpts = append(mergedOpts, opts...)
	mergedOpts = append(mergedOpts, WithoutFakeIntake())

	return Provisioner(mergedOpts...)
}

// Run deploys a Windows environment given a pulumi.Context.
// When agentOptions is nil (PostProvision handles the install), it only
// provisions the VM, optional Active Directory, and FakeIntake.
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
		def, err := defender.NewDefender(azureEnv.CommonEnvironment, host, params.defenderOptions...)
		if err != nil {
			return err
		}
		params.activeDirectoryOptions = append(params.activeDirectoryOptions,
			activedirectory.WithPulumiResourceOptions(pulumi.DependsOn(def.Resources)))
	}

	if params.activeDirectoryOptions != nil {
		activeDirectoryComp, _, err := activedirectory.NewActiveDirectory(ctx, &azureEnv, host, params.activeDirectoryOptions...)
		if err != nil {
			return err
		}
		err = activeDirectoryComp.Export(ctx, &env.ActiveDirectory.Output)
		if err != nil {
			return err
		}
	} else {
		env.ActiveDirectory = nil
	}

	if params.fakeintakeOptions != nil {
		fakeIntake, err := fakeintake.NewVMInstance(azureEnv, params.fakeintakeOptions...)
		if err != nil {
			return err
		}
		err = fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)
		if err != nil {
			return err
		}
	} else {
		env.FakeIntake = nil
	}

	// Agent installation is handled by PostProvision via hostagent.Install.
	env.Agent = nil

	return nil
}

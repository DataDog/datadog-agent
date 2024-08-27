// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package azurehost contains the definition of the Azure Host environment.
package azurehost

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/resources/azure"
	"github.com/DataDog/test-infra-definitions/scenarios/azure/compute"
	"github.com/DataDog/test-infra-definitions/scenarios/azure/fakeintake"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/updater"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	provisionerBaseID = "azure-vm-"
	defaultVMName     = "vm"
)

// Provisioner creates a VM environment with an VM, a FakeIntake and a Host Agent configured to talk to each other.
// FakeIntake and Agent creation can be deactivated by using [WithoutFakeIntake] and [WithoutAgent] options.
func Provisioner(opts ...ProvisionerOption) e2e.TypedProvisioner[environments.Host] {
	// We need to build params here to be able to use params.name in the provisioner name
	params := GetProvisionerParams(opts...)

	//TODO: Remove when https://datadoghq.atlassian.net/browse/ADXT-479 is done
	fmt.Println("PLEASE DO NOT USE THIS PROVIDER FOR TESTING ON THE CI YET. WE NEED TO FIND A WAY TO CLEAN INSTANCES FIRST")

	provisioner := e2e.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.Host) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard-to-debug issues.
		params := GetProvisionerParams(opts...)
		return Run(ctx, env, RunParams{ProvisionerParams: params})
	}, params.extraConfigParams)

	return provisioner
}

// Run deploys an environment given a pulumi.Context
func Run(ctx *pulumi.Context, env *environments.Host, runParams RunParams) error {
	var azureEnv azure.Environment
	if runParams.Environment == nil {
		var err error
		azureEnv, err = azure.NewEnvironment(ctx)
		if err != nil {
			return err
		}
	} else {
		azureEnv = *runParams.Environment
	}
	params := runParams.ProvisionerParams
	params.instanceOptions = append(params.instanceOptions, compute.WithOS(os.UbuntuDefault))

	host, err := compute.NewVM(azureEnv, params.name, params.instanceOptions...)
	if err != nil {
		return err
	}
	err = host.Export(ctx, &env.RemoteHost.HostOutput)
	if err != nil {
		return err
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
		// Suite inits all fields by default, so we need to explicitly set it to nil
		env.FakeIntake = nil
	}
	if !params.installUpdater {
		// Suite inits all fields by default, so we need to explicitly set it to nil
		env.Updater = nil
	}

	// Create Agent if required
	if params.installUpdater && params.agentOptions != nil {
		updater, err := updater.NewHostUpdater(&azureEnv, host, params.agentOptions...)
		if err != nil {
			return err
		}

		err = updater.Export(ctx, &env.Updater.HostUpdaterOutput)
		if err != nil {
			return err
		}
		// todo: add agent once updater installs agent on bootstrap
		env.Agent = nil
	} else if params.agentOptions != nil {
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
		// Suite inits all fields by default, so we need to explicitly set it to nil
		env.Agent = nil
	}

	return nil
}

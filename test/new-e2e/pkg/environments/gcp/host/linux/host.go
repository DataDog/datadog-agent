// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gcphost contains the definition of the GCP Host environment.
package gcphost

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/test-infra-definitions/resources/gcp"
	"github.com/DataDog/test-infra-definitions/scenarios/gcp/compute"
	"github.com/DataDog/test-infra-definitions/scenarios/gcp/fakeintake"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/updater"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	provisionerBaseID = "gcp-vm-"
	defaultVMName     = "vm"
)

// Provisioner creates a VM environment with an VM, a FakeIntake and a Host Agent configured to talk to each other.
// FakeIntake and Agent creation can be deactivated by using [WithoutFakeIntake] and [WithoutAgent] options.
func Provisioner(opts ...ProvisionerOption) e2e.TypedProvisioner[environments.Host] {
	// We need to build params here to be able to use params.name in the provisioner name
	params := GetProvisionerParams(opts...)

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
	var gcpEnv gcp.Environment
	if runParams.Environment == nil {
		var err error
		gcpEnv, err = gcp.NewEnvironment(ctx)
		if err != nil {
			return err
		}
	} else {
		gcpEnv = *runParams.Environment
	}
	params := runParams.ProvisionerParams

	host, err := compute.NewVM(gcpEnv, params.name, params.instanceOptions...)
	if err != nil {
		return err
	}
	err = host.Export(ctx, &env.RemoteHost.HostOutput)
	if err != nil {
		return err
	}

	// Create FakeIntake if required
	if params.fakeintakeOptions != nil {
		fakeIntake, err := fakeintake.NewVMInstance(gcpEnv, params.fakeintakeOptions...)
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
		updater, err := updater.NewHostUpdater(&gcpEnv, host, params.agentOptions...)
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
		agent, err := agent.NewHostAgent(&gcpEnv, host, params.agentOptions...)
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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gcphost contains the definition of the GCP Host environment.
package gcphost

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/gcp"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/compute"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/fakeintake"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/hostagent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/updater"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	provisionerBaseID = "gcp-vm-"
	defaultVMName     = "vm"
)

// Provisioner creates a VM environment with a GCP VM, a FakeIntake and a Host
// Agent configured to talk to each other.
//
// Agent installation is performed via SSH after Pulumi provisions the VM and
// FakeIntake (PostProvision), rather than inside Pulumi itself. This makes
// agent reconfiguration fast (no Pulumi cycle required for agent-only changes).
//
// FakeIntake and Agent creation can be deactivated by using [WithoutFakeIntake]
// and [WithoutAgent] options.
func Provisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Host] {
	// Capture user-provided agent options outside the closure so PostProvision
	// receives the clean options (before Pulumi would add the fakeintake resource).
	params := GetProvisionerParams(opts...)
	agentOpts := params.agentOptions
	usePostProvision := agentOpts != nil && !params.installUpdater

	pulumiProv := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.Host) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard-to-debug issues.
		params := GetProvisionerParams(opts...)
		if usePostProvision {
			// Tell Run not to install the agent – PostProvision handles it.
			params.agentOptions = nil
		}
		return Run(ctx, env, RunParams{ProvisionerParams: params})
	}, params.extraConfigParams)

	if !usePostProvision {
		return pulumiProv
	}

	return provisioners.WithPostProvision(pulumiProv, func(t *testing.T, env *environments.Host) {
		hostagent.Install(installers.FromT(t), env, agentOpts...)
	})
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

		// Only wire the Pulumi fakeintake resource into agent options for the
		// updater path; regular agent installs are handled by hostagent.Install
		// which reads env.FakeIntake directly.
		if params.installUpdater && params.agentOptions != nil {
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

	// Create Updater if required (agent install moved to PostProvision for regular installs)
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
	} else {
		// Agent installation is handled by PostProvision via hostagent.Install.
		// Suite inits all fields by default, so we need to explicitly set it to nil
		// so Init wires SetComponents correctly on the nil agent.
		env.Agent = nil
	}

	return nil
}

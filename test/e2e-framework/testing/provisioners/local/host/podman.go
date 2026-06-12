// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package localhost contains the provisioner for the local Host based environments
package localhost

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/hostagent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/local"

	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	localpodman "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/local/podman"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	provisionerBaseID = "local-host-"
	defaultName       = "podman-host"
)

// ProvisionerParams contains all the parameters needed to create the environment
type ProvisionerParams struct {
	name              string
	agentOptions      []agentparams.Option
	fakeintakeOptions []fakeintake.Option
	extraConfigParams runner.ConfigMap
}

func newProvisionerParams() *ProvisionerParams {
	return &ProvisionerParams{
		name:              defaultName,
		agentOptions:      []agentparams.Option{},
		fakeintakeOptions: []fakeintake.Option{},
		extraConfigParams: runner.ConfigMap{},
	}
}

// ProvisionerOption is a function that modifies the ProvisionerParams
type ProvisionerOption func(*ProvisionerParams) error

// WithName sets the name of the provisioner
func WithName(name string) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.name = name
		return nil
	}
}

// WithAgentOptions adds options to the agent
func WithAgentOptions(opts ...agentparams.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = opts
		return nil
	}
}

// WithoutFakeIntake removes the fake intake
func WithoutFakeIntake() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = nil
		return nil
	}
}

// WithoutAgent removes the agent
func WithoutAgent() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = nil
		return nil
	}
}

// WithExtraConfigParams adds extra config parameters to the environment
func WithExtraConfigParams(configMap runner.ConfigMap) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.extraConfigParams = configMap
		return nil
	}
}

// PodmanProvisionerNoAgentNoFakeIntake wraps Provisioner with hardcoded WithoutAgent and WithoutFakeIntake options.
func PodmanProvisionerNoAgentNoFakeIntake(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Host] {
	mergedOpts := make([]ProvisionerOption, 0, len(opts)+2)
	mergedOpts = append(mergedOpts, opts...)
	mergedOpts = append(mergedOpts, WithoutAgent(), WithoutFakeIntake())

	return PodmanProvisioner(mergedOpts...)
}

// PodmanProvisionerNoFakeIntake wraps Provisioner with hardcoded WithoutFakeIntake option.
func PodmanProvisionerNoFakeIntake(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Host] {
	mergedOpts := make([]ProvisionerOption, 0, len(opts)+1)
	mergedOpts = append(mergedOpts, opts...)
	mergedOpts = append(mergedOpts, WithoutFakeIntake())

	return PodmanProvisioner(mergedOpts...)
}

// PodmanProvisioner creates a new provisioner for a local Podman-based host environment.
//
// Agent installation is performed via SSH after Pulumi provisions the host and
// FakeIntake (PostProvision), rather than inside Pulumi itself.
func PodmanProvisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Host] {
	params := newProvisionerParams()
	_ = optional.ApplyOptions(params, opts)

	// Capture user-provided agent options before the closure so PostProvision
	// receives clean options (before Pulumi would add the fakeintake resource).
	agentOpts := params.agentOptions
	usePostProvision := agentOpts != nil

	pulumiProv := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.Host) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := newProvisionerParams()
		_ = optional.ApplyOptions(params, opts)
		if usePostProvision {
			params.agentOptions = nil
		}
		return PodmanRunFunc(ctx, env, params)
	}, params.extraConfigParams)

	if !usePostProvision {
		return pulumiProv
	}

	return provisioners.WithPostProvision(pulumiProv, func(t *testing.T, env *environments.Host) {
		hostagent.Install(installers.FromT(t), env, agentOpts...)
	})
}

// PodmanRunFunc is the Pulumi run function that runs the provisioner.
// Agent installation is handled by PodmanProvisioner's PostProvision step when
// agentOptions are provided; this function only provisions the VM and FakeIntake.
func PodmanRunFunc(ctx *pulumi.Context, env *environments.Host, params *ProvisionerParams) error {
	localEnv, err := local.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	vm, err := localpodman.NewVM(localEnv, params.name)
	if err != nil {
		return err
	}

	err = vm.Export(ctx, &env.RemoteHost.HostOutput)
	if err != nil {
		return err
	}

	if params.fakeintakeOptions != nil {
		fakeIntake, err := fakeintakeComp.NewLocalDockerFakeintake(&localEnv, "fakeintake")
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

	// Agent installation is handled by PostProvision; always nil here.
	env.Agent = nil

	// explicit set the updater as nil as we do not use it
	env.Updater = nil

	return nil
}

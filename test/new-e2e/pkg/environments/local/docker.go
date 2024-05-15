// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package local

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/updater"
	"github.com/DataDog/test-infra-definitions/resources/local/docker"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"
	dclocal "github.com/DataDog/test-infra-definitions/scenarios/local/docker"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	provisionerBaseID = "local-docker-"
	defaultVMName     = "vm"
)

// ProvisionerParams is a set of parameters for the Provisioner.
type ProvisionerParams struct {
	name string

	instanceOptions    []ec2.VMOption
	agentOptions       []agentparams.Option
	agentClientOptions []agentclientparams.Option
	fakeintakeOptions  []fakeintake.Option
	extraConfigParams  runner.ConfigMap
	installDocker      bool
	installUpdater     bool
}

func newProvisionerParams() *ProvisionerParams {
	// We use nil arrays to decide if we should create or not
	return &ProvisionerParams{
		name:               defaultVMName,
		instanceOptions:    []ec2.VMOption{},
		agentOptions:       []agentparams.Option{},
		agentClientOptions: []agentclientparams.Option{},
		fakeintakeOptions:  []fakeintake.Option{},
		extraConfigParams:  runner.ConfigMap{},
	}
}

// WithAgentOptions adds options to the Agent.
func WithAgentOptions(opts ...agentparams.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = append(params.agentOptions, opts...)
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

// WithoutAgent disables the creation of the Agent.
func WithoutAgent() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = nil
		return nil
	}
}

// WithUpdater installs the agent through the updater.
func WithUpdater() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.installUpdater = true
		return nil
	}
}

// WithDocker installs docker on the VM
func WithDocker() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.installDocker = true
		return nil
	}
}

// GetProvisionerParams return ProvisionerParams from options opts setup
func GetProvisionerParams(opts ...ProvisionerOption) *ProvisionerParams {
	params := newProvisionerParams()
	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(fmt.Errorf("unable to apply ProvisionerOption, err: %w", err))
	}
	return params
}

// ProvisionerOption is a provisioner option.
type ProvisionerOption func(*ProvisionerParams) error

func Provisioner(opts ...ProvisionerOption) e2e.TypedProvisioner[environments.DockerLocal] {
	// We need to build params here to be able to use params.name in the provisioner name
	params := GetProvisionerParams(opts...)

	provisioner := e2e.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.DockerLocal) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := GetProvisionerParams(opts...)
		return Run(ctx, env, params)
	}, params.extraConfigParams)

	return provisioner
}

// Run deploys a environment given a pulumi.Context
func Run(ctx *pulumi.Context, env *environments.DockerLocal, params *ProvisionerParams) (err error) {
	var localEnv docker.Environment
	if env.Local != nil {
		localEnv = *env.Local
	} else {
		localEnv, err = docker.NewEnvironment(ctx)
		if err != nil {
			return err
		}
	}
	host, err := dclocal.NewVM(localEnv, params.name)
	if err != nil {
		return err
	}
	err = host.Export(ctx, &env.RemoteHost.HostOutput)
	if err != nil {
		return err
	}
	_ = ctx.Log.Info(fmt.Sprintf("Running test on container '%v'", host.Name()), nil)

	// TODO: Create FakeIntake if required

	if !params.installUpdater {
		// Suite inits all fields by default, so we need to explicitly set it to nil
		env.Updater = nil
	}
	// Create Agent if required
	if params.installUpdater && params.agentOptions != nil {
		hupdater, err := updater.NewHostUpdater(&localEnv, host, params.agentOptions...)
		if err != nil {
			return err
		}

		err = hupdater.Export(ctx, &env.Updater.HostUpdaterOutput)
		if err != nil {
			return err
		}
		// todo: add agent once updater installs agent on bootstrap
		env.Agent = nil
	} else if params.agentOptions != nil {
		hagent, err := agent.NewHostAgent(&localEnv, host, params.agentOptions...)
		if err != nil {
			return err
		}

		err = hagent.Export(ctx, &env.Agent.HostAgentOutput)
		if err != nil {
			return err
		}

		env.Agent.ClientOptions = params.agentClientOptions
	} else {
		env.Agent = nil
	}

	return nil
}

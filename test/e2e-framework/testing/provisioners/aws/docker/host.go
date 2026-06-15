// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package awsdocker contains the definition of the AWS Docker environment.
package awsdocker

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/dockeragent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	provisionerBaseID = "aws-ec2docker-"
	defaultVMName     = "dockervm"
)

type ProvisionerParams struct {
	awsEnv            *aws.Environment
	extraConfigParams runner.ConfigMap
	runOptions        []ec2docker.Option
}

type ProvisionerOption func(*ProvisionerParams) error

func getProvisionerParams(opts ...ProvisionerOption) *ProvisionerParams {
	params := &ProvisionerParams{
		awsEnv:            nil,
		runOptions:        nil,
		extraConfigParams: runner.ConfigMap{},
	}

	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(fmt.Errorf("unable to apply ProvisionerOption, err: %w", err))
	}
	return params
}

func WithEnv(env *aws.Environment) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.awsEnv = env
		return nil
	}
}

func WithExtraConfigParams(configMap runner.ConfigMap) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.extraConfigParams = configMap
		return nil
	}
}

func WithRunOptions(runOptions ...ec2docker.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.runOptions = append(params.runOptions, runOptions...)
		return nil
	}
}

// Provisioner creates a VM environment with an EC2 VM with Docker, an ECS
// Fargate FakeIntake and a Docker Agent configured to talk to each other.
//
// Agent installation is performed by writing a docker-compose file to the VM
// and running it via SSH (PostProvision), after Pulumi provisions the VM,
// Docker daemon, and FakeIntake. FakeIntake and Agent creation can be
// deactivated by using [WithoutFakeIntake] and [WithoutAgent] options.
//
// Note: dockeragentparams options that use Pulumi-typed values
// (WithAgentServiceEnvVariable, WithExtraComposeManifest, etc.) are not yet
// supported in the PostProvision installer path; those tests must be migrated
// to dockeragent.WithEnvVar / equivalent raw options.
func Provisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.DockerHost] {
	params := getProvisionerParams(opts...)
	ec2Params := ec2docker.GetParams(params.runOptions...)

	// Extract non-Pulumi installer options from the ec2docker agent params.
	installerOpts := buildInstallerOpts(ec2Params)
	usePostProvision := ec2Params.AgentOptions() != nil

	pulumiProv := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+ec2Params.Name, func(ctx *pulumi.Context, env *environments.DockerHost) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := getProvisionerParams(opts...)
		runOpts := params.runOptions
		if usePostProvision {
			runOpts = append(runOpts, ec2docker.WithoutAgent())
		}
		runParams := ec2docker.GetParams(runOpts...)

		var awsEnv aws.Environment
		var err error
		if params.awsEnv != nil {
			awsEnv = *params.awsEnv
		} else {
			awsEnv, err = aws.NewEnvironment(ctx)
			if err != nil {
				return err
			}
			params.awsEnv = &awsEnv
		}

		return ec2docker.Run(ctx, awsEnv, env, runParams)
	}, params.extraConfigParams)

	if !usePostProvision {
		return pulumiProv
	}

	return provisioners.WithPostProvision(pulumiProv, func(t *testing.T, env *environments.DockerHost) {
		dockeragent.Install(installers.FromT(t), env, installerOpts...)
	})
}

// buildInstallerOpts extracts non-Pulumi agent options from ec2docker params
// and maps them to dockeragent installer options. Pulumi-typed options
// (WithAgentServiceEnvVariable, WithExtraComposeManifest, etc.) are silently
// ignored — those tests must migrate to dockeragent.WithEnvVar et al.
func buildInstallerOpts(params *ec2docker.Params) []dockeragent.Option {
	agentOpts := params.AgentOptions()
	if agentOpts == nil {
		return nil
	}

	// Apply options to a zero-value Params to extract the concrete (non-Pulumi)
	// fields. Pulumi-typed fields are set but we simply don't read them.
	p := &dockeragentparams.Params{
		AgentServiceEnvironment: pulumi.Map{},
		EnvironmentVariables:    pulumi.StringMap{},
		ExtraComposeManifests:   []docker.ComposeInlineManifest{},
	}
	for _, opt := range agentOpts {
		_ = opt(p) // errors are non-fatal; we only need the concrete fields
	}

	var out []dockeragent.Option
	if p.FullImagePath != "" {
		out = append(out, dockeragent.WithFullImagePath(p.FullImagePath))
	}
	if p.ImageTag != "" {
		out = append(out, dockeragent.WithImageTag(p.ImageTag))
	}
	if p.Repository != "" {
		out = append(out, dockeragent.WithRepository(p.Repository))
	}
	if p.FIPS {
		out = append(out, dockeragent.WithFIPS())
	}
	if p.JMX {
		out = append(out, dockeragent.WithJMX())
	}
	return out
}

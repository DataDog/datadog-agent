// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ecs contains the definition of the AWS ECS environment.
package ecs

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	scenecs "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/ecsagent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	provisionerBaseID = "aws-ecs-"
	defaultECS        = "ecs"
)

// ProvisionerParams contains all the parameters needed to create the environment
type ProvisionerParams struct {
	name              string
	runOptions        []scenecs.RunOption
	extraConfigParams runner.ConfigMap
	awsEnv            *aws.Environment
}

func newProvisionerParams() *ProvisionerParams {
	// We use nil arrays to decide if we should create or not
	return &ProvisionerParams{
		name:              defaultECS,
		runOptions:        []scenecs.RunOption{},
		extraConfigParams: runner.ConfigMap{},
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

// ProvisionerOption is a function that modifies the ProvisionerParams
type ProvisionerOption func(*ProvisionerParams) error

// WithAwsEnv asks the provisioner to use the given environment, it is created otherwise
func WithAwsEnv(env *aws.Environment) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.awsEnv = env
		return nil
	}
}

// WithRunOptions adds options to the ECS scenario
func WithRunOptions(opts ...scenecs.RunOption) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.runOptions = append(params.runOptions, opts...)
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

// Provisioner creates an ECS environment with the Datadog Agent installed as a
// Daemon service.
//
// Agent installation is performed via the AWS SDK after Pulumi provisions the
// ECS cluster and FakeIntake (PostProvision). Pulumi provisions only the ECS
// cluster, capacity providers, FakeIntake, and workloads — NOT the agent.
func Provisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.ECS] {
	params := GetProvisionerParams(opts...)
	runParams := scenecs.GetRunParams(params.runOptions...)
	agentOpts := runParams.AgentOptions()
	usePostProvision := agentOpts != nil

	pulumiProv := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.ECS) error {
		// deep copy on each invocation
		params := GetProvisionerParams(opts...)
		runOpts := params.runOptions
		if usePostProvision {
			// Tell Pulumi to skip agent install — PostProvision handles it.
			runOpts = append(runOpts, scenecs.WithoutAgent())
		}
		runParams := scenecs.GetRunParams(runOpts...)

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

		return scenecs.RunWithEnv(ctx, awsEnv, env, runParams)
	}, params.extraConfigParams)

	if !usePostProvision {
		return pulumiProv
	}

	return provisioners.WithPostProvision(pulumiProv, func(t *testing.T, env *environments.ECS) {
		ecsagent.Install(t, env, agentOpts...)
	})
}

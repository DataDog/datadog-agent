// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package winawshost contains the definition of the AWS Windows Host environment.
package winawshost

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2/windows"
	scenwin "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2/windows"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

const (
	provisionerBaseID = "aws-ec2vm-"
	defaultVMName     = "vm"
)

// ProvisionerParams is a set of parameters for the Provisioner.
type ProvisionerParams struct {
	extraConfigParams runner.ConfigMap
	runOptions        []windows.RunOption
}

// ProvisionerOption is a provisioner option.
type ProvisionerOption func(*ProvisionerParams) error

func WithRunOptions(opts ...windows.RunOption) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.runOptions = append(params.runOptions, opts...)
		return nil
	}
}

func WithExtraConfigParams(configMap runner.ConfigMap) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.extraConfigParams = configMap
		return nil
	}
}

// GetProvisionerParams return ProvisionerParams from options opts setup
func GetProvisionerParams(opts ...ProvisionerOption) *ProvisionerParams {
	params := &ProvisionerParams{
		runOptions:        []windows.RunOption{},
		extraConfigParams: runner.ConfigMap{},
	}

	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(err)
	}
	return params
}

// Provisioner creates a VM environment with a Windows EC2 VM, an ECS Fargate FakeIntake and a Host Agent configured to talk to each other.
// FakeIntake and Agent creation can be deactivated by using [WithoutFakeIntake] and [WithoutAgent] options.
func Provisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.WindowsHost] {
	// We need to build params here to be able to use params.name in the provisioner name
	params := GetProvisionerParams(opts...)
	runParams := scenwin.GetRunParams(params.runOptions...)

	provisioner := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+runParams.Name, func(ctx *pulumi.Context, env *environments.WindowsHost) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := GetProvisionerParams(opts...)
		runParams := scenwin.GetRunParams(params.runOptions...)

		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}
		return scenwin.RunWithEnv(ctx, awsEnv, env, runParams)

	}, nil)

	return provisioner
}

// ProvisionerNoAgent wraps Provisioner with hardcoded WithoutAgent options.
func ProvisionerNoAgent(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.WindowsHost] {
	mergedOpts := make([]ProvisionerOption, 0, len(opts)+1)
	mergedOpts = append(mergedOpts, opts...)
	mergedOpts = append(mergedOpts, WithRunOptions(windows.WithoutAgent()))

	return Provisioner(mergedOpts...)
}

// ProvisionerNoAgentNoFakeIntake wraps Provisioner with hardcoded WithoutAgent and WithoutFakeIntake options.
func ProvisionerNoAgentNoFakeIntake(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.WindowsHost] {
	mergedOpts := make([]ProvisionerOption, 0, len(opts)+2)
	mergedOpts = append(mergedOpts, opts...)
	mergedOpts = append(mergedOpts, WithRunOptions(windows.WithoutAgent(), windows.WithoutFakeIntake()))

	return Provisioner(mergedOpts...)
}

// ProvisionerNoFakeIntake wraps Provisioner with hardcoded WithoutFakeIntake option.
func ProvisionerNoFakeIntake(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.WindowsHost] {
	mergedOpts := make([]ProvisionerOption, 0, len(opts)+1)
	mergedOpts = append(mergedOpts, opts...)
	mergedOpts = append(mergedOpts, WithRunOptions(windows.WithoutFakeIntake()))

	return Provisioner(mergedOpts...)
}

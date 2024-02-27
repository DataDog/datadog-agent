// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package iis

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	provisionerBaseID = "aws-ec2vm-iis-"
	defaultVMName     = "iisvm"
)

// GetProvisionerParams return ProvisionerParams from options opts setup
func GetProvisionerParams(opts ...ProvisionerOption) *ProvisionerParams {
	params := newProvisionerParams()
	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(fmt.Errorf("unable to apply ProvisionerOption, err: %w", err))
	}
	return params
}

// Run deploys a environment given a pulumi.Context
func Run(ctx *pulumi.Context, env *Env, params *ProvisionerParams) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env.Environment = awsEnv.CommonEnvironment

	// JL: should the ec2 VM be customizable by the user?
	vm, err := ec2.NewVM(awsEnv, defaultVMName, ec2.WithOS(os.WindowsDefault))
	if err != nil {
		return err
	}
	err = vm.Export(ctx, &env.IISHost.HostOutput)
	if err != nil {
		return err
	}

	iisServer, err := NewIISServer(ctx, awsEnv.CommonEnvironment, vm, params.iisOptions...)
	if err != nil {
		return err
	}
	err = iisServer.Export(ctx, &env.IIS.Output)
	if err != nil {
		return err
	}

	return nil
}

// Provisioner creates an Active Directory environment on a given VM.
func Provisioner(opts ...ProvisionerOption) e2e.TypedProvisioner[Env] {

	provisioner := e2e.NewTypedPulumiProvisioner[Env](provisionerBaseID+defaultVMName, func(ctx *pulumi.Context, env *Env) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := GetProvisionerParams(opts...)
		return Run(ctx, env, params)
	}, nil)

	return provisioner
}

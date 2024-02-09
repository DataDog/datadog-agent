// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package activedirectory

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	provisionerBaseID = "aws-ec2vm-active-directory-"
	defaultVMName     = "dcvm"
)

// Provisioner creates an Active Directory environment on a given VM.
func Provisioner(opts ...ProvisionerOption) e2e.TypedProvisioner[Env] {
	params := newProvisionerParams()
	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(fmt.Errorf("unable to apply ProvisionerOption, err: %w", err))
	}

	return e2e.NewTypedPulumiProvisioner[Env](provisionerBaseID+params.name, func(ctx *pulumi.Context, env *Env) error {
		params := newProvisionerParams()
		_ = optional.ApplyOptions(params, opts)

		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		env.Environment = awsEnv.CommonEnvironment

		// JL: should the ec2 VM be customizable by the user?
		vm, err := ec2.NewVM(awsEnv, params.name, ec2.WithOS(os.WindowsDefault))
		if err != nil {
			return err
		}
		vm.Export(ctx, &env.DomainControllerHost.HostOutput)

		domainController, err := NewActiveDirectory(ctx, awsEnv.CommonEnvironment, vm, params.activeDirectoryOptions...)
		if err != nil {
			return err
		}
		domainController.Export(ctx, &env.DomainController.Output)

		// JL: can params.fakeintakeOptions be nil, and how should we handle it?
		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, params.name, params.fakeintakeOptions...)
		if err != nil {
			return err
		}
		fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)

		return nil
	}, nil)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package awshost contains the definition of the AWS Host environment.
package awshost

import (
	"context"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ec2/pool"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/infra"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// defaultPoolRegion is the fallback region used by the pool pre-Up hook when
// the stack's "aws:region" config isn't set explicitly. Every environmentDefault
// entry in resources/aws currently defaults to this region, but that mapping
// is duplicated here rather than read from aws.Environment.Region() because
// the hook runs before a *pulumi.Context (and therefore an aws.Environment)
// exists -- see Provisioner's pre-Up hook for the full gap this leaves.
const defaultPoolRegion = "us-east-1"

const provisionerBaseID = "aws-ec2vm-"

type ProvisionerParams struct {
	awsEnv            *aws.Environment
	extraConfigParams runner.ConfigMap
	runOptions        []ec2.Option
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

func WithRunOptions(runOptions ...ec2.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.runOptions = append(params.runOptions, runOptions...)
		return nil
	}
}

// Provisioner creates a VM environment with an EC2 VM, an ECS Fargate FakeIntake and a Host Agent configured to talk to each other.
// FakeIntake and Agent creation can be deactivated by using [WithoutFakeIntake] and [WithoutAgent] options.
func Provisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Host] {
	// Build provisioner parameters and initial run params for naming
	params := getProvisionerParams(opts...)
	runParams := ec2.GetParams(params.runOptions...)

	// preAcquired is set by the pre-Up hook below (run by getStack before
	// stack.Up, i.e. before the Pulumi stack lock is taken) and read by the
	// RunFunc closure (run during stack.Up, strictly after the hook returns).
	// Scoped to this Provisioner() call, so it's not shared across unrelated
	// provisioners/stacks.
	var preAcquired *ec2.PreAcquiredPoolResult

	provisioner := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+runParams.Name, func(ctx *pulumi.Context, env *environments.Host) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := getProvisionerParams(opts...)
		runOptions := params.runOptions
		if preAcquired != nil {
			runOptions = append(runOptions, ec2.WithEC2InstanceOptions(ec2.WithPreAcquiredPoolResult(preAcquired)))
		}
		runParams := ec2.GetParams(runOptions...)

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

		return ec2.Run(ctx, awsEnv, env, runParams)
	}, params.extraConfigParams)

	provisioner.SetGetStackOptions(infra.WithPreUpHook(newMacOSPoolPreUpHook(params.runOptions, &preAcquired)))

	return provisioner
}

// newMacOSPoolPreUpHook returns an infra.PreUpHookFunc that claims a macOS
// pool member -- via pool.Acquire -- before stack.Up runs, i.e. before the
// Pulumi stack lock is taken. It's a no-op (nil cleanup, nil error) for any
// VM that isn't a macOS pool candidate, so non-macOS/non-pool provisioning is
// unaffected. On success, *preAcquired is set so the RunFunc closure in
// Provisioner reuses this claim instead of calling pool.Acquire itself.
//
// region/profile come from the persisted "aws:region"/"aws:profile" stack
// config (read back by getStack). Most scenarios never set those explicitly --
// they rely on aws.Environment.Region()/Profile()'s per-environment defaults,
// resolved from a *pulumi.Context that doesn't exist yet at this point -- so
// this hook falls back to defaultPoolRegion and $AWS_PROFILE respectively.
// That matches Environment.Region()'s current defaults (every environmentDefault
// entry defaults to defaultPoolRegion) and the first part of Environment.Profile()'s
// precedence, but NOT its final fallback to a per-environment default profile
// when neither $AWS_PROFILE nor static AWS credentials are set: in that one
// case this hook and aws.Environment could resolve to different profiles.
func newMacOSPoolPreUpHook(runOptions []ec2.Option, preAcquired **ec2.PreAcquiredPoolResult) infra.PreUpHookFunc {
	return func(ctx context.Context, region, profile, pipelineID string) (func(error), error) {
		isPoolCandidate, err := ec2.GetParams(runOptions...).IsMacOSPoolCandidate()
		if err != nil {
			return nil, err
		}
		if !isPoolCandidate {
			return nil, nil
		}

		if region == "" {
			region = defaultPoolRegion
		}
		if profile == "" {
			profile = os.Getenv("AWS_PROFILE")
		}

		poolClient, err := pool.NewEC2Client(ctx, region, profile)
		if err != nil {
			return nil, err
		}
		result, err := pool.Acquire(ctx, region, profile, poolClient, pipelineID)
		if err != nil {
			return nil, err
		}

		claimed := false
		*preAcquired = &ec2.PreAcquiredPoolResult{Result: result, Claimed: &claimed}

		return func(_ error) {
			// If NewVM never got as far as scheduling the release-on-destroy
			// command (e.g. Up failed before then), ownership of the lease
			// was never transferred to it, so release it here instead.
			if !claimed {
				if releaseErr := pool.ReleaseInstance(context.Background(), region, profile, result.InstanceID, result.LeaseToken); releaseErr != nil {
					fmt.Printf("Failed to release pre-acquired macOS pool instance %s: %v\n", result.InstanceID, releaseErr)
				}
			}
		}, nil
	}
}

func ProvisionerNoFakeIntake(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Host] {
	opts = append(opts, WithRunOptions(ec2.WithoutFakeIntake()))
	return Provisioner(opts...)
}

func ProvisionerNoAgentNoFakeIntake(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Host] {
	opts = append(opts, WithRunOptions(ec2.WithoutAgent(), ec2.WithoutFakeIntake()))
	return Provisioner(opts...)
}

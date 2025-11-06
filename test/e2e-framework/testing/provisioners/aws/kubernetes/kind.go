// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package awskubernetes contains the provisioner for the Kubernetes based environments
package awskubernetes

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	provisionerBaseID = "aws-kind-"
	defaultVMName     = "kind"
)

//go:embed agent_helm_values.yaml
var agentHelmValues string

// KindDiagnoseFunc is the diagnose function for the Kind provisioner
func KindDiagnoseFunc(ctx context.Context, stackName string) (string, error) {
	dumpResult, err := dumpKindClusterState(ctx, stackName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Dumping Kind cluster state:\n%s", dumpResult), nil
}

// KindProvisioner creates a new provisioner
// Kind provisioner local params/options mapping to scenario params
type kindProvisionerParams struct {
	awsEnv            *aws.Environment
	runOptions        []kindvm.RunOption
	extraConfigParams runner.ConfigMap
}

type kindProvisionerOption func(*kindProvisionerParams) error

func getKindProvisionerParams(opts ...kindProvisionerOption) *kindProvisionerParams {
	p := &kindProvisionerParams{awsEnv: nil, runOptions: []kindvm.RunOption{}, extraConfigParams: runner.ConfigMap{}}
	_ = optional.ApplyOptions(p, opts)
	return p
}

func WithKindAwsEnv(env *aws.Environment) kindProvisionerOption {
	return func(p *kindProvisionerParams) error { p.awsEnv = env; return nil }
}
func WithKindRunOptions(opts ...kindvm.RunOption) kindProvisionerOption {
	return func(p *kindProvisionerParams) error { p.runOptions = append(p.runOptions, opts...); return nil }
}
func WithKindExtraConfigParams(cm runner.ConfigMap) kindProvisionerOption {
	return func(p *kindProvisionerParams) error { p.extraConfigParams = cm; return nil }
}

func KindProvisioner(opts ...kindProvisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	params := getKindProvisionerParams(opts...)
	runParams := kindvm.GetRunParams(params.runOptions...)

	provisioner := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+runParams.Name, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		params := getKindProvisionerParams(opts...)
		runParams := kindvm.GetRunParams(params.runOptions...)

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

		return kindvm.RunWithEnv(ctx, awsEnv, env, runParams)
	}, params.extraConfigParams)

	provisioner.SetDiagnoseFunc(KindDiagnoseFunc)
	return provisioner
}

// KindRunFunc is the Pulumi run function that runs the provisioner
// KindRunFunc has been replaced by scenarios/aws/kindvm.RunWithEnv

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package awskubernetes contains the provisioner for the Kubernetes based environments
package eks

import (
	"context"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/helmagent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awskubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	provisionerBaseID = "aws-eks-"
)

func eksDiagnoseFunc(ctx context.Context, stackName string) (string, error) {
	dumpResult, err := awskubernetes.DumpEKSClusterState(ctx, stackName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Dumping EKS cluster state:\n%s", dumpResult), nil
}

// Provisioner creates a new EKS provisioner.
//
// Agent installation is performed via Helm after Pulumi provisions the EKS
// cluster and FakeIntake (PostProvision), rather than inside Pulumi itself.
// The operator install path (WithOperator) is not yet handled by PostProvision
// and continues to use the Pulumi path.
func Provisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	params := getProvisionerParams(opts...)
	runParams := eks.GetRunParams(params.runOptions...)

	// Capture user-provided agent options outside the closure and build the
	// complete set of conditional options (Windows/GPU nodes) that would
	// otherwise be added inside eks.RunWithEnv.
	agentOpts := buildAgentOpts(runParams)
	usePostProvision := agentOpts != nil && !runParams.DeployOperator()

	pulumiProv := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+runParams.Name, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := getProvisionerParams(opts...)
		runOpts := params.runOptions
		if usePostProvision {
			runOpts = append(runOpts, eks.WithoutAgent())
		}
		runParams := eks.GetRunParams(runOpts...)

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

		return eks.RunWithEnv(ctx, awsEnv, env, runParams)
	}, params.extraConfigParams)

	pulumiProv.SetDiagnoseFunc(eksDiagnoseFunc)

	if !usePostProvision {
		return pulumiProv
	}

	return provisioners.WithPostProvision(pulumiProv, func(t *testing.T, env *environments.Kubernetes) {
		helmagent.Install(t, env, runner.CloudAWS, agentOpts...)
	})
}

// buildAgentOpts assembles the complete set of Pulumi-free agent options for
// PostProvision, mirroring the conditional logic in eks.RunWithEnv (Windows
// nodes, GPU nodes) without the Pulumi-dependent additions (fakeintake
// resource, PulumiResourceOptions, Tags).
func buildAgentOpts(runParams *eks.RunParams) []kubernetesagentparams.Option {
	base := runParams.AgentOptions()
	if base == nil {
		return nil
	}

	opts := make([]kubernetesagentparams.Option, len(base))
	copy(opts, base)

	eksParams, err := eks.NewParams(runParams.EksOptions()...)
	if err == nil {
		if eksParams.WindowsNodeGroup {
			opts = append(opts, kubernetesagentparams.WithDeployWindows())
		}
		if eksParams.GPUNodeGroup {
			opts = append(opts, kubernetesagentparams.WithGPUMonitoring())
		}
	}
	return opts
}

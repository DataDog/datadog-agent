// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package kindvm contains the provisioner for the Kind-on-VM Kubernetes based environments
package kindvm

import (
	"context"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/helmagent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/workloads"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awskubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	provisionerBaseID = "aws-kind-"

	// kindDefaultHelmValues are the KinD-specific Helm overrides applied
	// automatically by PostProvision: kubelet TLS skip, CSI driver, host network.
	kindDefaultHelmValues = `
datadog:
  kubelet:
    tlsVerify: false
  csi:
    enabled: true
agents:
  useHostNetwork: true
`
)

// DiagnoseFunc is the diagnose function for the Kind provisioner
func DiagnoseFunc(ctx context.Context, stackName string) (string, error) {
	dumpResult, err := awskubernetes.DumpKindClusterState(ctx, stackName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Dumping Kind cluster state:\n%s", dumpResult), nil
}

// kindProvisionerWrapper wraps a Pulumi provisioner and adds Helm-based agent
// installation and workload deployment via PostProvision. BaseSuite.SetupSuite
// calls PostProvision after Pulumi finishes, so test SetupSuites only need
// test-specific setup beyond the standard workloads.
type kindProvisionerWrapper struct {
	provisioners.TypedProvisioner[environments.Kubernetes]
	agentOpts    []kubernetesagentparams.Option
	workloadOpts []workloads.Option
}

// Diagnose forwards to the underlying Pulumi provisioner's Diagnose if it
// implements provisioners.Diagnosable, so provisioning failures are still diagnosed.
func (w *kindProvisionerWrapper) Diagnose(ctx context.Context, stackName string) (string, error) {
	if d, ok := w.TypedProvisioner.(provisioners.Diagnosable); ok {
		return d.Diagnose(ctx, stackName)
	}
	return "", nil
}

// PostProvision installs the Datadog agent via Helm and deploys any configured
// workloads after the KinD cluster is ready. KinD-specific defaults (kubelet
// TLS skip, CSI driver, host network, stackid tag) are prepended automatically.
func (w *kindProvisionerWrapper) PostProvision(t *testing.T, env *environments.Kubernetes) {
	opts := []kubernetesagentparams.Option{
		kubernetesagentparams.WithHelmValues(kindDefaultHelmValues),
		kubernetesagentparams.WithTags([]string{"stackid:" + env.KubernetesCluster.ClusterName}),
	}
	opts = append(opts, w.agentOpts...)
	helmagent.Install(t, env, opts...)

	if len(w.workloadOpts) > 0 {
		workloads.Deploy(t, env, w.workloadOpts...)
	}
}

// Provisioner returns a provisioner for a KinD-on-EC2 Kubernetes environment.
// The provisioner always installs the Datadog agent via Helm in its PostProvision
// step — never via Pulumi. Pass WithAgentOptions to supply test-specific Helm
// values on top of the KinD defaults; pass WithWorkloads to deploy test workloads
// after agent installation.
func Provisioner(opts ...provisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	params := getProvisionerParams(opts...)
	runParams := kindvm.GetRunParams(params.runOptions...)

	pulumiProvisioner := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+runParams.Name, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		params := getProvisionerParams(opts...)
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

	pulumiProvisioner.SetDiagnoseFunc(DiagnoseFunc)

	return &kindProvisionerWrapper{
		TypedProvisioner: pulumiProvisioner,
		agentOpts:        params.agentOpts,
		workloadOpts:     params.workloadOpts,
	}
}

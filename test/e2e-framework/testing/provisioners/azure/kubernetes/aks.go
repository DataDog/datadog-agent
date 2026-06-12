// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package azurekubernetes contains the provisioner for Azure Kubernetes Service (AKS)
package azurekubernetes

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/azure"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/azure/aks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/azure/fakeintake"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/helmagent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
)

const (
	provisionerBaseID = "azure-aks"
)

// aksDefaultHelmValues are AKS-specific Helm overrides applied automatically
// by PostProvision. On Kata nodes, AKS uses the node-name as the only SAN in
// the Kubelet certificate which is not DNS-resolvable, so we enable the AKS
// provider which sets tlsVerify:false and uses status.hostIP as the Kubelet
// host.
const aksDefaultHelmValues = `
providers:
  aks:
    enabled: true
`

// AKSProvisioner creates a new provisioner for AKS on Azure.
//
// Agent installation is performed via Helm after Pulumi provisions the AKS
// cluster and FakeIntake (PostProvision), rather than inside Pulumi itself.
func AKSProvisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	// Capture user-provided agent options outside the closure so PostProvision
	// receives clean options (before Pulumi would add the fakeintake resource).
	params := newProvisionerParams(opts...)
	agentOpts := params.agentOptions
	usePostProvision := agentOpts != nil

	pulumiProv := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := newProvisionerParams(opts...)
		if usePostProvision {
			params.agentOptions = nil
		}
		return AKSRunFunc(ctx, env, params)
	}, params.extraConfigParams)

	if !usePostProvision {
		return pulumiProv
	}

	// Prepend AKS-specific defaults then user options.
	postProvisionOpts := append(
		[]kubernetesagentparams.Option{kubernetesagentparams.WithHelmValues(aksDefaultHelmValues)},
		agentOpts...,
	)

	return provisioners.WithPostProvision(pulumiProv, func(t *testing.T, env *environments.Kubernetes) {
		helmagent.Install(t, env, runner.CloudAzure, postProvisionOpts...)
	})
}

// AKSRunFunc is the run function for AKS provisioner.
// When agentOptions is nil (PostProvision handles the install), it only
// provisions the cluster and FakeIntake.
func AKSRunFunc(ctx *pulumi.Context, env *environments.Kubernetes, params *ProvisionerParams) error {
	azureEnv, err := azure.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	// Create the AKS cluster
	aksCluster, err := aks.NewAKSCluster(azureEnv, params.aksOptions...)
	if err != nil {
		return err
	}
	err = aksCluster.Export(ctx, &env.KubernetesCluster.ClusterOutput)
	if err != nil {
		return err
	}

	// Deploy a fakeintake
	if params.fakeintakeOptions != nil {
		fakeIntake, err := fakeintake.NewVMInstance(azureEnv, params.fakeintakeOptions...)
		if err != nil {
			return err
		}
		err = fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)
		if err != nil {
			return err
		}
	} else {
		env.FakeIntake = nil
	}

	// Agent installation is handled by PostProvision via helmagent.Install.
	env.Agent = nil

	// Deploy Pulumi-based workloads (workloadAppFuncs use the Pulumi k8s provider).
	for _, appFunc := range params.workloadAppFuncs {
		_, err := appFunc(&azureEnv, aksCluster.KubeProvider)
		if err != nil {
			return err
		}
	}
	return nil
}

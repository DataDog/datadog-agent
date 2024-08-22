// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package azurekubernetes

import (
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent/helm"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	"github.com/DataDog/test-infra-definitions/resources/azure"
	"github.com/DataDog/test-infra-definitions/scenarios/azure/aks"
	"github.com/DataDog/test-infra-definitions/scenarios/azure/fakeintake"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
)

const (
	provisionerBaseID = "azure-aks"
)

func AKSProvisioner(opts ...ProvisionerOption) e2e.TypedProvisioner[environments.Kubernetes] {
	// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
	// and it's easy to forget about it, leading to hard to debug issues.
	params := newProvisionerParams()
	_ = optional.ApplyOptions(params, opts)

	//TODO: Remove when https://datadoghq.atlassian.net/browse/ADXT-479 is done
	fmt.Println("PLEASE DO NOT USE THIS PROVIDER FOR TESTING ON THE CI YET. WE NEED TO FIND A WAY TO CLEAN INSTANCES FIRST")

	provisioner := e2e.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := newProvisionerParams()
		_ = optional.ApplyOptions(params, opts)

		return AKSRunFunc(ctx, env, params)
	}, params.extraConfigParams)

	return provisioner
}

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

	agentOptions := params.agentOptions

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
		agentOptions = append(agentOptions, kubernetesagentparams.WithFakeintake(fakeIntake))

	} else {
		env.FakeIntake = nil
	}

	if params.agentOptions != nil {
		// On Kata nodes, AKS uses the node-name (like aks-kata-21213134-vmss000000) as the only SAN in the Kubelet
		// certificate. However, the DNS name aks-kata-21213134-vmss000000 is not resolvable, so it cannot be used
		// to reach the Kubelet. Thus we need to use `tlsVerify: false` and `and `status.hostIP` as `host` in
		// the Helm values
		customValues := `
datadog:
  kubelet:
    host:
      valueFrom:
        fieldRef:
          fieldPath: status.hostIP
    hostCAPath: /etc/kubernetes/certs/kubeletserver.crt
    tlsVerify: false
providers:
  aks:
    enabled: true
`
		agentOptions = append(agentOptions, kubernetesagentparams.WithHelmValues(customValues))
		agent, err := helm.NewKubernetesAgent(&azureEnv, params.name, aksCluster.KubeProvider, agentOptions...)
		if err != nil {
			return err
		}
		err = agent.Export(ctx, &env.Agent.KubernetesAgentOutput)
		if err != nil {
			return err
		}
	} else {
		env.Agent = nil
	}
	return nil
}

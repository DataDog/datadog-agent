// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aks

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/cpustress"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/dogstatsd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/etcd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/mutatedbyadmissioncontroller"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/prometheus"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redis"
	dogstatsdstandalone "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dogstatsd-standalone"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/azure"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/azure/fakeintake"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const kataRuntimeClass = "kata-mshv-vm-isolation"

func Run(ctx *pulumi.Context) error {
	env, err := azure.NewEnvironment(ctx)
	if err != nil {
		return err
	}
	aksClusterOptions := []Option{}
	if env.LinuxKataNodeGroup() {
		aksClusterOptions = append(aksClusterOptions, WithKataNodePool())
	}

	aksCluster, err := NewAKSCluster(env, aksClusterOptions...)
	if err != nil {
		return err
	}
	err = aksCluster.Export(ctx, nil)
	if err != nil {
		return err
	}

	var dependsOnDDAgent pulumi.ResourceOption

	// Deploy the agent
	if env.AgentDeploy() {
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
		k8sAgentOptions := make([]kubernetesagentparams.Option, 0)
		k8sAgentOptions = append(
			k8sAgentOptions,
			kubernetesagentparams.WithNamespace("datadog"),
			kubernetesagentparams.WithHelmValues(customValues),
		)

		if env.AgentUseFakeintake() {
			fakeIntakeOptions := []fakeintake.Option{}
			if env.AgentUseDualShipping() {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithoutDDDevForwarding())
			}

			if retentionPeriod := env.AgentFakeintakeRetentionPeriod(); retentionPeriod != "" {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithRetentionPeriod(retentionPeriod))
			}

			fakeintake, err := fakeintake.NewVMInstance(env, fakeIntakeOptions...)
			if err != nil {
				return err
			}
			if err := fakeintake.Export(env.Ctx(), nil); err != nil {
				return err
			}

			k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithFakeintake(fakeintake))
		}

		if env.AgentUseDualShipping() {
			k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithDualShipping())
		}

		k8sAgentComponent, err := helm.NewKubernetesAgent(&env, env.Namer.ResourceName("datadog-agent"), aksCluster.KubeProvider, k8sAgentOptions...)

		if err != nil {
			return err
		}

		if err := k8sAgentComponent.Export(env.Ctx(), nil); err != nil {
			return err
		}

		dependsOnDDAgent = utils.PulumiDependsOn(k8sAgentComponent)
	}

	// Deploy standalone dogstatsd
	if env.DogstatsdDeploy() {
		if _, err := dogstatsdstandalone.K8sAppDefinition(&env, aksCluster.KubeProvider, "dogstatsd-standalone", "/run/containerd/containerd.sock", nil, true, ""); err != nil {
			return err
		}
	}

	// Deploy testing workload
	if env.TestingWorkloadDeploy() {
		if _, err := nginx.K8sAppDefinition(&env, aksCluster.KubeProvider, "workload-nginx", 80, "", true, dependsOnDDAgent /* for DDM */); err != nil {
			return err
		}

		if _, err := nginx.K8sAppDefinition(&env, aksCluster.KubeProvider, "workload-nginx-kata", 80, kataRuntimeClass, true, dependsOnDDAgent /* for DDM */); err != nil {
			return err
		}

		if _, err := redis.K8sAppDefinition(&env, aksCluster.KubeProvider, "workload-redis", true, dependsOnDDAgent /* for DDM */); err != nil {
			return err
		}

		if _, err := cpustress.K8sAppDefinition(&env, aksCluster.KubeProvider, "workload-cpustress"); err != nil {
			return err
		}

		// dogstatsd clients that report to the Agent
		if _, err := dogstatsd.K8sAppDefinition(&env, aksCluster.KubeProvider, "workload-dogstatsd", 8125, "/var/run/datadog/dsd.socket", dependsOnDDAgent /* for admission */); err != nil {
			return err
		}

		if env.DogstatsdDeploy() {
			// dogstatsd clients that report to the dogstatsd standalone deployment
			if _, err := dogstatsd.K8sAppDefinition(&env, aksCluster.KubeProvider, "workload-dogstatsd-standalone", dogstatsdstandalone.HostPort, dogstatsdstandalone.Socket, dependsOnDDAgent /* for admission */); err != nil {
				return err
			}
		}

		if _, err := prometheus.K8sAppDefinition(&env, aksCluster.KubeProvider, "workload-prometheus"); err != nil {
			return err
		}

		if _, err := mutatedbyadmissioncontroller.K8sAppDefinition(&env, aksCluster.KubeProvider, "workload-mutated", "workload-mutated-lib-injection", dependsOnDDAgent /* for admission */); err != nil {
			return err
		}

		if _, err := etcd.K8sAppDefinition(&env, aksCluster.KubeProvider); err != nil {
			return err
		}
	}

	if err != nil {
		return err
	}
	return nil
}

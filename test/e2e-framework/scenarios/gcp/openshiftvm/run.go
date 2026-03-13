// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package openshiftvm

import (
	kubernetesNewProvider "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/cpustress"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/dogstatsd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/mutatedbyadmissioncontroller"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/prometheus"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redis"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/tracegen"
	dogstatsdstandalone "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dogstatsd-standalone"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/vpa"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	resGcp "github.com/DataDog/datadog-agent/test/e2e-framework/resources/gcp"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/compute"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/fakeintake"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Run(ctx *pulumi.Context) error {
	gcpEnv, err := resGcp.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	osDesc := os.DescriptorFromString("redhat:9", os.RedHat9)
	vm, err := compute.NewVM(gcpEnv, "openshift",
		compute.WithOS(osDesc),
		compute.WithInstancetype("n2-standard-8"),
		compute.WithNestedVirt(true),
	)
	if err != nil {
		return err
	}
	if err := vm.Export(ctx, nil); err != nil {
		return err
	}

	openshiftCluster, err := kubernetes.NewOpenShiftCluster(&gcpEnv, vm, "openshift", gcpEnv.OpenShiftPullSecretPath())
	if err != nil {
		return err
	}
	if err := openshiftCluster.Export(ctx, nil); err != nil {
		return err
	}

	if gcpEnv.InitOnly() {
		return nil
	}

	// Building Kubernetes provider for OpenShift
	openshiftKubeProvider, err := kubernetesNewProvider.NewProvider(ctx, gcpEnv.Namer.ResourceName("openshift-k8s-provider"), &kubernetesNewProvider.ProviderArgs{
		Kubeconfig:            openshiftCluster.KubeConfig,
		EnableServerSideApply: pulumi.BoolPtr(true),
		DeleteUnreachable:     pulumi.BoolPtr(true),
	})
	if err != nil {
		return err
	}

	vpaCrd, err := vpa.DeployCRD(&gcpEnv, openshiftKubeProvider)
	if err != nil {
		return err
	}
	dependsOnVPA := utils.PulumiDependsOn(vpaCrd)

	var fakeIntake *fakeintakeComp.Fakeintake
	if gcpEnv.AgentUseFakeintake() {
		fakeIntakeOptions := []fakeintake.Option{
			fakeintake.WithMemory(2048),
		}
		if gcpEnv.InfraShouldDeployFakeintakeWithLB() {
			fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithLoadBalancer())
		}
		if gcpEnv.AgentUseDualShipping() {
			fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithoutDDDevForwarding())
		}

		if retentionPeriod := gcpEnv.AgentFakeintakeRetentionPeriod(); retentionPeriod != "" {
			fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithRetentionPeriod(retentionPeriod))
		}

		if fakeIntake, err = fakeintake.NewVMInstance(gcpEnv, fakeIntakeOptions...); err != nil {
			return err
		}

		if err := fakeIntake.Export(gcpEnv.Ctx(), nil); err != nil {
			return err
		}
	}

	var dependsOnDDAgent pulumi.ResourceOption

	// Deploy the agent
	if gcpEnv.AgentDeploy() {
		customValues := `
datadog:
  kubelet:
    tlsVerify: false
  # https://docs.datadoghq.com/containers/troubleshooting/admission-controller/?tab=helm#openshift
  apm:
    portEnabled: true
    socketEnabled: false
agents:
  enabled: true
  tolerations:
    # Deploy Agents on master nodes
    - effect: NoSchedule
      key: node-role.kubernetes.io/master
      operator: Exists
    # Deploy Agents on infra nodes
    - effect: NoSchedule
      key: node-role.kubernetes.io/infra
      operator: Exists
    # Tolerate disk pressure
    - effect: NoSchedule
      key: node.kubernetes.io/disk-pressure
      operator: Exists
  useHostNetwork: true
  replicas: 1
  podSecurity:
    securityContextConstraints:
      create: true
clusterAgent:
  resources:
    limits:
      cpu: 300m
      memory: 400Mi
    requests:
      cpu: 150m
      memory: 300Mi
  enabled: true
  podSecurity:
    securityContextConstraints:
      create: true`

		k8sAgentOptions := make([]kubernetesagentparams.Option, 0)
		k8sAgentOptions = append(
			k8sAgentOptions,
			kubernetesagentparams.WithNamespace("datadog"),
			kubernetesagentparams.WithHelmValues(customValues),
			kubernetesagentparams.WithClusterName(openshiftCluster.ClusterName),
		)
		if fakeIntake != nil {
			k8sAgentOptions = append(
				k8sAgentOptions,
				kubernetesagentparams.WithFakeintake(fakeIntake),
			)
		}

		if gcpEnv.AgentUseDualShipping() {
			k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithDualShipping())
		}

		k8sAgentComponent, err := helm.NewKubernetesAgent(&gcpEnv, gcpEnv.Namer.ResourceName("datadog-agent"), openshiftKubeProvider, k8sAgentOptions...)

		if err != nil {
			return err
		}

		if err := k8sAgentComponent.Export(gcpEnv.Ctx(), nil); err != nil {
			return err
		}

		dependsOnDDAgent = utils.PulumiDependsOn(k8sAgentComponent)
	}

	// Deploy testing workload
	if gcpEnv.TestingWorkloadDeploy() {

		if _, err := redis.K8sAppDefinition(&gcpEnv, openshiftKubeProvider, "workload-redis", true, dependsOnDDAgent /* for DDM */, dependsOnVPA); err != nil {
			return err
		}

		if _, err := prometheus.K8sAppDefinition(&gcpEnv, openshiftKubeProvider, "workload-prometheus"); err != nil {
			return err
		}

		if _, err := cpustress.K8sAppDefinition(&gcpEnv, openshiftKubeProvider, "workload-cpustress"); err != nil {
			return err
		}

		if _, err := tracegen.K8sAppDefinition(&gcpEnv, openshiftKubeProvider, "workload-tracegen"); err != nil {
			return err
		}

		if _, err := nginx.K8sAppDefinition(&gcpEnv, openshiftKubeProvider, "workload-nginx", 8080, "", true, dependsOnDDAgent /* for DDM */, dependsOnVPA); err != nil {
			return err
		}

		if _, err := mutatedbyadmissioncontroller.K8sAppDefinition(&gcpEnv, openshiftKubeProvider, "workload-mutated", "workload-mutated-lib-injection", dependsOnDDAgent /* for admission */); err != nil {
			return err
		}

		if gcpEnv.DogstatsdDeploy() {
			// Standalone dogstatsd
			if _, err := dogstatsdstandalone.K8sAppDefinition(&gcpEnv, openshiftKubeProvider, "dogstatsd-standalone", "/run/crio/crio.sock", fakeIntake, true, ""); err != nil {
				return err
			}

			// Dogstatsd clients that report to the standalone dogstatsd deployment
			if _, err := dogstatsd.K8sAppDefinition(&gcpEnv, openshiftKubeProvider, "workload-dogstatsd-standalone", dogstatsdstandalone.HostPort, "/run/datadog/dsd.socket", dependsOnDDAgent /* for admission */); err != nil {
				return err
			}

			// Dogstatsd clients that report to the Agent
			if _, err := dogstatsd.K8sAppDefinition(&gcpEnv, openshiftKubeProvider, "workload-dogstatsd", 8125, "/var/run/datadog/dsd.socket", dependsOnDDAgent /* for admission */); err != nil {
				return err
			}
		}
	}

	return nil
}

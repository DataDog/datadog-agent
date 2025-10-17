package gke

import (
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent/helm"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/cpustress"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/dogstatsd"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/etcd"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/mutatedbyadmissioncontroller"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/nginx"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/prometheus"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/tracegen"
	dogstatsdstandalone "github.com/DataDog/test-infra-definitions/components/datadog/dogstatsd-standalone"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	"github.com/DataDog/test-infra-definitions/components/kubernetes/vpa"
	"github.com/DataDog/test-infra-definitions/resources/gcp"
	"github.com/DataDog/test-infra-definitions/scenarios/gcp/fakeintake"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Run(ctx *pulumi.Context) error {
	env, err := gcp.NewEnvironment(ctx)
	if err != nil {
		return err
	}
	clusterOptions := []Option{}

	if env.GKEAutopilot() {
		clusterOptions = append(clusterOptions, WithAutopilot())
	}

	cluster, err := NewGKECluster(env, clusterOptions...)
	if err != nil {
		return err
	}
	err = cluster.Export(ctx, nil)
	if err != nil {
		return err
	}

	var dependsOnVPA pulumi.ResourceOption
	if !env.GKEAutopilot() {
		vpaCrd, err := vpa.DeployCRD(&env, cluster.KubeProvider)
		if err != nil {
			return err
		}

		dependsOnVPA = utils.PulumiDependsOn(vpaCrd)
	}

	var dependsOnDDAgent pulumi.ResourceOption

	// Deploy the agent
	if env.AgentDeploy() {
		k8sAgentOptions := make([]kubernetesagentparams.Option, 0)
		k8sAgentOptions = append(
			k8sAgentOptions,
			kubernetesagentparams.WithNamespace("datadog"),
		)

		if env.GKEAutopilot() {
			k8sAgentOptions = append(
				k8sAgentOptions,
				kubernetesagentparams.WithGKEAutopilot(),
			)
		}

		if env.AgentUseDualShipping() {
			k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithDualShipping())
		}

		if env.AgentUseFakeintake() {
			fakeIntakeOptions := []fakeintake.Option{}
			if env.AgentUseDualShipping() {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithoutDDDevForwarding())
			}

			if storeType := env.AgentFakeintakeStoreType(); storeType != "" {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithStoreType(storeType))
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

		k8sAgentComponent, err := helm.NewKubernetesAgent(&env, env.Namer.ResourceName("datadog-agent"), cluster.KubeProvider, k8sAgentOptions...)

		if err != nil {
			return err
		}

		if err := k8sAgentComponent.Export(env.Ctx(), nil); err != nil {
			return err
		}

		dependsOnDDAgent = utils.PulumiDependsOn(k8sAgentComponent)
	}

	// Deploy testing workload
	if env.TestingWorkloadDeploy() {

		if _, err := nginx.K8sAppDefinition(&env, cluster.KubeProvider, "workload-nginx", "", true, dependsOnDDAgent /* for DDM */, dependsOnVPA); err != nil {
			return err
		}

		if _, err := cpustress.K8sAppDefinition(&env, cluster.KubeProvider, "workload-cpustress"); err != nil {
			return err
		}

		if _, err := prometheus.K8sAppDefinition(&env, cluster.KubeProvider, "workload-prometheus"); err != nil {
			return err
		}

		if _, err := mutatedbyadmissioncontroller.K8sAppDefinition(&env, cluster.KubeProvider, "workload-mutated", "workload-mutated-lib-injection", dependsOnDDAgent /* for admission */); err != nil {
			return err
		}

		if _, err := etcd.K8sAppDefinition(&env, cluster.KubeProvider); err != nil {
			return err
		}

		// These workloads cannot be deployed on Autopilot because of the constraints on hostPath volumes
		if !env.GKEAutopilot() {
			// Deploy standalone dogstatsd
			if env.DogstatsdDeploy() {
				if _, err := dogstatsdstandalone.K8sAppDefinition(&env, cluster.KubeProvider, "dogstatsd-standalone", nil, true, ""); err != nil {
					return err
				}

				// dogstatsd clients that report to the dogstatsd standalone deployment
				if _, err := dogstatsd.K8sAppDefinition(&env, cluster.KubeProvider, "workload-dogstatsd-standalone", dogstatsdstandalone.HostPort, dogstatsdstandalone.Socket, dependsOnDDAgent /* for admission */); err != nil {
					return err
				}
			}

			// dogstatsd clients that report to the Agent
			if _, err := dogstatsd.K8sAppDefinition(&env, cluster.KubeProvider, "workload-dogstatsd", 8125, "/var/run/datadog/dsd.socket", dependsOnDDAgent /* for admission */); err != nil {
				return err
			}

			if _, err := tracegen.K8sAppDefinition(&env, cluster.KubeProvider, "workload-tracegen"); err != nil {
				return err
			}
		}
	}

	return nil
}

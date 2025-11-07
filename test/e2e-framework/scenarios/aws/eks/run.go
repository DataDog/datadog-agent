// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package eks

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/cpustress"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/dogstatsd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/etcd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/mutatedbyadmissioncontroller"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/prometheus"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redis"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/tracegen"
	dogstatsdstandalone "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dogstatsd-standalone"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/vpa"
	resourcesAws "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Run(ctx *pulumi.Context) error {
	awsEnv, err := resourcesAws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env, _, _, err := environments.CreateEnv[environments.Kubernetes]()
	if err != nil {
		return err
	}

	params := ParamsFromEnvironment(awsEnv)
	return RunWithEnv(ctx, awsEnv, env, params)
}

// RunWithEnv deploys an EKS environment using a provided env and params, enabling reuse between provisioners and direct Pulumi runs.
func RunWithEnv(ctx *pulumi.Context, awsEnv resourcesAws.Environment, env *environments.Kubernetes, params *RunParams) error {
	cluster, err := NewCluster(awsEnv, "eks", params.eksOptions...)
	if err != nil {
		return err
	}

	if err := cluster.Export(ctx, &env.KubernetesCluster.ClusterOutput); err != nil {
		return err
	}

	vpaCrd, err := vpa.DeployCRD(&awsEnv, cluster.KubeProvider)
	if err != nil {
		return err
	}
	dependsOnVPA := utils.PulumiDependsOn(vpaCrd)

	if awsEnv.InitOnly() {
		return nil
	}

	// Create fakeintake if needed
	var fakeIntake *fakeintakeComp.Fakeintake

	var dependsOnDDAgent pulumi.ResourceOption
	var k8sAgentComponent *agent.KubernetesAgent
	if awsEnv.AgentDeploy() {
		if params.fakeintakeOptions != nil {
			if fakeIntake, err = fakeintake.NewECSFargateInstance(awsEnv, "ecs", params.fakeintakeOptions...); err != nil {
				return err
			}
			if err := fakeIntake.Export(awsEnv.Ctx(), &env.FakeIntake.FakeintakeOutput); err != nil {
				return err
			}
		} else {
			env.FakeIntake = nil
		}

		k8sAgentOptions := make([]kubernetesagentparams.Option, 0)
		k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(cluster)))
		k8sAgentOptions = append(k8sAgentOptions, params.agentOptions...)
		if params.fakeintakeOptions != nil {
			k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithFakeintake(fakeIntake))
		}
		if awsEnv.EKSWindowsNodeGroup() {
			k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithDeployWindows())
		}

		k8sAgentComponent, err = helm.NewKubernetesAgent(&awsEnv, awsEnv.Namer.ResourceName("datadog-agent"), cluster.KubeProvider, k8sAgentOptions...)
		if err != nil {
			return err
		}
		if err := k8sAgentComponent.Export(awsEnv.Ctx(), &env.Agent.KubernetesAgentOutput); err != nil {
			return err
		}
		dependsOnDDAgent = utils.PulumiDependsOn(k8sAgentComponent)

		if params.deployDogstatsd {
			if _, err := dogstatsdstandalone.K8sAppDefinition(&awsEnv, cluster.KubeProvider, "dogstatsd-standalone", fakeIntake, true, ""); err != nil {
				return err
			}
		}

		if params.deployTestWorkload {
			if _, err := nginx.K8sAppDefinition(&awsEnv, cluster.KubeProvider, "workload-nginx", "", true, dependsOnDDAgent, dependsOnVPA); err != nil {
				return err
			}
			if _, err := nginx.EksFargateAppDefinition(&awsEnv, cluster.KubeProvider, "workload-nginx-fargate", k8sAgentComponent.ClusterAgentToken, dependsOnDDAgent); err != nil {
				return err
			}
			if _, err := redis.K8sAppDefinition(&awsEnv, cluster.KubeProvider, "workload-redis", true, dependsOnDDAgent, dependsOnVPA); err != nil {
				return err
			}
			if _, err := cpustress.K8sAppDefinition(&awsEnv, cluster.KubeProvider, "workload-cpustress"); err != nil {
				return err
			}
			if _, err := dogstatsd.K8sAppDefinition(&awsEnv, cluster.KubeProvider, "workload-dogstatsd", 8125, "/var/run/datadog/dsd.socket", dependsOnDDAgent); err != nil {
				return err
			}
			if _, err := dogstatsd.EksFargateAppDefinition(&awsEnv, cluster.KubeProvider, "workload-dogstatsd-fargate", k8sAgentComponent.ClusterAgentToken, dependsOnDDAgent); err != nil {
				return err
			}
			if params.deployDogstatsd {
				if _, err := dogstatsd.K8sAppDefinition(&awsEnv, cluster.KubeProvider, "workload-dogstatsd-standalone", dogstatsdstandalone.HostPort, dogstatsdstandalone.Socket, dependsOnDDAgent); err != nil {
					return err
				}
			}
			if _, err := tracegen.K8sAppDefinition(&awsEnv, cluster.KubeProvider, "workload-tracegen"); err != nil {
				return err
			}
			if _, err := prometheus.K8sAppDefinition(&awsEnv, cluster.KubeProvider, "workload-prometheus"); err != nil {
				return err
			}
			if _, err := mutatedbyadmissioncontroller.K8sAppDefinition(&awsEnv, cluster.KubeProvider, "workload-mutated", "workload-mutated-lib-injection", dependsOnDDAgent); err != nil {
				return err
			}
			if _, err := etcd.K8sAppDefinition(&awsEnv, cluster.KubeProvider); err != nil {
				return err
			}
		}
	} else {
		env.Agent = nil
	}

	// Deploy workloads
	for _, appFunc := range params.workloadAppFuncs {
		_, err := appFunc(&awsEnv, cluster.KubeProvider)
		if err != nil {
			return err
		}
	}

	return nil
}

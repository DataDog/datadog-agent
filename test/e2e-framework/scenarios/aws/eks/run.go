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
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/prometheus"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redis"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/tracegen"
	dogstatsdstandalone "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dogstatsd-standalone"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/argorollouts"
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

	if awsEnv.InitOnly() {
		return nil
	}

	vpaCrd, err := vpa.DeployCRD(&awsEnv, cluster.KubeProvider)
	if err != nil {
		return err
	}
	dependsOnVPA := utils.PulumiDependsOn(vpaCrd)

	var dependsOnArgoRollout pulumi.ResourceOption
	if params.deployArgoRollout {
		argoParams, err := argorollouts.NewParams()
		if err != nil {
			return err
		}
		argoHelm, err := argorollouts.NewHelmInstallation(&awsEnv, argoParams, cluster.KubeProvider)
		if err != nil {
			return err
		}
		dependsOnArgoRollout = utils.PulumiDependsOn(argoHelm)
	}

	var fakeIntake *fakeintakeComp.Fakeintake
	var dependsOnDDAgent pulumi.ResourceOption
	var kubernetesAgent *agent.KubernetesAgent
	// Deploy the agent
	if params.agentOptions != nil {

		if params.fakeintakeOptions != nil {
			fakeIntakeOptions := []fakeintake.Option{
				fakeintake.WithCPU(1024),
				fakeintake.WithMemory(6144),
			}
			if awsEnv.GetCommonEnvironment().InfraShouldDeployFakeintakeWithLB() {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithLoadBalancer())
			}

			if fakeIntake, err = fakeintake.NewECSFargateInstance(awsEnv, "ecs", fakeIntakeOptions...); err != nil {
				return err
			}
			if err := fakeIntake.Export(awsEnv.Ctx(), &env.FakeIntake.FakeintakeOutput); err != nil {
				return err
			}
		} else {
			env.FakeIntake = nil
		}

		params.agentOptions = append(params.agentOptions, kubernetesagentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(cluster)), kubernetesagentparams.WithFakeintake(fakeIntake), kubernetesagentparams.WithTags([]string{"stackid:" + ctx.Stack()}))

		eksParams, err := NewParams(params.eksOptions...)
		if err != nil {
			return err
		}
		if eksParams.WindowsNodeGroup {
			params.agentOptions = append(params.agentOptions, kubernetesagentparams.WithDeployWindows())
		}

		kubernetesAgent, err = helm.NewKubernetesAgent(&awsEnv, "eks", cluster.KubeProvider, params.agentOptions...)
		if err != nil {
			return err
		}
		err = kubernetesAgent.Export(ctx, &env.Agent.KubernetesAgentOutput)
		if err != nil {
			return err
		}
		dependsOnDDAgent = utils.PulumiDependsOn(kubernetesAgent)
		// Deploy standalone dogstatsd
		if params.deployDogstatsd {
			if _, err := dogstatsdstandalone.K8sAppDefinition(&awsEnv, cluster.KubeProvider, "dogstatsd-standalone", "/run/containerd/containerd.sock", fakeIntake, true, ""); err != nil {
				return err
			}
		}
	} else {
		env.Agent = nil
	}

	if params.deployTestWorkload {

		if _, err := cpustress.K8sAppDefinition(&awsEnv, cluster.KubeProvider, "workload-cpustress"); err != nil {
			return err
		}

		// The following workloads require the Agent to be deployed.
		if params.agentOptions != nil {

			if _, err := nginx.K8sAppDefinition(&awsEnv, cluster.KubeProvider, "workload-nginx", 80, "", true, dependsOnDDAgent /* for DDM */, dependsOnVPA); err != nil {
				return err
			}

			if _, err := nginx.EksFargateAppDefinition(&awsEnv, cluster.KubeProvider, "workload-nginx-fargate", kubernetesAgent.ClusterAgentToken, dependsOnDDAgent); err != nil {
				return err
			}

			if _, err := redis.K8sAppDefinition(&awsEnv, cluster.KubeProvider, "workload-redis", true, dependsOnDDAgent /* for DDM */, dependsOnVPA); err != nil {
				return err
			}

			// dogstatsd clients that report to the Agent
			if _, err := dogstatsd.K8sAppDefinition(&awsEnv, cluster.KubeProvider, "workload-dogstatsd", 8125, "/var/run/datadog/dsd.socket", dependsOnDDAgent /* for admission */); err != nil {
				return err
			}
			if _, err := dogstatsd.EksFargateAppDefinition(&awsEnv, cluster.KubeProvider, "workload-dogstatsd-fargate", kubernetesAgent.ClusterAgentToken, dependsOnDDAgent /* for admission */); err != nil {
				return err
			}

			if params.deployDogstatsd {
				// dogstatsd clients that report to the dogstatsd standalone deployment
				if _, err := dogstatsd.K8sAppDefinition(&awsEnv, cluster.KubeProvider, "workload-dogstatsd-standalone", dogstatsdstandalone.HostPort, dogstatsdstandalone.Socket, dependsOnDDAgent /* for admission */); err != nil {
					return err
				}
			}

		}

		if _, err := tracegen.K8sAppDefinition(&awsEnv, cluster.KubeProvider, "workload-tracegen", utils.PulumiDependsOn(cluster)); err != nil {
			return err
		}

		if _, err := prometheus.K8sAppDefinition(&awsEnv, cluster.KubeProvider, "workload-prometheus", utils.PulumiDependsOn(cluster)); err != nil {
			return err
		}

		if _, err := etcd.K8sAppDefinition(&awsEnv, cluster.KubeProvider, utils.PulumiDependsOn(cluster)); err != nil {
			return err
		}

		if params.deployArgoRollout {
			if _, err := nginx.K8sRolloutAppDefinition(&awsEnv, cluster.KubeProvider, "workload-argo-rollout-nginx", dependsOnDDAgent, dependsOnArgoRollout); err != nil {
				return err
			}
		}
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

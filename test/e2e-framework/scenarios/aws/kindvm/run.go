// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kindvm

import (
	_ "embed"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/cpustress"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/dogstatsd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/etcd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/mutatedbyadmissioncontroller"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/prometheus"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redis"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/tracegen"
	csidriver "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/csi-driver"
	dogstatsdstandalone "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dogstatsd-standalone"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operator"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operatorparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/argorollouts"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/cilium"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/vpa"
	resAws "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed agent_helm_values.yaml
var agentHelmValues string

// Run is the entry point for the scenario when run via pulumi.
// It uses outputs.Kubernetes which is lightweight and doesn't pull in test dependencies.
func Run(ctx *pulumi.Context) error {
	awsEnv, err := resAws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env := outputs.NewKubernetes()

	params := ParamsFromEnvironment(awsEnv)
	return RunWithEnv(ctx, awsEnv, env, params)
}

// RunWithEnv deploys a KIND-on-EC2 environment using a provided env and params.
// It accepts KubernetesOutputs interface, enabling reuse between provisioners and direct Pulumi runs.
func RunWithEnv(ctx *pulumi.Context, awsEnv resAws.Environment, env outputs.KubernetesOutputs, params *RunParams) error {

	var err error
	var fakeIntake *fakeintakeComp.Fakeintake
	if params.fakeintakeOptions != nil {
		fakeintakeOpts := []fakeintake.Option{fakeintake.WithLoadBalancer()}
		params.fakeintakeOptions = append(fakeintakeOpts, params.fakeintakeOptions...)
		fakeIntake, err = fakeintake.NewECSFargateInstance(awsEnv, params.Name, params.fakeintakeOptions...)
		if err != nil {
			return err
		}
		err = fakeIntake.Export(ctx, env.FakeIntakeOutput())
		if err != nil {
			return err
		}

		if len(params.agentOptions) > 0 {
			newOpts := []kubernetesagentparams.Option{kubernetesagentparams.WithFakeintake(fakeIntake)}
			params.agentOptions = append(newOpts, params.agentOptions...)
		}
		if params.operatorDDAOptions != nil {
			newDdaOpts := []agentwithoperatorparams.Option{agentwithoperatorparams.WithFakeIntake(fakeIntake)}
			params.operatorDDAOptions = append(newDdaOpts, params.operatorDDAOptions...)
		}
		params.vmOptions = append(params.vmOptions, ec2.WithPulumiResourceOptions(utils.PulumiDependsOn(fakeIntake)))
	} else {
		env.DisableFakeIntake()
	}

	host, err := ec2.NewVM(awsEnv, params.Name, params.vmOptions...)
	if err != nil {
		return err
	}

	installEcrCredsHelperCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, host)
	if err != nil {
		return err
	}

	var kindCluster *kubeComp.Cluster
	if len(params.ciliumOptions) > 0 {
		kindCluster, err = cilium.NewKindCluster(&awsEnv, host, params.Name, awsEnv.KubernetesVersion(), params.ciliumOptions, utils.PulumiDependsOn(installEcrCredsHelperCmd))
	} else {
		kindCluster, err = kubeComp.NewKindCluster(&awsEnv, host, params.Name, awsEnv.KubernetesVersion(), utils.PulumiDependsOn(installEcrCredsHelperCmd))
	}

	if err != nil {
		return err
	}

	err = kindCluster.Export(ctx, env.KubernetesClusterOutput())
	if err != nil {
		return err
	}

	kubeProvider, err := kubernetes.NewProvider(ctx, awsEnv.Namer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
		EnableServerSideApply: pulumi.Bool(true),
		Kubeconfig:            kindCluster.KubeConfig,
	})
	if err != nil {
		return err
	}

	vpaCrd, err := vpa.DeployCRD(&awsEnv, kubeProvider)
	if err != nil {
		return err
	}
	dependsOnVPA := utils.PulumiDependsOn(vpaCrd)

	if len(params.ciliumOptions) > 0 {
		// deploy cilium
		ciliumParams, err := cilium.NewParams(params.ciliumOptions...)
		if err != nil {
			return err
		}

		_, err = cilium.NewHelmInstallation(&awsEnv, kindCluster, ciliumParams, pulumi.Provider(kubeProvider))
		if err != nil {
			return err
		}
	}

	var dependsOnArgoRollout pulumi.ResourceOption
	if params.deployArgoRollout {
		argoParams, err := argorollouts.NewParams()
		if err != nil {
			return err
		}
		argoHelm, err := argorollouts.NewHelmInstallation(&awsEnv, argoParams, kubeProvider)
		if err != nil {
			return err
		}
		dependsOnArgoRollout = utils.PulumiDependsOn(argoHelm)
	}

	var dependsOnDDAgent pulumi.ResourceOption
	if len(params.agentOptions) > 0 && !params.deployOperator {
		newOpts := []kubernetesagentparams.Option{kubernetesagentparams.WithHelmValues(agentHelmValues), kubernetesagentparams.WithClusterName(kindCluster.ClusterName), kubernetesagentparams.WithTags([]string{"stackid:" + ctx.Stack()})}
		params.agentOptions = append(newOpts, params.agentOptions...)
		agent, err := helm.NewKubernetesAgent(&awsEnv, "kind", kubeProvider, params.agentOptions...)
		if err != nil {
			return err
		}
		err = agent.Export(ctx, env.KubernetesAgentOutput())
		if err != nil {
			return err
		}
		dependsOnDDAgent = utils.PulumiDependsOn(agent)
	}

	if params.deployOperator {
		operatorOpts := make([]operatorparams.Option, 0)
		operatorOpts = append(
			operatorOpts,
			params.operatorOptions...,
		)

		operatorComp, err := operator.NewOperator(&awsEnv, awsEnv.Namer.ResourceName("dd-operator"), kubeProvider, operatorOpts...)
		if err != nil {
			return err
		}
		err = operatorComp.Export(ctx, nil)
		if err != nil {
			return err
		}
		dependsOnDDAgent = utils.PulumiDependsOn(operatorComp)
	}

	if params.deployDogstatsd {
		if _, err := dogstatsdstandalone.K8sAppDefinition(&awsEnv, kubeProvider, "dogstatsd-standalone", "/run/containerd/containerd.sock", fakeIntake, false, ctx.Stack()); err != nil {
			return err
		}
	}

	// Deploy testing workload
	if params.deployTestWorkload {
		// dogstatsd clients that report to the Agent
		if _, err := dogstatsd.K8sAppDefinition(&awsEnv, kubeProvider, "workload-dogstatsd", 8125, "/var/run/datadog/dsd.socket", dependsOnDDAgent /* for admission */); err != nil {
			return err
		}

		if params.deployDogstatsd {
			// dogstatsd clients that report to the dogstatsd standalone deployment
			if _, err := dogstatsd.K8sAppDefinition(&awsEnv, kubeProvider, "workload-dogstatsd-standalone", dogstatsdstandalone.HostPort, dogstatsdstandalone.Socket, dependsOnDDAgent /* for admission */); err != nil {
				return err
			}
		}

		if _, err := tracegen.K8sAppDefinition(&awsEnv, kubeProvider, "workload-tracegen"); err != nil {
			return err
		}

		if _, err := prometheus.K8sAppDefinition(&awsEnv, kubeProvider, "workload-prometheus"); err != nil {
			return err
		}

		if _, err := mutatedbyadmissioncontroller.K8sAppDefinition(&awsEnv, kubeProvider, "workload-mutated", "workload-mutated-lib-injection", dependsOnDDAgent /* for admission */); err != nil {
			return err
		}

		if _, err := etcd.K8sAppDefinition(&awsEnv, kubeProvider); err != nil {
			return err
		}

		// These workloads can be deployed only if the agent is installed, they rely on CRDs installed by Agent helm chart
		if len(params.agentOptions) > 0 {
			if _, err := nginx.K8sAppDefinition(&awsEnv, kubeProvider, "workload-nginx", 80, "", true, dependsOnDDAgent /* for DDM */, dependsOnVPA); err != nil {
				return err
			}

			if _, err := redis.K8sAppDefinition(&awsEnv, kubeProvider, "workload-redis", true, dependsOnDDAgent /* for DDM */, dependsOnVPA); err != nil {
				return err
			}

			if _, err := cpustress.K8sAppDefinition(&awsEnv, kubeProvider, "workload-cpustress"); err != nil {
				return err
			}
		}

		if params.deployArgoRollout {
			if _, err := nginx.K8sRolloutAppDefinition(&awsEnv, kubeProvider, "workload-argo-rollout-nginx", 80, dependsOnDDAgent, dependsOnArgoRollout); err != nil {
				return err
			}
		}
	}

	if dependsOnDDAgent != nil {
		for _, appFunc := range params.depWorkloadAppFuncs {
			_, err := appFunc(&awsEnv, kubeProvider, dependsOnDDAgent)
			if err != nil {
				return err
			}
		}
	}

	for _, appFunc := range params.workloadAppFuncs {
		_, err := appFunc(&awsEnv, kubeProvider)
		if err != nil {
			return err
		}
	}

	if params.deployOperator && len(params.operatorDDAOptions) > 0 {
		// Deploy the datadog CSI driver
		if err := csidriver.NewDatadogCSIDriver(&awsEnv, kubeProvider, csiDriverCommitSHA); err != nil {
			return err
		}
		ddaWithOperatorComp, err := agent.NewDDAWithOperator(&awsEnv, awsEnv.CommonNamer().ResourceName("kind-with-operator"), kubeProvider, params.operatorDDAOptions...)
		if err != nil {
			return err
		}

		if err := ddaWithOperatorComp.Export(ctx, env.KubernetesAgentOutput()); err != nil {
			return err
		}

	}

	if len(params.agentOptions) == 0 && len(params.operatorDDAOptions) == 0 {
		env.DisableAgent()
	}

	return nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kindvm

import (
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
	dogstatsdstandalone "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dogstatsd-standalone"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operator"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operatorparams"

	localKubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/vpa"
	resAws "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Run(ctx *pulumi.Context) error {
	awsEnv, err := resAws.NewEnvironment(ctx)
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

// RunWithEnv deploys a KIND-on-EC2 environment using a provided env and params
func RunWithEnv(ctx *pulumi.Context, awsEnv resAws.Environment, env *environments.Kubernetes, params *RunParams) error {

	vm, err := ec2.NewVM(awsEnv, params.Name, params.vmOptions...)
	if err != nil {
		return err
	}
	if err := vm.Export(ctx, nil); err != nil {
		return err
	}

	installEcrCredsHelperCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, vm)
	if err != nil {
		return err
	}

	var kindCluster *localKubernetes.Cluster
	if len(params.ciliumOptions) > 0 {
		kindCluster, err = localKubernetes.NewKindCluster(&awsEnv, vm, params.Name, awsEnv.KubernetesVersion(), utils.PulumiDependsOn(installEcrCredsHelperCmd))
	} else {
		kindCluster, err = localKubernetes.NewKindCluster(&awsEnv, vm, params.Name, awsEnv.KubernetesVersion(), utils.PulumiDependsOn(installEcrCredsHelperCmd))
	}
	if err != nil {
		return err
	}
	if err := kindCluster.Export(ctx, &env.KubernetesCluster.ClusterOutput); err != nil {
		return err
	}

	// Kube provider
	kindKubeProvider, err := kubernetes.NewProvider(ctx, awsEnv.Namer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
		Kubeconfig:            kindCluster.KubeConfig,
		EnableServerSideApply: pulumi.BoolPtr(true),
		DeleteUnreachable:     pulumi.BoolPtr(true),
	})
	if err != nil {
		return err
	}

	// VPA
	vpaCrd, err := vpa.DeployCRD(&awsEnv, kindKubeProvider)
	if err != nil {
		return err
	}
	dependsOnVPA := utils.PulumiDependsOn(vpaCrd)

	// Fakeintake
	var fakeIntake *fakeintakeComp.Fakeintake
	if params.fakeintakeOptions != nil {
		if fakeIntake, err = fakeintake.NewECSFargateInstance(awsEnv, kindCluster.Name(), params.fakeintakeOptions...); err != nil {
			return err
		}
		if err := fakeIntake.Export(awsEnv.Ctx(), &env.FakeIntake.FakeintakeOutput); err != nil {
			return err
		}
	} else {
		env.FakeIntake = nil
	}

	var dependsOnDDAgent pulumi.ResourceOption
	// Agent
	if params.agentOptions != nil && !params.deployOperator {
		customValues := `
datadog:
  kubelet:
    tlsVerify: false
agents:
  useHostNetwork: true
`
		k8sAgentOptions := make([]kubernetesagentparams.Option, 0)
		k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithHelmValues(customValues), kubernetesagentparams.WithClusterName(kindCluster.ClusterName))
		if fakeIntake != nil {
			k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithFakeintake(fakeIntake))
		}
		k8sAgentOptions = append(k8sAgentOptions, params.agentOptions...)
		k8sAgentComponent, err := helm.NewKubernetesAgent(&awsEnv, awsEnv.Namer.ResourceName("datadog-agent"), kindKubeProvider, k8sAgentOptions...)
		if err != nil {
			return err
		}
		if err := k8sAgentComponent.Export(awsEnv.Ctx(), &env.Agent.KubernetesAgentOutput); err != nil {
			return err
		}
		dependsOnDDAgent = utils.PulumiDependsOn(k8sAgentComponent)
	}

	// Operator
	if params.deployOperator {
		operatorOpts := []operatorparams.Option{operatorparams.WithNamespace("datadog")}
		operatorComp, err := operator.NewOperator(&awsEnv, awsEnv.CommonNamer().ResourceName("dd-operator"), kindKubeProvider, operatorOpts...)
		if err != nil {
			return err
		}
		if err := operatorComp.Export(awsEnv.Ctx(), nil); err != nil {
			return err
		}

		ddaConfig := agentwithoperatorparams.DDAConfig{
			Name: "dda-with-operator",
			YamlConfig: `
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
spec:
  global:
    kubelet:
      tlsVerify: false
`}
		ddaOptions := []agentwithoperatorparams.Option{agentwithoperatorparams.WithNamespace("datadog"), agentwithoperatorparams.WithDDAConfig(ddaConfig)}
		if fakeIntake != nil {
			ddaOptions = append(ddaOptions, agentwithoperatorparams.WithFakeIntake(fakeIntake))
		}
		ddaOptions = append(ddaOptions, params.operatorDDAOptions...)
		k8sAgentWithOperatorComp, err := agent.NewDDAWithOperator(&awsEnv, awsEnv.CommonNamer().ResourceName("datadog-agent-with-operator"), kindKubeProvider, ddaOptions...)
		if err != nil {
			return err
		}
		dependsOnDDAgent = utils.PulumiDependsOn(k8sAgentWithOperatorComp)
		if err := k8sAgentWithOperatorComp.Export(awsEnv.Ctx(), &env.Agent.KubernetesAgentOutput); err != nil {
			return err
		}
	}

	// Dogstatsd
	if params.deployDogstatsd {
		if _, err := dogstatsdstandalone.K8sAppDefinition(&awsEnv, kindKubeProvider, "dogstatsd-standalone", fakeIntake, false, ctx.Stack()); err != nil {
			return err
		}
	}

	// Workloads
	if params.deployTestWorkload {
		if _, err := nginx.K8sAppDefinition(&awsEnv, kindKubeProvider, "workload-nginx", "", true, dependsOnDDAgent, dependsOnVPA); err != nil {
			return err
		}
		if _, err := redis.K8sAppDefinition(&awsEnv, kindKubeProvider, "workload-redis", true, dependsOnDDAgent, dependsOnVPA); err != nil {
			return err
		}
		if _, err := cpustress.K8sAppDefinition(&awsEnv, kindKubeProvider, "workload-cpustress"); err != nil {
			return err
		}
		if _, err := dogstatsd.K8sAppDefinition(&awsEnv, kindKubeProvider, "workload-dogstatsd", 8125, "/var/run/datadog/dsd.socket", dependsOnDDAgent); err != nil {
			return err
		}
		if params.deployDogstatsd {
			if _, err := dogstatsd.K8sAppDefinition(&awsEnv, kindKubeProvider, "workload-dogstatsd-standalone", dogstatsdstandalone.HostPort, dogstatsdstandalone.Socket, dependsOnDDAgent); err != nil {
				return err
			}
		}
		if _, err := tracegen.K8sAppDefinition(&awsEnv, kindKubeProvider, "workload-tracegen"); err != nil {
			return err
		}
		if _, err := prometheus.K8sAppDefinition(&awsEnv, kindKubeProvider, "workload-prometheus"); err != nil {
			return err
		}
		if _, err := mutatedbyadmissioncontroller.K8sAppDefinition(&awsEnv, kindKubeProvider, "workload-mutated", "workload-mutated-lib-injection", dependsOnDDAgent); err != nil {
			return err
		}
		if _, err := etcd.K8sAppDefinition(&awsEnv, kindKubeProvider); err != nil {
			return err
		}
	}

	return nil
}

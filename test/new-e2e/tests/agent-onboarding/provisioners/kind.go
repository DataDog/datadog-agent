// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioners

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	fakeintakeComp "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	"github.com/DataDog/test-infra-definitions/components/datadog/operator"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/resources/local"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// LocalKindRunFunc is the Pulumi run function that runs the local Kind provisioner
func LocalKindRunFunc(ctx *pulumi.Context, env *environments.Kubernetes, params *KubernetesProvisionerParams) error {
	localEnv, err := local.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	kindCluster, err := kubeComp.NewLocalKindCluster(&localEnv, localEnv.CommonNamer().ResourceName("local-kind"), params.k8sVersion)
	if err != nil {
		return err
	}

	if err = kindCluster.Export(ctx, &env.KubernetesCluster.ClusterOutput); err != nil {
		return err
	}

	// Build Kubernetes provider
	kindKubeProvider, err := kubernetes.NewProvider(ctx, localEnv.CommonNamer().ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
		Kubeconfig:            kindCluster.KubeConfig,
		EnableServerSideApply: pulumi.BoolPtr(true),
	})
	if err != nil {
		return err
	}
	if params.fakeintakeOptions != nil {
		fakeIntake, intakeErr := fakeintakeComp.NewLocalDockerFakeintake(&localEnv, "fakeintake")
		if intakeErr != nil {
			return err
		}
		if err = fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput); err != nil {
			return err
		}

		if params.ddaOptions != nil {
			params.ddaOptions = append(params.ddaOptions, agentwithoperatorparams.WithFakeIntake(fakeIntake))
		}
	} else {
		env.FakeIntake = nil
	}

	ns, err := corev1.NewNamespace(ctx, localEnv.CommonNamer().ResourceName("k8s-namespace"), &corev1.NamespaceArgs{Metadata: &metav1.ObjectMetaArgs{
		Name: pulumi.String("e2e-operator"),
	}}, pulumi.Provider(kindKubeProvider))

	if err != nil {
		return err
	}

	// Install kustomizations
	kustomizeAppFunc := KustomizeWorkloadAppFunc(params.testName, params.kustomizeResources)

	e2eKustomize, err := kustomizeAppFunc(&localEnv, kindKubeProvider)
	if err != nil {
		return err
	}

	// Create Operator component
	var operatorComp *operator.Operator
	if params.operatorOptions != nil {
		operatorOpts := []pulumi.ResourceOption{
			pulumi.DependsOn([]pulumi.Resource{e2eKustomize, ns}),
		}
		params.operatorOptions = append(params.operatorOptions, operatorparams.WithPulumiResourceOptions(operatorOpts...))

		operatorComp, err = operator.NewOperator(&localEnv, localEnv.CommonNamer().ResourceName("operator"), kindKubeProvider, params.operatorOptions...)
		if err != nil {
			return err
		}
	}

	// Setup DDA options
	if params.ddaOptions != nil && params.operatorOptions != nil {
		ddaResourceOpts := []pulumi.ResourceOption{
			pulumi.DependsOn([]pulumi.Resource{e2eKustomize, operatorComp}),
		}
		params.ddaOptions = append(
			params.ddaOptions,
			agentwithoperatorparams.WithPulumiResourceOptions(ddaResourceOpts...))

		ddaComp, aErr := agent.NewDDAWithOperator(&localEnv, "agent-with-operator", kindKubeProvider, params.ddaOptions...)
		if aErr != nil {
			return aErr
		}

		if err = ddaComp.Export(ctx, &env.Agent.KubernetesAgentOutput); err != nil {
			return err
		}
	} else {
		env.Agent = nil
	}

	for _, workload := range params.yamlWorkloads {
		_, err = yaml.NewConfigFile(ctx, workload.Name, &yaml.ConfigFileArgs{
			File: workload.Path,
		}, pulumi.Provider(kindKubeProvider))
		if err != nil {
			return err
		}
	}

	for _, appFunc := range params.workloadAppFuncs {
		_, err := appFunc(&localEnv, kindKubeProvider)
		if err != nil {
			return err
		}
	}

	return nil
}

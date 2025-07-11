// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioners

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/operator"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	"github.com/DataDog/test-infra-definitions/resources/gcp"
	"github.com/DataDog/test-infra-definitions/scenarios/gcp/fakeintake"
	"github.com/DataDog/test-infra-definitions/scenarios/gcp/gke"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// GkeRunFunc is the Pulumi run function that runs the GCP GKE provisioner
func GkeRunFunc(ctx *pulumi.Context, env *environments.Kubernetes, params *KubernetesProvisionerParams) error {
	gcpEnv, err := gcp.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	cluster, err := gke.NewGKECluster(gcpEnv, params.gkeOptions...)
	if err != nil {
		return err
	}

	if err = cluster.Export(ctx, &env.KubernetesCluster.ClusterOutput); err != nil {
		return err
	}

	if params.fakeintakeOptions != nil {
		fakeIntake, fakeIntakeErr := fakeintake.NewVMInstance(gcpEnv, params.fakeintakeOptions...)
		if fakeIntakeErr != nil {
			return fakeIntakeErr
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

	ns, err := corev1.NewNamespace(ctx, gcpEnv.CommonNamer().ResourceName("k8s-namespace"), &corev1.NamespaceArgs{Metadata: &metav1.ObjectMetaArgs{
		Name: pulumi.String("e2e-operator"),
	}}, pulumi.Provider(cluster.KubeProvider))

	if err != nil {
		return err
	}

	// Install kustomizations
	kustomizeAppFunc := KustomizeWorkloadAppFunc(params.testName, params.kustomizeResources)

	e2eKustomize, err := kustomizeAppFunc(&gcpEnv, cluster.KubeProvider)
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

		operatorComp, err = operator.NewOperator(&gcpEnv, gcpEnv.CommonNamer().ResourceName("operator"), cluster.KubeProvider, params.operatorOptions...)
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

		ddaComp, aErr := agent.NewDDAWithOperator(&gcpEnv, params.name, cluster.KubeProvider, params.ddaOptions...)
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
		}, pulumi.Provider(cluster.KubeProvider))
		if err != nil {
			return err
		}
	}

	for _, appFunc := range params.workloadAppFuncs {
		_, err := appFunc(&gcpEnv, cluster.KubeProvider)
		if err != nil {
			return err
		}
	}

	return nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package openshiftvm

import (
	kubernetesProvider "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/local"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/openshift"
)

const localOpenShiftAgentHelmValues = `
datadog:
  criSocketPath: /var/run/crio/crio.sock
  confd:
    crio.yaml: |-
      init_config:
      instances:
      - prometheus_url: http://localhost:9537/metrics
`

func deployClusterResourceQuota(ctx *pulumi.Context, kubeProvider *kubernetesProvider.Provider) error {
	namespace, err := corev1.NewNamespace(ctx, "workload-clusterresourcequota", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String("workload-clusterresourcequota"),
			Labels: pulumi.StringMap{
				"clusterresourcequota-enabled": pulumi.String("true"),
			},
		},
	}, pulumi.Provider(kubeProvider))
	if err != nil {
		return err
	}

	_, err = apiextensions.NewCustomResource(ctx, "pod-cpu-quota", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("quota.openshift.io/v1"),
		Kind:       pulumi.String("ClusterResourceQuota"),
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String("pod-cpu-quota"),
		},
		OtherFields: kubernetesProvider.UntypedArgs{
			"spec": map[string]interface{}{
				"quota": map[string]interface{}{
					"hard": map[string]interface{}{
						"cpu": "2",
					},
				},
				"selector": map[string]interface{}{
					"annotations": nil,
					"labels": map[string]interface{}{
						"matchExpressions": []interface{}{
							map[string]interface{}{
								"key":      "clusterresourcequota-enabled",
								"operator": "In",
								"values":   []interface{}{"true"},
							},
						},
					},
				},
			},
		},
	}, pulumi.Provider(kubeProvider), pulumi.DependsOn([]pulumi.Resource{namespace}))
	if err != nil {
		return err
	}
	return nil
}

// Run is the entry point for the scenario when run via pulumi.
func Run(ctx *pulumi.Context) error {
	localEnv, err := local.NewEnvironment(ctx)
	if err != nil {
		return err
	}
	return RunWithParams(ctx, localEnv, ParamsFromEnvironment(localEnv))
}

func RunWithParams(ctx *pulumi.Context, localEnv local.Environment, params *Params) error {
	cluster, err := kubernetes.NewLocalOpenShiftCluster(&localEnv, "openshift", params.OpenShiftClusterArgs)
	if err != nil {
		return err
	}
	if err := cluster.Export(ctx, nil); err != nil {
		return err
	}

	if localEnv.InitOnly() {
		return nil
	}

	kubeProvider, err := kubernetesProvider.NewProvider(ctx, localEnv.CommonNamer().ResourceName("openshift-k8s-provider"), &kubernetesProvider.ProviderArgs{
		Kubeconfig:            cluster.KubeConfig,
		EnableServerSideApply: pulumi.BoolPtr(true),
		DeleteUnreachable:     pulumi.BoolPtr(true),
	})
	if err != nil {
		return err
	}
	if err := deployClusterResourceQuota(ctx, kubeProvider); err != nil {
		return err
	}

	var fakeIntake *fakeintakeComp.Fakeintake
	if params.deployFakeIntake {
		fakeIntake, err = fakeintakeComp.NewLocalDockerFakeintake(&localEnv, "fakeintake")
		if err != nil {
			return err
		}
		if err := fakeIntake.Export(ctx, nil); err != nil {
			return err
		}
	}

	return openshift.DeployComponents(ctx, &localEnv, kubeProvider, cluster, fakeIntake, params.AgentOptions)
}

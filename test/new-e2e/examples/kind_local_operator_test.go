// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"context"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	fakeintakeComp "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	compkube "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/resources/local"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
)

const namespace = "e2e-operator"

type localKindOperatorSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func localKindOperatorProvisioner() e2e.PulumiEnvRunFunc[environments.Kubernetes] {
	return func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		kindEnv, err := local.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		kindCluster, err := compkube.NewLocalKindCluster(&kindEnv, kindEnv.CommonNamer().ResourceName("kind-operator"), kindEnv.KubernetesVersion())
		if err != nil {
			return err
		}

		if err := kindCluster.Export(ctx, &env.KubernetesCluster.ClusterOutput); err != nil {
			return err
		}

		// Build Kubernetes provider
		kindKubeProvider, err := kubernetes.NewProvider(ctx, kindEnv.CommonNamer().ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
			Kubeconfig:            kindCluster.KubeConfig,
			EnableServerSideApply: pulumi.BoolPtr(true),
		})
		if err != nil {
			return err
		}

		fakeIntake, err := fakeintakeComp.NewLocalDockerFakeintake(&kindEnv, "fakeintake")
		if err != nil {
			return err
		}
		if err := fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput); err != nil {
			return err
		}

		// Setup operator options
		operatorOpts := make([]operatorparams.Option, 0)
		operatorOpts = append(
			operatorOpts,
			operatorparams.WithNamespace(namespace),
		)

		customDDA := `
spec:
  features:
    apm:
      enabled: true
`
		// Setup DDA options
		ddaOptions := make([]agentwithoperatorparams.Option, 0)
		ddaOptions = append(
			ddaOptions,
			agentwithoperatorparams.WithNamespace(namespace),
			agentwithoperatorparams.WithTLSKubeletVerify(false),
			agentwithoperatorparams.WithDDAConfig(customDDA),
		)

		// Create Datadog Agent with Operator component
		operatorAgentComponent, err := agent.NewDDAWithOperator(&kindEnv, kindEnv.CommonNamer().ResourceName("dd-operator-agent"), kindKubeProvider, operatorOpts, ddaOptions...)

		if err != nil {
			return err
		}

		if err := operatorAgentComponent.Export(ctx, &env.Agent.KubernetesAgentOutput); err != nil {
			return err
		}

		return nil
	}
}

func TestOperatorKindSuite(t *testing.T) {
	e2e.Run(t, &localKindOperatorSuite{}, e2e.WithPulumiProvisioner(localKindOperatorProvisioner(), nil))
}

func (k *localKindOperatorSuite) TestClusterAgentInstalled() {
	res, _ := k.Env().KubernetesCluster.Client().CoreV1().Pods(namespace).List(context.TODO(), v1.ListOptions{})
	containsClusterAgent := false
	for _, pod := range res.Items {
		if strings.Contains(pod.Name, "cluster-agent") {
			containsClusterAgent = true
			break
		}
	}
	assert.True(k.T(), containsClusterAgent, "Cluster Agent not found")
}

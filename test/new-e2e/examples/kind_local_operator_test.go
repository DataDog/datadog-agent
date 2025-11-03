// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"context"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentwithoperatorparams"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operator"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operatorparams"
	compkube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/local"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

const namespace = "datadog"

type localKindOperatorSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func localKindOperatorProvisioner() provisioners.PulumiEnvRunFunc[environments.Kubernetes] {
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

		kindCluster.KubeProvider = kindKubeProvider

		fakeIntake, err := fakeintakeComp.NewLocalDockerFakeintake(&kindEnv, "fakeintake")
		if err != nil {
			return err
		}
		if err := fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput); err != nil {
			return err
		}

		// Setup default operator options
		operatorOpts := make([]operatorparams.Option, 0)
		operatorOpts = append(
			operatorOpts,
			operatorparams.WithNamespace(namespace),
		)

		// Create Operator component
		_, err = operator.NewOperator(&kindEnv, kindEnv.CommonNamer().ResourceName("dd-operator"), kindCluster.KubeProvider, operatorOpts...)

		if err != nil {
			return err
		}

		customDDA := agentwithoperatorparams.DDAConfig{
			Name: "ccr-enabled",
			YamlConfig: `
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
spec:
  global:
    kubelet:
      tlsVerify: false
  features:
    clusterChecks:
      useClusterChecksRunners: true
`,
		}

		// Setup DDA options
		ddaOptions := make([]agentwithoperatorparams.Option, 0)
		ddaOptions = append(
			ddaOptions,
			agentwithoperatorparams.WithDDAConfig(customDDA),
			agentwithoperatorparams.WithNamespace(namespace),
			agentwithoperatorparams.WithFakeIntake(fakeIntake),
		)

		// Create Datadog Agent with Operator component
		agentComponent, err := agent.NewDDAWithOperator(&kindEnv, ctx.Stack(), kindCluster.KubeProvider, ddaOptions...)

		if err != nil {
			return err
		}
		err = agentComponent.Export(ctx, &env.Agent.KubernetesAgentOutput)
		if err != nil {
			return err
		}

		return nil
	}
}

func TestOperatorKindSuite(t *testing.T) {
	e2e.Run(t, &localKindOperatorSuite{}, e2e.WithPulumiProvisioner(localKindOperatorProvisioner(), nil))
}

func (k *localKindOperatorSuite) TestClusterChecksRunner() {
	res, _ := k.Env().KubernetesCluster.Client().CoreV1().Pods(namespace).List(context.TODO(), v1.ListOptions{})
	containsCCR := false
	for _, pod := range res.Items {
		if strings.Contains(pod.Name, "cluster-checks-runner") {
			containsCCR = true
			break
		}
	}
	assert.True(k.T(), containsCCR, "Cluster checks runner not found")
}

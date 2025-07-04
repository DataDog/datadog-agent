// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"context"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/operator"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	"github.com/DataDog/test-infra-definitions/resources/gcp"
	fakeintakeComp "github.com/DataDog/test-infra-definitions/scenarios/gcp/fakeintake"
	"github.com/DataDog/test-infra-definitions/scenarios/gcp/gke"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
)

type gkeOperatorSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func gkeOperatorProvisioner() provisioners.PulumiEnvRunFunc[environments.Kubernetes] {
	return func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		gcpEnv, err := gcp.NewEnvironment(ctx)
		if err != nil {
			return err
		}
		var clusterOptions []gke.Option // ?

		cluster, err := gke.NewGKECluster(gcpEnv, clusterOptions...)
		if err != nil {
			return err
		}

		if err := cluster.Export(ctx, &env.KubernetesCluster.ClusterOutput); err != nil {
			return err
		}
		var fakeintakeOpts []fakeintakeComp.Option

		fakeIntake, fakeIntakeErr := fakeintakeComp.NewVMInstance(gcpEnv, fakeintakeOpts...)
		if fakeIntakeErr != nil {
			return fakeIntakeErr
		}
		if err = fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput); err != nil {
			return err
		}

		// Setup default operator options
		operatorOpts := make([]operatorparams.Option, 0)
		operatorOpts = append(
			operatorOpts,
			operatorparams.WithNamespace(namespace),
		)

		// Create Operator component
		_, err = operator.NewOperator(&gcpEnv, gcpEnv.CommonNamer().ResourceName("dd-operator"), cluster.KubeProvider, operatorOpts...)

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
		agentComponent, err := agent.NewDDAWithOperator(&gcpEnv, ctx.Stack(), cluster.KubeProvider, ddaOptions...)

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

func TestOperatorGKE(t *testing.T) {
	e2e.Run(t, &gkeOperatorSuite{}, e2e.WithPulumiProvisioner(gkeOperatorProvisioner(), nil))

}

func (k *gkeOperatorSuite) TestClusterChecksRunner() {
	res, _ := k.Env().KubernetesCluster.Client().CoreV1().Pods(namespace).List(context.TODO(), v1.ListOptions{})
	containsCCR := false
	for _, pod := range res.Items {
		if strings.Contains(pod.Name, "cluster-checks-runner") {
			containsCCR = true
			break
		}
	}
	assert.True(k.T(), containsCCR, "Cluster checks runner not found")

	metrics, err := k.Env().FakeIntake.Client().GetMetricNames()
	assert.NoError(k.T(), err)
	assert.NotEmpty(k.T(), metrics, "No metrics received from the agent")
	k.T().Logf("Received metrics: %v", metrics)
}

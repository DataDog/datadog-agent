// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"context"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentwithoperatorparams"
	scenariokindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type kindOperatorSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestKindOperatorSuite(t *testing.T) {
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

	e2e.Run(t, &kindOperatorSuite{}, e2e.WithProvisioner(provkindvm.Provisioner(
		provkindvm.WithRunOptions(
			scenariokindvm.WithDeployOperator(),
			scenariokindvm.WithOperatorDDAOptions([]agentwithoperatorparams.Option{
				agentwithoperatorparams.WithDDAConfig(customDDA),
			}...),
		))),
	)
}

func (k *kindOperatorSuite) TestClusterChecksRunner() {
	{
		res, _ := k.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.TODO(), v1.ListOptions{})
		containsCCR := false
		for _, pod := range res.Items {
			if strings.Contains(pod.Name, "cluster-checks-runner") {
				containsCCR = true
				break
			}
		}
		assert.True(k.T(), containsCCR, "Cluster checks runner not found")
	}
}

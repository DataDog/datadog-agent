// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

// This file demonstrates the recommended pattern for Kubernetes E2E tests
// using the non-Pulumi installer approach:
//
//  1. The Pulumi provisioner creates only the KinD cluster and fakeintake.
//  2. provkindvm.WithAgentOptions triggers helmagent.Install in the provisioner's
//     PostProvision step — the agent is installed via Helm, not via Pulumi.
//     KinD-specific defaults (kubelet TLS skip, host network, CSI driver) are
//     applied automatically; only pass test-specific overrides here.
//  3. provkindvm.WithWorkloads deploys test workloads via the K8s API after
//     the agent is ready, also without Pulumi.
//  4. agent.Configure reconfigures the agent mid-suite without re-running Pulumi,
//     replacing the old UpdateEnv pattern for agent-only changes.
//
// If you need direct control over the Helm install (e.g. for a custom environment),
// call helmagent.Install directly in your SetupSuite instead.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/workloads"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

type helmAgentInstallSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestHelmAgentInstall(t *testing.T) {
	// WithAgentOptions causes the provisioner to call helmagent.Install in
	// its PostProvision step, after the KinD cluster is ready.
	// WithWorkloads deploys test workloads via the K8s API after agent install.
	// WithoutDeployTestWorkload prevents Pulumi from deploying the old-style
	// workloads so the two approaches don't conflict.
	e2e.Run(t, &helmAgentInstallSuite{}, e2e.WithProvisioner(
		provkindvm.Provisioner(
			provkindvm.WithRunOptions(scenkindvm.WithoutDeployTestWorkload()),
			provkindvm.WithAgentOptions(
				kubernetesagentparams.WithHelmValues(`
datadog:
  logs:
    enabled: true
    containerCollectAll: true
  processAgent:
    processCollection: true
`),
			),
			provkindvm.WithWorkloads(
				workloads.WithNginx(),
				workloads.WithRedis(),
				workloads.WithTracegen(),
			),
		),
	))
}

// TestAgentPodsRunning verifies the agent DaemonSet pods are up after
// helmagent.Install completed in PostProvision.
func (s *helmAgentInstallSuite) TestAgentPodsRunning() {
	pods, err := s.Env().KubernetesCluster.Client().
		CoreV1().Pods("datadog").List(context.TODO(), v1.ListOptions{})
	require.NoError(s.T(), err)
	assert.Greater(s.T(), len(pods.Items), 0, "no agent pods found in namespace 'datadog'")
}

// TestFakeIntakeReceivesKubernetesMetrics verifies the Helm-installed agent
// sends Kubernetes metrics to the fakeintake.
func (s *helmAgentInstallSuite) TestFakeIntakeReceivesKubernetesMetrics() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("kubernetes.pods.running")
		assert.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no kubernetes.pods.running metrics yet")
	}, 5*time.Minute, 10*time.Second)
}

// TestReconfigure shows agent.Configure as a lightweight alternative to
// UpdateEnv: it upgrades the Helm release without re-running Pulumi,
// making config-only changes faster.
func (s *helmAgentInstallSuite) TestReconfigure() {
	s.Env().Agent.Configure(s.T(),
		kubernetesagentparams.WithHelmValues(`
datadog:
  logs:
    enabled: true
    containerCollectAll: true
  processAgent:
    processCollection: true
  apm:
    portEnabled: true
`),
	)

	// Verify the reconfigured agent is still healthy.
	pods, err := s.Env().KubernetesCluster.Client().
		CoreV1().Pods("datadog").List(context.TODO(), v1.ListOptions{})
	require.NoError(s.T(), err)
	assert.Greater(s.T(), len(pods.Items), 0, "no agent pods after reconfigure")
}

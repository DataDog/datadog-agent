// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package process

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"testing"
	"text/template"
	"time"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/cpustress"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeClient "k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"
)

// helmTemplate define the embedded minimal configuration for NPM
//
//go:embed config/helm-template.tmpl
var helmTemplate string

type helmConfig struct {
	ProcessAgentEnabled        bool
	ProcessCollection          bool
	ProcessDiscoveryCollection bool
}

func createHelmValues(cfg helmConfig) (string, error) {
	var buffer bytes.Buffer
	tmpl, err := template.New("agent").Parse(helmTemplate)
	if err != nil {
		return "", err
	}
	err = tmpl.Execute(&buffer, cfg)
	if err != nil {
		return "", err
	}
	return buffer.String(), nil
}

type K8sSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestK8sTestSuite(t *testing.T) {
	t.Parallel()
	helmValues, err := createHelmValues(helmConfig{
		ProcessAgentEnabled: true,
		ProcessCollection:   true,
	})
	require.NoError(t, err)

	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awskubernetes.KindProvisioner(
			awskubernetes.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
				return cpustress.K8sAppDefinition(e, kubeProvider, "workload-stress")
			}),
			awskubernetes.WithAgentOptions(kubernetesagentparams.WithHelmValues(helmValues)),
		)),
	}

	e2e.Run(t, &K8sSuite{}, options...)
}

func (s *K8sSuite) TestProcessCheck() {
	t := s.T()

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		status := k8sAgentStatus(t, s.Env().KubernetesCluster)
		assert.ElementsMatch(t, []string{"process", "rtprocess"}, status.ProcessAgentStatus.Expvars.Map.EnabledChecks)
	}, 2*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessCollected(t, payloads, false, "stress-ng-cpu [run]")
	assertContainersCollected(t, payloads, []string{"stress-ng"})
}

func (s *K8sSuite) TestProcessDiscoveryCheck() {
	s.T().Skip("WIP: test is failing as process collection is still enabled with 'DD_PROCESS_AGENT_ENABLED=true'." +
		"The bug appears to be in test-infra-definitions and it's default helm values taking precedence")
	t := s.T()
	helmValues, err := createHelmValues(helmConfig{
		ProcessAgentEnabled:        true,
		ProcessDiscoveryCollection: true,
	})
	require.NoError(t, err)

	s.UpdateEnv(awskubernetes.KindProvisioner(
		awskubernetes.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
			return cpustress.K8sAppDefinition(e, kubeProvider, "workload-stress")
		}),
		awskubernetes.WithAgentOptions(kubernetesagentparams.WithHelmValues(helmValues)),
	))

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		status := k8sAgentStatus(t, s.Env().KubernetesCluster)
		assert.ElementsMatch(t, []string{"process_discovery"}, status.ProcessAgentStatus.Expvars.Map.EnabledChecks)
	}, 2*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessDiscoveryPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcessDiscoveries()
		assert.NoError(c, err, "failed to get process discovery payloads from fakeintake")
		assert.NotEmpty(c, payloads, "no process discovery payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessDiscoveryCollected(t, payloads, "stress-ng-cpu [run]")
}

func (s *K8sSuite) TestManualProcessCheck() {
	checkOutput := execProcessAgentCheck(s.T(), s.Env().KubernetesCluster, "process")
	assertManualProcessCheck(s.T(), checkOutput, false, "stress-ng-cpu [run]", "stress-ng")
}

func (s *K8sSuite) TestManualProcessDiscoveryCheck() {
	checkOutput := execProcessAgentCheck(s.T(), s.Env().KubernetesCluster, "process_discovery")
	assertManualProcessDiscoveryCheck(s.T(), checkOutput, "stress-ng-cpu [run]")
}

func (s *K8sSuite) TestManualContainerCheck() {
	checkOutput := execProcessAgentCheck(s.T(), s.Env().KubernetesCluster, "container")
	assertManualContainerCheck(s.T(), checkOutput, "stress-ng")
}

func execProcessAgentCheck(t *testing.T, cluster *components.KubernetesCluster, check string) string {
	agent := getAgentPod(t, cluster.Client())
	cmd := fmt.Sprintf("DD_LOG_LEVEL=OFF process-agent check %s -w 5s --json", check)

	// The log level needs to be overridden as the pod has an ENV var set.
	// This is so we get just json back from the check
	stdout, stderr, err := cluster.KubernetesClient.
		PodExec(agent.Namespace, agent.Name, "process-agent", []string{"bash", "-c", cmd})
	assert.NoError(t, err)
	assert.Empty(t, stderr)

	return stdout
}

func k8sAgentStatus(t *testing.T, cluster *components.KubernetesCluster) AgentStatus {
	agent := getAgentPod(t, cluster.Client())

	stdout, stderr, err := cluster.KubernetesClient.
		PodExec(agent.Namespace, agent.Name, "agent",
			[]string{"bash", "-c", "DD_LOG_LEVEL=OFF agent status --json"})
	require.NoError(t, err)
	assert.Empty(t, stderr)
	assert.NotNil(t, stdout, "failed to get agent status")

	var statusMap AgentStatus
	err = json.Unmarshal([]byte(stdout), &statusMap)
	assert.NoError(t, err, "failed to unmarshal agent status")

	return statusMap
}

func getAgentPod(t *testing.T, client kubeClient.Interface) corev1.Pod {
	res, err := client.CoreV1().Pods("datadog").
		List(context.Background(), v1.ListOptions{LabelSelector: "app=dda-linux-datadog"})
	require.NoError(t, err)
	require.NotEmpty(t, res.Items)

	return res.Items[0]
}

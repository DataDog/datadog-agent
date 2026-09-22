// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cspm contains the e2e tests for cspm
package cspm

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

type cspmTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

type findings = map[string][]map[string]string

//go:embed values.yaml
var values string

func TestCSPM(t *testing.T) {
	e2e.Run(t, &cspmTestSuite{}, e2e.WithProvisioner(
		provkindvm.Provisioner(
			provkindvm.WithRunOptions(
				scenkindvm.WithAgentOptions(kubernetesagentparams.WithHelmValues(values)),
				// Surface the EC2 host's dockerd inside the kind nodes so
				// TestDockerRulesFilteredOnContainerdCRI can reproduce the
				// GKE-COS shape (containerd CRI alongside dockerd).
				scenkindvm.WithMountDockerSocket(),
			),
		),
	))
}

// TestFindings checks that the CIS Kubernetes benchmark still loads and
// evaluates in the Cluster Agent. 5.3.2 is the one surviving rule that
// resolves against the API server; a kind cluster defines no NetworkPolicy,
// so it reports failed.
func (s *cspmTestSuite) TestFindings() {
	pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.Background(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", s.Env().Agent.LinuxClusterAgent.LabelSelectors["app"]).String(),
	})
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), pods.Items)
	clusterAgentPod := pods.Items[0].Name

	_, _, err = s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", clusterAgentPod, "cluster-agent",
		[]string{"datadog-cluster-agent", "compliance", "check", "--dump-reports", "/tmp/reports"})
	require.NoError(s.T(), err)
	dumpContent, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", clusterAgentPod, "cluster-agent",
		[]string{"cat", "/tmp/reports"})
	require.NoError(s.T(), err)
	findings, err := parseFindingOutput(dumpContent)
	require.NoError(s.T(), err)

	results := findings["cis-kubernetes-1.5.1-5.3.2"]
	require.NotEmpty(s.T(), results, "the Cluster Agent must still evaluate cis-kubernetes-1.5.1-5.3.2")
	for _, result := range results {
		assert.Contains(s.T(), []string{"passed", "failed"}, result["result"])
	}
}

func (s *cspmTestSuite) waitForSecurityAgentPodReady(namespace, podName string) string {
	s.T().Helper()

	var selectedPodName string
	require.Eventuallyf(s.T(), func() bool {
		pod, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
		if err != nil {
			s.T().Logf("unable to get pod %s: %v", podName, err)
			return false
		}
		if !podHasContainer(pod, "security-agent") {
			s.T().Logf("pod %s does not contain security-agent yet", pod.Name)
			return false
		}
		if !isContainerReady(pod, "security-agent") {
			s.T().Logf("security-agent container is not ready yet in pod %s", pod.Name)
			return false
		}
		selectedPodName = pod.Name
		return true
	}, 3*time.Minute, 5*time.Second, "security-agent container was not ready in pod %s", podName)

	return selectedPodName
}

func podHasContainer(pod *corev1.Pod, containerName string) bool {
	for _, container := range pod.Spec.Containers {
		if container.Name == containerName {
			return true
		}
	}
	return false
}

func isContainerReady(pod *corev1.Pod, containerName string) bool {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == containerName {
			return status.Ready
		}
	}
	return false
}

// TestDockerRulesFilteredOnContainerdCRI reproduces the GKE-COS shape -
// kubelet on containerd with a reachable dockerd - and asserts the filter
// suppresses CIS Docker rules.
func (s *cspmTestSuite) TestDockerRulesFilteredOnContainerdCRI() {
	pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.Background(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
	})
	require.NoError(s.T(), err)
	require.Len(s.T(), pods.Items, 1)
	agentPod := s.waitForSecurityAgentPodReady("datadog", pods.Items[0].Name)

	// Without a reachable dockerd the scope gate would skip docker rules
	// on its own, so the test would pass for the wrong reason.
	info, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(
		"datadog", agentPod, "security-agent",
		[]string{"curl", "-sS", "--unix-socket", "/host/var/run/docker.sock", "http://localhost/info"})
	require.NoError(s.T(), err, "agent pod must be able to reach dockerd via /host/var/run/docker.sock")
	require.Contains(s.T(), info, `"ServerVersion"`, "/info response from the agent pod must look like a real Docker daemon")

	_, _, err = s.Env().KubernetesCluster.KubernetesClient.PodExec(
		"datadog", agentPod, "security-agent",
		[]string{"security-agent", "compliance", "check", "--dump-reports", "/tmp/reports", "--report"})
	require.NoError(s.T(), err)
	dump, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(
		"datadog", agentPod, "security-agent",
		[]string{"cat", "/tmp/reports"})
	require.NoError(s.T(), err)
	findings, err := parseFindingOutput(dump)
	require.NoError(s.T(), err)

	for rule := range findings {
		assert.NotContains(s.T(), rule, "cis-docker-1.2.0-",
			"CIS Docker rule %q must be filtered when kubelet's CRI runtime is containerd", rule)
	}

	// The assertion above holds vacuously on an agent that shipped no Docker
	// rules at all, so check the benchmark is on disk and was filtered rather
	// than missing.
	_, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(
		"datadog", agentPod, "security-agent",
		[]string{"test", "-s", "/etc/datadog-agent/compliance.d/cis-docker-1.2.0.yaml"})
	require.NoError(s.T(), err, "the CIS Docker benchmark must ship in the agent image: %s", stderr)
}

func (s *cspmTestSuite) TestMetrics() {
	s.T().Log("Waiting for datadog.security_agent.compliance.running metrics")
	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {

		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("datadog.security_agent.compliance.running")
		require.NoError(c, err)
		if assert.NotEmpty(c, metrics) {
			s.T().Log("Metrics found: datadog.security_agent.compliance.running")
		}
	}, 2*time.Minute, 10*time.Second)

	s.T().Log("Waiting for datadog.security_agent.compliance.containers_running metrics")
	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("datadog.security_agent.compliance.containers_running")
		require.NoError(c, err)
		if assert.NotEmpty(c, metrics) {
			s.T().Log("Metrics found: datadog.security_agent.compliance.containers_running")
		}
	}, 2*time.Minute, 10*time.Second)

}
func parseFindingOutput(output string) (findings, error) {

	result := map[string]any{}
	parsedResult := findings{}
	err := json.Unmarshal([]byte(output), &result)
	if err != nil {
		return nil, err
	}
	for rule, ruleFindings := range result {
		ruleFindingsCasted, ok := ruleFindings.([]any)
		if !ok {
			return nil, fmt.Errorf("failed to parse output: %s for rule %s cannot be casted into []any", ruleFindings, rule)
		}
		parsedRuleFinding := []map[string]string{}
		for _, finding := range ruleFindingsCasted {
			findingCasted, ok := finding.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("failed to parse output: %s for rule %s cannot be casted into map[string]any", finding, rule)
			}
			parsedFinding := map[string]string{}
			for k, v := range findingCasted {
				if _, ok := v.(string); ok {
					parsedFinding[k] = v.(string)
				}
			}
			parsedRuleFinding = append(parsedRuleFinding, parsedFinding)

		}
		parsedResult[rule] = parsedRuleFinding

	}
	return parsedResult, nil
}

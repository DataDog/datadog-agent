// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build e2e
// +build e2e

package agent_onboarding

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-onboarding/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-onboarding/provisioners"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-onboarding/utils"
	kubeClient "k8s.io/client-go/kubernetes"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/stretchr/testify/assert"
)

var (
	DefaultOperatorOpts = []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
		operatorparams.WithHelmValues("installCRDs: false"),
	}

	matchTags = []*regexp.Regexp{regexp.MustCompile("kube_container_name:.*")}
	matchOpts = []client.MatchOpt[*aggregator.MetricSeries]{client.WithMatchingTags[*aggregator.MetricSeries](matchTags)}
)

type k8sSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	local     bool
	k8sClient kubeClient.Interface
}

//func (s *k8sSuite) BeforeTest() {
//	if s.Env().KubernetesCluster.Client() != nil {
//		s.k8sClient = s.Env().KubernetesCluster.Client()
//	}
//	s.BeforeTest()
//}

func (s *k8sSuite) TestGenericK8s() {
	var ddaConfigPath string
	var err error
	if s.Env().KubernetesCluster.KubernetesClient.K8sClient != nil {
		s.k8sClient = s.Env().KubernetesCluster.KubernetesClient.K8sClient
	}

	defaultDDAOpts := []agentwithoperatorparams.Option{
		agentwithoperatorparams.WithNamespace(common.NamespaceName),
	}

	defaultProvisionerOpts := []provisioners.KubernetesProvisionerOption{
		provisioners.WithK8sVersion(common.K8sVersion),
		provisioners.WithOperatorOptions(DefaultOperatorOpts...),
		provisioners.WithLocal(s.local),
	}

	s.T().Run("Verify Operator", func(t *testing.T) {
		s.EventuallyWithT(func(c *assert.CollectT) {
			utils.VerifyOperator(t, common.NamespaceName, s.k8sClient)
		}, 120*time.Second, 15*time.Second, "Could not validate operator pod in time")
	})

	s.T().Run("Minimal DDA config", func(t *testing.T) {
		// Install DDA
		ddaConfigPath, err = common.GetAbsPath(common.DdaMinimalPath)
		assert.NoError(t, err)

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "dda-minimum",
				YamlFilePath: ddaConfigPath,
			}),
		}
		ddaOpts = append(ddaOpts, defaultDDAOpts...)

		provisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-minimal-dda"),
			provisioners.WithK8sVersion(common.K8sVersion),
			provisioners.WithOperatorOptions(DefaultOperatorOpts...),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithLocal(s.local),
		}

		s.UpdateEnv(provisioners.KubernetesProvisioner(provisionerOptions...))

		s.EventuallyWithT(func(c *assert.CollectT) {
			utils.VerifyAgentPods(t, common.NamespaceName, s.k8sClient, common.NodeAgentSelector+",agent.datadoghq.com/name=dda-minimum")
			utils.VerifyNumPodsForSelector(t, common.NamespaceName, s.k8sClient, 1, common.ClusterAgentSelector+",agent.datadoghq.com/name=dda-minimum")

		}, 60*time.Second, 15*time.Second, "Agent pods did not become ready in time.")

		agentPods, err := s.k8sClient.CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-minimum",
			FieldSelector: "status.phase=Running"})
		assert.NoError(t, err)

		s.EventuallyWithTf(func(c *assert.CollectT) {
			for _, pod := range agentPods.Items {
				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, pod.Name, "agent", []string{"agent", "status", "collector", "-j"})
				assert.NoError(c, err)
				utils.VerifyCheck(c, output, "kubelet")
			}
		}, 120*time.Second, 15*time.Second, "could not validate kubelet check on agent pod")

		s.EventuallyWithTf(func(c *assert.CollectT) {
			metricNames, err := s.Env().FakeIntake.Client().GetMetricNames()
			s.Assert().NoError(err)
			s.Assert().Contains(metricNames, "kubernetes.cpu.usage.total")

			metrics, err := s.Env().FakeIntake.Client().FilterMetrics("kubernetes.cpu.usage.total", matchOpts...)
			s.Assert().NoError(err)
			for _, metric := range metrics {
				for _, points := range metric.Points {
					s.Assert().Greater(points.Value, float64(0))
				}
			}
		}, 120*time.Second, 15*time.Second, "Could not verify kubelet metrics in time")

		s.EventuallyWithTf(func(c *assert.CollectT) {
			clusterAgentPods, err := s.k8sClient.CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: common.ClusterAgentSelector + ",agent.datadoghq.com/e2e-test=datadog-agent-minimum"})
			assert.NoError(t, err)

			for _, pod := range clusterAgentPods.Items {
				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, pod.Name, "agent", []string{"agent", "status", "collector", "-j"})
				assert.NoError(c, err)
				utils.VerifyCheck(c, output, "kubernetes_state_core")
			}
		}, 300*time.Second, 15*time.Second, "could not validate kubernetes_state_core check on cluster agent pod")

		s.EventuallyWithTf(func(c *assert.CollectT) {
			verifyKSMCheck(s)
		}, 300*time.Second, 15*time.Second, "could not validate KSM (cluster check) metrics in time")
	})

	s.T().Run("KSM check works (cluster check runner)", func(t *testing.T) {
		// Update DDA
		ddaConfigPath, err = common.GetAbsPath(filepath.Join(common.ManifestsPath, "datadog-agent-ccr-enabled.yaml"))
		assert.NoError(t, err)

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "dda-minimum",
				YamlFilePath: ddaConfigPath,
			}),
		}
		ddaOpts = append(ddaOpts, defaultDDAOpts...)

		provisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-ksm-ccr"),
			provisioners.WithK8sVersion(common.K8sVersion),
			provisioners.WithOperatorOptions(DefaultOperatorOpts...),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithLocal(s.local),
		}

		s.UpdateEnv(provisioners.KubernetesProvisioner(provisionerOptions...))
		utils.VerifyAgentPods(t, common.NamespaceName, s.k8sClient, "app.kubernetes.io/instance=datadog-ccr-enabled-agent")

		utils.VerifyNumPodsForSelector(t, common.NamespaceName, s.k8sClient, 1, "app.kubernetes.io/instance=datadog-ccr-enabled-cluster-checks-runner")

		s.EventuallyWithTf(func(c *assert.CollectT) {
			ccrPods, err := s.k8sClient.CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/instance=datadog-ccr-enabled-cluster-checks-runner"})
			assert.NoError(t, err)

			for _, ccr := range ccrPods.Items {
				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, ccr.Name, "agent", []string{"agent", "status", "collector", "-j"})
				assert.NoError(c, err)
				utils.VerifyCheck(c, output, "kubernetes_state_core")
			}
		}, 300*time.Second, 15*time.Second, "could not validate kubernetes_state_core check on cluster check runners pod")

		s.EventuallyWithTf(func(c *assert.CollectT) {
			verifyKSMCheck(s)
		}, 300*time.Second, 15*time.Second, "could not validate kubernetes_state_core (cluster check on CCR) check in time")
	})

	s.T().Run("Autodiscovery works", func(t *testing.T) {
		// Install DDA
		ddaConfigPath, err = common.GetAbsPath(common.DdaMinimalPath)
		assert.NoError(t, err)

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{Name: "dda-autodiscovery", YamlFilePath: ddaConfigPath}),
		}
		ddaOpts = append(ddaOpts, defaultDDAOpts...)

		provisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-autodiscovery"),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithYAMLWorkload(provisioners.YAMLWorkload{Name: "nginx", Path: strings.Join([]string{common.ManifestsPath, "autodiscovery-annotation.yaml"}, "/")}),
		}
		provisionerOptions = append(provisionerOptions, defaultProvisionerOpts...)

		// Add nginx with annotations
		s.UpdateEnv(provisioners.KubernetesProvisioner(provisionerOptions...))

		utils.VerifyNumPodsForSelector(t, common.NamespaceName, s.k8sClient, 1, "app=nginx")

		utils.VerifyAgentPods(t, common.NamespaceName, s.k8sClient, common.NodeAgentSelector+",agent.datadoghq.com/name=dda-autodiscovery")

		// check agent pods for http check
		s.EventuallyWithTf(func(c *assert.CollectT) {
			agentPods, err := s.k8sClient.CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-autodiscovery",
				FieldSelector: "status.phase=Running"})
			assert.NoError(t, err)

			assert.NoError(c, err)

			for _, pod := range agentPods.Items {
				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, pod.Name, "agent", []string{"agent", "status", "collector", "-j"})
				assert.NoError(c, err)

				utils.VerifyCheck(c, output, "http_check")
			}
		}, 60*time.Second, 15*time.Second, "could not validate http check on agent pod")

		s.EventuallyWithTf(func(c *assert.CollectT) {
			verifyHTTPCheck(s)
		}, 60*time.Second, 15*time.Second, "could not validate http.can_connect check fake intake client")
	})

	s.T().Run("Logs collection works", func(t *testing.T) {
		// Update DDA
		ddaConfigPath, err = common.GetAbsPath(filepath.Join(common.ManifestsPath, "datadog-agent-logs.yaml"))
		assert.NoError(t, err)

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "datadog-agent-logs",
				YamlFilePath: ddaConfigPath,
			}),
		}
		ddaOpts = append(ddaOpts, defaultDDAOpts...)

		provisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-logs-collection"),
			provisioners.WithK8sVersion(common.K8sVersion),
			provisioners.WithOperatorOptions(DefaultOperatorOpts...),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithLocal(s.local),
		}

		s.UpdateEnv(provisioners.KubernetesProvisioner(provisionerOptions...))
		utils.VerifyAgentPods(t, common.NamespaceName, s.k8sClient, "app.kubernetes.io/instance=datadog-agent-logs-agent")

		// Verify logs collection on agent pod
		s.EventuallyWithTf(func(c *assert.CollectT) {
			agentPods, err := s.k8sClient.CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/instance=datadog-agent-logs-agent"})
			assert.NoError(c, err)

			for _, pod := range agentPods.Items {
				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, pod.Name, "agent", []string{"agent", "status", "logs agent", "-j"})
				assert.NoError(c, err)
				utils.VerifyAgentPodLogs(c, output)
			}
		}, 120*time.Second, 15*time.Second, "could not validate logs status on agent pod")

		s.EventuallyWithTf(func(c *assert.CollectT) {
			verifyAPILogs(s)
		}, 120*time.Second, 15*time.Second, "could not valid logs collection with fake intake client")
	})
}

func verifyAPILogs(s *k8sSuite) {
	logs, err := s.Env().FakeIntake.Client().FilterLogs("agent")
	s.Assert().NoError(err)
	s.Assert().NotEmptyf(logs, fmt.Sprintf("Expected fake intake-ingested logs to not be empty: %s", err))
}

func verifyKSMCheck(s *k8sSuite) {
	metricNames, err := s.Env().FakeIntake.Client().GetMetricNames()
	s.Assert().NoError(err)
	s.Assert().Contains(metricNames, "kubernetes_state.container.running")

	metrics, err := s.Env().FakeIntake.Client().FilterMetrics("kubernetes_state.container.running", matchOpts...)
	s.Assert().NoError(err)
	s.Assert().NotEmptyf(metrics, fmt.Sprintf("expected metric series to not be empty: %s", err))
}

func verifyHTTPCheck(s *k8sSuite) {
	metricNames, err := s.Env().FakeIntake.Client().GetMetricNames()
	s.Assert().NoError(err)
	s.Assert().Contains(metricNames, "network.http.can_connect")
	metrics, err := s.Env().FakeIntake.Client().FilterMetrics("network.http.can_connect")
	s.Assert().NoError(err)
	s.Assert().Greater(len(metrics), 0)
	for _, metric := range metrics {
		for _, points := range metric.Points {
			s.Assert().Greater(points.Value, float64(0))
		}
	}
}

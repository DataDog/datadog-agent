// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/stretchr/testify/assert"
)

var (
	matchTags = []*regexp.Regexp{regexp.MustCompile("kube_container_name:.*")}
	matchOpts = []client.MatchOpt[*aggregator.MetricSeries]{client.WithMatchingTags[*aggregator.MetricSeries](matchTags)}
)

type k8sSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	local bool
}

func (s *k8sSuite) TestGenericK8s() {
	defaultOperatorOpts := []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
		operatorparams.WithHelmValues(`
installCRDs: false`),
	}

	defaultProvisionerOpts := []provisioners.KubernetesProvisionerOption{
		provisioners.WithTestName("generic-k8s"),
		provisioners.WithK8sVersion(common.K8sVersion),
		provisioners.WithOperatorOptions(defaultOperatorOpts...),
		provisioners.WithLocal(s.local),
	}

	defaultDDAOpts := []agentwithoperatorparams.Option{
		agentwithoperatorparams.WithNamespace(common.NamespaceName),
	}

	s.T().Run("Verify Operator", func(t *testing.T) {
		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			res, _ := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{})
			containsOperator := false
			for _, pod := range res.Items {
				if strings.Contains(pod.Name, "datadog-operator") {
					containsOperator = true
					break
				}
			}
			assert.True(s.T(), containsOperator, "Datadog Operator not found")
		}, 300*time.Second, 15*time.Second, "Could not validate operator pod in time")
	})

	s.T().Run("Minimal DDA config", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(common.DdaMinimalPath)
		assert.NoError(s.T(), err)

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
			provisioners.WithOperatorOptions(defaultOperatorOpts...),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithLocal(s.local),
		}

		provisionerOptions = append(provisionerOptions, defaultProvisionerOpts...)

		s.UpdateEnv(provisioners.KubernetesProvisioner(provisionerOptions...))

		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), common.NodeAgentSelector+",agent.datadoghq.com/name=dda-minimum")
			pods, podsListErr := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{})
			assert.NoError(s.T(), podsListErr)
			assert.NotNil(t, pods)
		}, 1*time.Minute, 15*time.Second, "could not validate agent pod in time")

		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			utils.VerifyNumPodsForSelector(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), 1, common.ClusterAgentSelector+",agent.datadoghq.com/name=dda-minimum")
		}, 1*time.Minute, 15*time.Second, "could not validate cluster agent pod in time")

		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			kubeletCheckRun, err := s.Env().FakeIntake.Client().GetCheckRun("kubernetes.kubelet.check")
			assert.NoError(c, err)
			assert.NotEmpty(c, kubeletCheckRun)
			assert.Equal(c, 0, kubeletCheckRun[0].Status, "kubelet check status should be running")

			kubeletMetricSeries, err := s.Env().FakeIntake.Client().FilterMetrics("kubernetes.cpu.usage.total", matchOpts...)
			s.Assert().NoError(err)
			s.Assert().NotEmptyf(kubeletMetricSeries, fmt.Sprintf("expected Kubelet check series to not be empty: %s", err))

		}, 1*time.Minute, 15*time.Second, "could not validate kubelet check in time")

		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			s.verifyKSMCheck(c)
		}, 1*time.Minute, 15*time.Second, "could not validate KSM check in time")

	})

	s.T().Run("KSM check works cluster check runner", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "datadog-agent-ccr-enabled.yaml"))
		assert.NoError(s.T(), err)

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
			provisioners.WithOperatorOptions(defaultOperatorOpts...),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithLocal(s.local),
		}

		s.UpdateEnv(provisioners.KubernetesProvisioner(provisionerOptions...))

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), "app.kubernetes.io/instance=datadog-ccr-enabled-agent")
			utils.VerifyNumPodsForSelector(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), 1, "app.kubernetes.io/instance=datadog-ccr-enabled-cluster-checks-runner")
			s.verifyKSMCheck(c)
		}, 10*time.Minute, 15*time.Second, "could not validate kubernetes_state_core (cluster check on CCR) check in time")
	})

	s.T().Run("Autodiscovery works", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(common.DdaMinimalPath)
		assert.NoError(s.T(), err)

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{Name: "dda-autodiscovery", YamlFilePath: ddaConfigPath}),
		}
		ddaOpts = append(ddaOpts, defaultDDAOpts...)

		provisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-autodiscovery"),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithYAMLWorkload(provisioners.YAMLWorkload{Name: "nginx", Path: strings.Join([]string{common.ManifestsPath, "autodiscovery-annotation.yaml"}, "/")}),
			provisioners.WithLocal(s.local),
		}
		provisionerOptions = append(provisionerOptions, defaultProvisionerOpts...)

		// Add nginx with annotations
		s.UpdateEnv(provisioners.KubernetesProvisioner(provisionerOptions...))

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyNumPodsForSelector(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), 1, "app=nginx")
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), common.NodeAgentSelector+",agent.datadoghq.com/name=dda-autodiscovery")
			s.verifyHTTPCheck(c)
		}, 5*time.Minute, 15*time.Second, "could not validate http_check in time")
	})

	s.T().Run("Logs collection works", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "datadog-agent-logs.yaml"))
		assert.NoError(s.T(), err)

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
			provisioners.WithOperatorOptions(defaultOperatorOpts...),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithLocal(s.local),
		}

		s.UpdateEnv(provisioners.KubernetesProvisioner(provisionerOptions...))

		// Verify logs collection on agent pod
		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), "app.kubernetes.io/instance=datadog-agent-logs-agent")

			agentPods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/instance=datadog-agent-logs-agent"})
			assert.NoError(c, err)

			for _, pod := range agentPods.Items {
				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, pod.Name, "agent", []string{"agent", "status", "logs agent", "-j"})
				assert.NoError(c, err)
				utils.VerifyAgentPodLogs(c, output)
			}

			s.verifyAPILogs()
		}, 5*time.Minute, 15*time.Second, "could not valid logs collection in time")
	})

	s.T().Run("APM hostPort k8s service UDP works", func(t *testing.T) {

		// Cleanup to avoid potential lingering DatadogAgent
		// Avoid race with the new Agent not being able to bind to the hostPort
		withoutDDAProvisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-apm"),
			provisioners.WithoutDDA(),
			provisioners.WithLocal(s.local),
		}
		withoutDDAProvisionerOptions = append(withoutDDAProvisionerOptions, defaultProvisionerOpts...)
		s.UpdateEnv(provisioners.KubernetesProvisioner(withoutDDAProvisionerOptions...))

		var apmAgentSelector = ",agent.datadoghq.com/name=datadog-agent-apm"
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "apm", "datadog-agent-apm.yaml"))
		assert.NoError(s.T(), err)

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "datadog-agent-apm",
				YamlFilePath: ddaConfigPath,
			}),
		}
		ddaOpts = append(ddaOpts, defaultDDAOpts...)

		ddaProvisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-apm"),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithYAMLWorkload(provisioners.YAMLWorkload{
				Name: "tracegen-deploy",
				Path: strings.Join([]string{common.ManifestsPath, "apm", "tracegen-deploy.yaml"}, "/"),
			}),
			provisioners.WithLocal(s.local),
		}
		ddaProvisionerOptions = append(ddaProvisionerOptions, defaultProvisionerOpts...)

		// Deploy APM DatadogAgent and tracegen
		s.UpdateEnv(provisioners.KubernetesProvisioner(ddaProvisionerOptions...))

		// Verify traces collection on agent pod
		s.EventuallyWithTf(func(c *assert.CollectT) {
			// Verify tracegen deployment is running
			utils.VerifyNumPodsForSelector(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), 1, "app=tracegen-tribrid")

			// Verify agent pods are running
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), common.NodeAgentSelector+apmAgentSelector)
			agentPods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: common.NodeAgentSelector + apmAgentSelector, FieldSelector: "status.phase=Running"})
			assert.NoError(c, err)

			// This works because we have a single Agent pod (so located on same node as tracegen)
			// Otherwise, we would need to deploy tracegen on the same node as the Agent pod / as a DaemonSet
			for _, pod := range agentPods.Items {

				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, pod.Name, "agent", []string{"agent", "status", "apm agent", "-j"})
				assert.NoError(c, err)

				utils.VerifyAgentTraces(c, output)
			}

			// Verify traces collection ingestion by fakeintake
			s.verifyAPITraces(c)
		}, 5*time.Minute, 15*time.Second, "could not validate traces on agent pod") // TODO: check duration
	})
}

func (s *k8sSuite) verifyAPILogs() {
	logs, err := s.Env().FakeIntake.Client().FilterLogs("agent")
	s.Assert().NoError(err)
	s.Assert().NotEmptyf(logs, fmt.Sprintf("Expected fake intake-ingested logs to not be empty: %s", err))
}

func (s *k8sSuite) verifyAPITraces(c *assert.CollectT) {
	traces, err := s.Env().FakeIntake.Client().GetTraces()
	assert.NoError(c, err)
	assert.NotEmptyf(c, traces, fmt.Sprintf("Expected fake intake-ingested traces to not be empty: %s", err))
}

func (s *k8sSuite) verifyKSMCheck(c *assert.CollectT) {
	ksmCheckRun, err := s.Env().FakeIntake.Client().GetCheckRun("kubernetes_state.node.ready")
	assert.NoError(c, err)
	require.NotEmpty(c, ksmCheckRun)
	assert.Equal(c, 0, ksmCheckRun[0].Status, "KSM check status should be running")

	metricNames, err := s.Env().FakeIntake.Client().GetMetricNames()
	assert.NoError(c, err)
	assert.Contains(c, metricNames, "kubernetes_state.container.running")

	metrics, err := s.Env().FakeIntake.Client().FilterMetrics("kubernetes_state.container.running", matchOpts...)
	assert.NoError(c, err)
	assert.NotEmptyf(c, metrics, fmt.Sprintf("expected metric series to not be empty: %s", err))
}

func (s *k8sSuite) verifyHTTPCheck(c *assert.CollectT) {
	httpCheckRun, err := s.Env().FakeIntake.Client().GetCheckRun("http.can_connect")
	assert.NoError(c, err)
	require.NotEmpty(c, httpCheckRun)
	assert.Equal(c, 0, httpCheckRun[0].Status, "HTTP check status should be running")

	metricNames, err := s.Env().FakeIntake.Client().GetMetricNames()
	assert.NoError(c, err)
	assert.Contains(c, metricNames, "network.http.can_connect")
	metrics, err := s.Env().FakeIntake.Client().FilterMetrics("network.http.can_connect")
	assert.NoError(c, err)
	assert.Greater(c, len(metrics), 0)
	for _, metric := range metrics {
		for _, points := range metric.Points {
			assert.Greater(c, points.Value, float64(0))
		}
	}
}

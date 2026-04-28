// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package lazyartifacts

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redis"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubecomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	localkubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/local/kubernetes"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
)

const (
	lazySourceImage = "docker.io/alidatadog/agent-latest-full-estargz-poc@sha256:3d4c01e9511a86f4c9fc05cb2cca1ce53f1325107e6e9b8720716d07da8e7bff"
	lazyCacheDir    = "/var/lib/datadog-agent/lazy-artifacts"
)

const lazyPythonHelmValues = `
datadog:
  logLevel: DEBUG
  admissionController:
    enabled: false
  apm:
    instrumentation:
      enabled: false
  autoscaling:
    workload:
      enabled: false
  clusterChecks:
    enabled: false
  helmCheck:
    enabled: false
  kubeStateMetricsCore:
    enabled: false
  prometheusScrape:
    enabled: false
  sbom:
    containerImage:
      enabled: false
    host:
      enabled: false
clusterAgent:
  enabled: false
  admissionController:
    enabled: false
    agentSidecarInjection:
      enabled: false
  metricsProvider:
    enabled: false
clusterChecksRunner:
  enabled: false
agents:
  image:
    pullPolicy: Never
  volumes:
    - name: lazy-artifacts-cache
      emptyDir: {}
  volumeMounts:
    - name: lazy-artifacts-cache
      mountPath: /var/lib/datadog-agent/lazy-artifacts
  containers:
    agent:
      env:
        - name: DD_PYTHON_LAZY_ARTIFACTS_ENABLED
          value: "true"
        - name: DD_PYTHON_LAZY_ARTIFACTS_SOURCE_IMAGE
          value: "` + lazySourceImage + `"
        - name: DD_PYTHON_LAZY_ARTIFACTS_PLATFORM
          value: "linux/amd64"
        - name: DD_PYTHON_LAZY_ARTIFACTS_CACHE_DIR
          value: "` + lazyCacheDir + `"
`

type lazyPythonKindSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestLazyPythonArtifactsLocalKind(t *testing.T) {
	agentImage := os.Getenv("DD_TEST_LAZY_AGENT_IMAGE")
	if agentImage == "" {
		t.Skip("DD_TEST_LAZY_AGENT_IMAGE not set; skipping local kind lazy artifact e2e")
	}

	e2e.Run(t, &lazyPythonKindSuite{}, e2e.WithProvisioner(
		localkubernetes.Provisioner(
			localkubernetes.WithName("lazy-artifacts"),
			localkubernetes.WithKindAPIServerPort(18443),
			localkubernetes.WithKindLoadImage(agentImage),
			localkubernetes.WithAgentOptions(
				kubernetesagentparams.WithAgentFullImagePath(agentImage),
				kubernetesagentparams.WithHelmValues(lazyPythonHelmValues),
			),
			localkubernetes.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubecomp.Workload, error) {
				return redis.K8sAppDefinition(e, kubeProvider, "workload-redis", false, nil)
			}),
		),
	))
}

func (s *lazyPythonKindSuite) TestRedisCheckMaterializedFromLazySourceImage() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			"redis.net.instantaneous_ops_per_sec",
			fakeintake.WithMatchingTags[*aggregator.MetricSeries]([]*regexp.Regexp{
				regexp.MustCompile(`^kube_namespace:workload-redis$`),
			}),
		)
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics, "redisdb metrics were not received yet")
	}, 5*time.Minute, 10*time.Second)

	agentPod := getAgentPod(s.T(), s.Env().KubernetesCluster.Client())

	stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(
		agentPod.Namespace,
		agentPod.Name,
		"agent",
		[]string{"bash", "-c", "test ! -d /opt/datadog-agent/embedded/lib/python3.13/site-packages/datadog_checks/redisdb"},
	)
	assert.NoError(s.T(), err)
	assert.Empty(s.T(), stdout)
	assert.Empty(s.T(), stderr)

	s.EventuallyWithT(func(c *assert.CollectT) {
		stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(
			agentPod.Namespace,
			agentPod.Name,
			"agent",
			[]string{"bash", "-c", "marker=$(find " + lazyCacheDir + " -name .complete.json -print -quit); test -n \"$marker\" && cat \"$marker\""},
		)
		assert.NoError(c, err)
		assert.Empty(c, stderr)

		var marker struct {
			Image      string `json:"image"`
			Capability string `json:"capability"`
			Stats      struct {
				RangeRequests int   `json:"range_requests"`
				RangeBytes    int64 `json:"range_bytes"`
			} `json:"stats"`
		}
		if assert.NoError(c, json.Unmarshal([]byte(stdout), &marker)) {
			assert.Equal(c, lazySourceImage, marker.Image)
			assert.Equal(c, "python-check:redisdb", marker.Capability)
			assert.Greater(c, marker.Stats.RangeRequests, 0)
			assert.Less(c, marker.Stats.RangeBytes, int64(5*1024*1024))
		}
	}, 2*time.Minute, 5*time.Second)
}

func getAgentPod(t assert.TestingT, client kubeclient.Interface) corev1.Pod {
	pods, err := client.CoreV1().Pods("datadog").List(context.Background(), metav1.ListOptions{
		LabelSelector: "app=dda-linux-datadog",
	})
	assert.NoError(t, err)
	assert.NotEmpty(t, pods.Items)

	return pods.Items[0]
}

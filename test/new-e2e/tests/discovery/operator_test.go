// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package discovery

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
)

type operatorDiscoveryTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestDiscoveryOperator(t *testing.T) {
	customDDA := agentwithoperatorparams.DDAConfig{
		Name: "discovery-enabled",
		YamlConfig: `
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
spec:
  global:
    kubelet:
      tlsVerify: false
  features:
    serviceDiscovery:
      enabled: true
  override:
    nodeAgent:
      containers:
        agent:
          env:
            - name: DD_DISCOVERY_ENABLED
              value: "true"
        system-probe:
          env:
            - name: DD_DISCOVERY_ENABLED
              value: "true"
`}

	e2e.Run(t, &operatorDiscoveryTestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(
		awskubernetes.WithOperator(),
		awskubernetes.WithOperatorDDAOptions([]agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(customDDA),
		}...),
	)))
}

func (s *operatorDiscoveryTestSuite) TestDiscoveryOperator() {
	t := s.T()

	// Get agent pod for diagnostics
	client := s.Env().KubernetesCluster.Client()
	agentPods, err := client.CoreV1().Pods("datadog").List(context.Background(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/component=agent",
	})
	require.NoError(t, err)
	require.NotEmpty(t, agentPods.Items, "No agent pods found")
	agentPod := agentPods.Items[0]

	// Create Python server pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "python-http-server",
			Labels: map[string]string{
				"app": "python-server",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "server",
					Image: "python:3",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 8000,
							Name:          "http",
						},
					},
					Command: []string{"python", "-m", "http.server", "8000"},
				},
			},
		},
	}

	// Deploy Python server pod
	_, err = client.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})
	require.NoError(t, err)

	// Wait for pod to be ready
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		podStatus, err := client.CoreV1().Pods("default").Get(context.Background(), "python-http-server", metav1.GetOptions{})
		if !assert.NoError(c, err) {
			return
		}

		if podStatus.Status.Phase == corev1.PodRunning {
			for _, condition := range podStatus.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					t.Log("Python HTTP server pod is ready")
					return
				}
			}
		}

		assert.Fail(c, "Python HTTP server pod is not ready yet")
	}, 2*time.Minute, 10*time.Second)

	// Check services via unix socket
	servicesOutput, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(
		"datadog",
		agentPod.Name,
		"system-probe",
		[]string{"curl", "-s", "--unix-socket", "/var/run/sysprobe/sysprobe.sock", "http://unix/discovery/debug"},
	)
	if err != nil {
		t.Logf("Failed to get services info: %v", err)
	} else {
		t.Logf("Services discovery info: %s", servicesOutput)
		if strings.Contains(servicesOutput, "python") || strings.Contains(servicesOutput, "http.server") {
			t.Log("Python server found in discovery services output")
		}
	}

	// Check service discovery payloads
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		payloads, err := s.Env().FakeIntake.Client().GetServiceDiscoveries()
		if !assert.NoError(c, err) {
			return
		}

		t.Logf("Found %d service discovery payloads", len(payloads))
		for _, p := range payloads {
			t.Logf("Service discovery: RequestType=%s, ServiceName=%s", p.RequestType, p.Payload.ServiceName)
		}

		found := false
		for _, p := range payloads {
			if p.RequestType == "start-service" && p.Payload.ServiceName == "http.server" {
				t.Logf("Found service discovery for http.server: %v", p.Payload)
				found = true
				break
			}
		}

		assert.True(c, found, "Did not find service discovery for Python HTTP server")
	}, 3*time.Minute, 10*time.Second)
}

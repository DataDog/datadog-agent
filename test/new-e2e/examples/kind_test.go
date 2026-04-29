// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/helmagent"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

type myKindSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestMyKindSuite(t *testing.T) {
	// Provisioner creates infrastructure only — KinD cluster + fakeintake.
	// No agent and no workloads in Pulumi (test workloads are disabled
	// via WithoutDeployTestWorkload because they depend on the agent's
	// admission controller, which is now installed post-Pulumi).
	e2e.Run(t, &myKindSuite{}, e2e.WithProvisioner(
		provkindvm.Provisioner(provkindvm.WithRunOptions(scenkindvm.WithoutDeployTestWorkload())),
	))
}

func (v *myKindSuite) SetupSuite() {
	v.BaseSuite.SetupSuite()
	defer v.CleanupOnSetupFailure()

	// Step 1: install the Datadog agent via Helm, decoupled from Pulumi.
	// The fakeintake URL is automatically wired into the agent config.
	//
	// The kindvm-specific values (kubelet.tlsVerify, csi.enabled,
	// useHostNetwork) are required for the agent to function in a KinD
	// cluster — they were previously embedded in the kindvm scenario
	// (agent_helm_values.yaml) and applied automatically.
	helmagent.Install(v.T(), v.Env(),
		kubernetesagentparams.WithHelmValues(`
datadog:
  kubelet:
    tlsVerify: false
  csi:
    enabled: true
  processAgent:
    processCollection: true
  logs:
    enabled: true
    containerCollectAll: true
agents:
  useHostNetwork: true
`),
	)

	// Step 2: deploy nginx workload via the K8s client (no Pulumi).
	// This must run after the agent is installed so the agent can discover
	// the workload via Kubernetes metadata.
	deployNginx(v.T(), v.Env())
}

// deployNginx creates a minimal nginx Deployment and Service via the K8s API.
// The agent's Kubernetes integration will discover this workload automatically
// and emit container/pod metrics.
func deployNginx(t *testing.T, env *environments.Kubernetes) {
	t.Helper()
	ctx := context.Background()
	k8sClient := env.KubernetesCluster.Client()

	// Create namespace
	_, err := k8sClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: v1.ObjectMeta{Name: "nginx"},
	}, v1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		require.NoError(t, err, "failed to create nginx namespace")
	}

	// Create Deployment
	replicas := int32(1)
	_, err = k8sClient.AppsV1().Deployments("nginx").Create(ctx, &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{Name: "nginx", Namespace: "nginx"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &v1.LabelSelector{MatchLabels: map[string]string{"app": "nginx"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{Labels: map[string]string{"app": "nginx"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "nginx",
						Image: "nginx:1.27",
						Ports: []corev1.ContainerPort{{ContainerPort: 80}},
					}},
				},
			},
		},
	}, v1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		require.NoError(t, err, "failed to create nginx deployment")
	}

	// Create Service
	_, err = k8sClient.CoreV1().Services("nginx").Create(ctx, &corev1.Service{
		ObjectMeta: v1.ObjectMeta{Name: "nginx", Namespace: "nginx"},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "nginx"},
			Ports: []corev1.ServicePort{{
				Port:       80,
				TargetPort: intstr.FromInt(80),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}, v1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		require.NoError(t, err, "failed to create nginx service")
	}

	// Wait for the nginx pod to be ready
	require.Eventually(t, func() bool {
		pods, err := k8sClient.CoreV1().Pods("nginx").List(ctx, v1.ListOptions{
			LabelSelector: "app=nginx",
		})
		if err != nil || len(pods.Items) == 0 {
			return false
		}
		for _, pod := range pods.Items {
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					return true
				}
			}
		}
		return false
	}, 2*time.Minute, 5*time.Second, "nginx pod not ready")
}

func (v *myKindSuite) TestClusterAgentInstalled() {
	res, err := v.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.TODO(), v1.ListOptions{})
	require.NoError(v.T(), err)

	containsClusterAgent := false
	for _, pod := range res.Items {
		if strings.Contains(pod.Name, "cluster-agent") {
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					containsClusterAgent = true
					break
				}
			}
		}
	}
	assert.True(v.T(), containsClusterAgent, "Cluster Agent not found or not ready")
}

func (v *myKindSuite) TestFakeIntakeReceivesMetrics() {
	v.EventuallyWithT(func(c *assert.CollectT) {
		metricNames, err := v.Env().FakeIntake.Client().GetMetricNames()
		assert.NoError(c, err)
		assert.Greater(c, len(metricNames), 0, "no metrics received at fakeintake yet")
	}, 5*time.Minute, 10*time.Second)
}

func (v *myKindSuite) TestNginxWorkloadDiscovered() {
	// The agent should discover the nginx pod via Kubernetes metadata
	// and emit container metrics with kube_deployment:nginx tags.
	v.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := v.Env().FakeIntake.Client().FilterMetrics(
			"kubernetes.cpu.usage.total",
		)
		assert.NoError(c, err)
		// Look for at least one metric tagged with our nginx deployment
		found := false
		for _, m := range metrics {
			for _, tag := range m.Tags {
				if tag == "kube_deployment:nginx" {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		assert.True(c, found, "no kubernetes.cpu.usage.total metric with kube_deployment:nginx tag yet")
	}, 5*time.Minute, 15*time.Second)
}

func (v *myKindSuite) TestReconfigureAgent() {
	// Reconfigure the agent via helm upgrade — enable APM
	v.Env().Agent.Configure(v.T(),
		kubernetesagentparams.WithHelmValues(`
datadog:
  apm:
    portEnabled: true
    instrumentation:
      enabled: true
`),
	)

	// Verify the agent pods are still running after reconfiguration
	v.EventuallyWithT(func(c *assert.CollectT) {
		pods, err := v.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(
			context.TODO(), v1.ListOptions{LabelSelector: "app=dda-datadog"},
		)
		assert.NoError(c, err)
		assert.Greater(c, len(pods.Items), 0, "no agent pods found after reconfiguration")

		for _, pod := range pods.Items {
			ready := false
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					ready = true
				}
			}
			assert.True(c, ready, "agent pod %s not ready after reconfiguration", pod.Name)
		}
	}, 5*time.Minute, 10*time.Second)
}

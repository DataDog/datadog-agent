// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package environments

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	agentoutput "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	kubeoutput "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCheckAgentPodsReady(t *testing.T) {
	linuxTarget := podReadinessTarget{
		name:          "linux node agent",
		namespace:     agentNamespace,
		labelSelector: "app=agent",
		nodeSelector:  "kubernetes.io/os=linux",
	}

	t.Run("ready", func(t *testing.T) {
		env := newReadinessTestEnvironment(
			linuxNode("node-1"),
			readyPod("agent-1", "agent", 0),
		)
		require.NoError(t, checkAgentPodsReady(context.Background(), env.KubernetesCluster, []podReadinessTarget{linuxTarget}))
	})

	t.Run("missing pod", func(t *testing.T) {
		env := newReadinessTestEnvironment(linuxNode("node-1"))
		err := checkAgentPodsReady(context.Background(), env.KubernetesCluster, []podReadinessTarget{linuxTarget})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "got 0 pod(s), want 1")
	})

	t.Run("container not ready and restarted", func(t *testing.T) {
		env := newReadinessTestEnvironment(
			linuxNode("node-1"),
			unreadyPod("agent-1", "agent", 2),
		)
		err := checkAgentPodsReady(context.Background(), env.KubernetesCluster, []podReadinessTarget{linuxTarget})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "container agent in pod agent-1 is not ready")
		assert.Contains(t, err.Error(), "restarted 2 time(s)")
	})

	t.Run("minimum replica count", func(t *testing.T) {
		env := newReadinessTestEnvironment()
		err := checkAgentPodsReady(context.Background(), env.KubernetesCluster, []podReadinessTarget{{
			name:          "cluster agent",
			namespace:     agentNamespace,
			labelSelector: "app=cluster-agent",
			minimumCount:  1,
		}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "want at least 1")
	})
}

func TestWaitForKubernetesAgentReady(t *testing.T) {
	// The installation only runs on the nodes of its variant, in its own namespace.
	baselineNode := linuxNode("baseline-node")
	baselineNode.Labels["variant"] = "baseline"
	comparisonNode := linuxNode("comparison-node")
	comparisonNode.Labels["variant"] = "comparison"
	pod := readyPod("agent-1", "agent", 0)
	pod.Namespace = "datadog-baseline"

	cluster := &components.KubernetesCluster{
		KubernetesClient: &client.KubernetesClient{K8sClient: fake.NewSimpleClientset(baselineNode, comparisonNode, pod)},
	}
	agent := &components.KubernetesAgent{
		KubernetesAgentOutput: agentoutput.KubernetesAgentOutput{
			LinuxNodeAgent: kubeoutput.KubernetesObjRefOutput{
				Namespace:      "datadog-baseline",
				LabelSelectors: map[string]string{"app": "agent"},
			},
		},
	}

	t.Run("ready on the selected nodes", func(t *testing.T) {
		require.NoError(t, WaitForKubernetesAgentReady(context.Background(), cluster, agent,
			WithLinuxNodeAgentReady(),
			WithLinuxNodeAgentNodeSelector("variant=baseline"),
			WithAgentReadinessTimeout(time.Second),
		))
	})

	t.Run("missing pod on a node without selector", func(t *testing.T) {
		// The wait times out on the virtual clock of the bubble.
		synctest.Test(t, func(t *testing.T) {
			err := WaitForKubernetesAgentReady(context.Background(), cluster, agent,
				WithLinuxNodeAgentReady(),
				WithAgentReadinessTimeout(time.Second),
			)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "got 1 pod(s), want 2")
		})
	})

	t.Run("nil agent", func(t *testing.T) {
		require.Error(t, WaitForKubernetesAgentReady(context.Background(), cluster, nil, WithLinuxNodeAgentReady()))
	})
}

func TestWithPodsReady(t *testing.T) {
	inNamespace := func(pod *corev1.Pod) *corev1.Pod {
		pod.Namespace = "workload"
		return pod
	}
	option := WithPodsReady("workload generator", "workload", "app=generator", 1)

	t.Run("ready", func(t *testing.T) {
		env := newReadinessTestEnvironment(inNamespace(readyPod("generator-1", "generator", 0)))
		require.NoError(t, env.WaitForAgentReady(context.Background(), option, WithAgentReadinessTimeout(time.Second)))
	})

	t.Run("missing pod", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// A ready pod of the same app in another namespace doesn't count.
			env := newReadinessTestEnvironment(readyPod("generator-1", "generator", 0))
			err := env.WaitForAgentReady(context.Background(), option, WithAgentReadinessTimeout(time.Second))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "workload generator: got 0 pod(s), want at least 1")
		})
	})

	t.Run("restarted", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			env := newReadinessTestEnvironment(inNamespace(readyPod("generator-1", "generator", 1)))
			err := env.WaitForAgentReady(context.Background(), option, WithAgentReadinessTimeout(time.Second))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "container generator in pod generator-1 restarted 1 time(s)")
		})
	})
}

func TestWithAgentRestartsAllowed(t *testing.T) {
	generator := readyPod("generator-1", "generator", 1)
	generator.Namespace = "workload"

	t.Run("restarted Agent ready", func(t *testing.T) {
		env := newReadinessTestEnvironment(linuxNode("node-1"), readyPod("agent-1", "agent", 2))
		require.NoError(t, env.WaitForAgentReady(context.Background(),
			WithLinuxNodeAgentReady(),
			WithAgentRestartsAllowed(),
			WithAgentReadinessTimeout(time.Second),
		))
	})

	t.Run("restarted Agent not ready", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			env := newReadinessTestEnvironment(linuxNode("node-1"), unreadyPod("agent-1", "agent", 2))
			err := env.WaitForAgentReady(context.Background(),
				WithLinuxNodeAgentReady(),
				WithAgentRestartsAllowed(),
				WithAgentReadinessTimeout(time.Second),
			)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "container agent in pod agent-1 is not ready")
			assert.NotContains(t, err.Error(), "restarted")
		})
	})

	t.Run("restarted auxiliary workload", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			env := newReadinessTestEnvironment(linuxNode("node-1"), readyPod("agent-1", "agent", 2), generator)
			err := env.WaitForAgentReady(context.Background(),
				WithLinuxNodeAgentReady(),
				WithPodsReady("workload generator", "workload", "app=generator", 1),
				WithAgentRestartsAllowed(),
				WithAgentReadinessTimeout(time.Second),
			)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "container generator in pod generator-1 restarted 1 time(s)")
			assert.NotContains(t, err.Error(), "agent-1")
		})
	})
}

func newReadinessTestEnvironment(objects ...runtime.Object) *Kubernetes {
	return &Kubernetes{
		KubernetesCluster: &components.KubernetesCluster{
			KubernetesClient: &client.KubernetesClient{K8sClient: fake.NewSimpleClientset(objects...)},
		},
		Agent: &components.KubernetesAgent{
			KubernetesAgentOutput: agentoutput.KubernetesAgentOutput{
				LinuxNodeAgent: kubeoutput.KubernetesObjRefOutput{LabelSelectors: map[string]string{"app": "agent"}},
			},
		},
	}
}

func linuxNode(name string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   name,
		Labels: map[string]string{"kubernetes.io/os": "linux"},
	}}
}

func readyPod(name, app string, restartCount int32) *corev1.Pod {
	return podWithStatus(name, app, true, restartCount)
}

func unreadyPod(name, app string, restartCount int32) *corev1.Pod {
	return podWithStatus(name, app, false, restartCount)
}

func podWithStatus(name, app string, ready bool, restartCount int32) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: agentNamespace,
			Labels:    map[string]string{"app": app},
		},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
			Name:         app,
			Ready:        ready,
			RestartCount: restartCount,
		}}},
	}
}

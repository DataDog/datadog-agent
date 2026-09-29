// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package environments

import (
	"context"
	"testing"

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
		require.NoError(t, env.checkAgentPodsReady(context.Background(), []podReadinessTarget{linuxTarget}))
	})

	t.Run("missing pod", func(t *testing.T) {
		env := newReadinessTestEnvironment(linuxNode("node-1"))
		err := env.checkAgentPodsReady(context.Background(), []podReadinessTarget{linuxTarget})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "got 0 pod(s), want 1")
	})

	t.Run("container not ready and restarted", func(t *testing.T) {
		env := newReadinessTestEnvironment(
			linuxNode("node-1"),
			unreadyPod("agent-1", "agent", 2),
		)
		err := env.checkAgentPodsReady(context.Background(), []podReadinessTarget{linuxTarget})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "container agent in pod agent-1 is not ready")
		assert.Contains(t, err.Error(), "restarted 2 time(s)")
	})

	t.Run("minimum replica count", func(t *testing.T) {
		env := newReadinessTestEnvironment()
		err := env.checkAgentPodsReady(context.Background(), []podReadinessTarget{{
			name:          "cluster agent",
			namespace:     agentNamespace,
			labelSelector: "app=cluster-agent",
			minimumCount:  1,
		}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "want at least 1")
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

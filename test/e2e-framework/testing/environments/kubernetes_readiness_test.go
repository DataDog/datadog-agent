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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	k8stesting "k8s.io/client-go/testing"
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
	nodeA := linuxNode("node-a")
	nodeA.Labels["variant"] = "a"
	nodeB := linuxNode("node-b")
	nodeB.Labels["variant"] = "b"
	pod := readyPod("agent-1", "agent", 0)
	pod.Namespace = "datadog-a"

	env := newReadinessTestEnvironment(nodeA, nodeB, pod)
	env.Agent.LinuxNodeAgent.Namespace = "datadog-a"
	cluster, agent := env.KubernetesCluster, env.Agent

	t.Run("ready on the selected nodes", func(t *testing.T) {
		require.NoError(t, WaitForKubernetesAgentReady(context.Background(), cluster, agent,
			WithLinuxNodeAgentReady(),
			WithLinuxNodeAgentNodeSelector("variant=a"),
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
		err := WaitForKubernetesAgentReady(context.Background(), cluster, nil, WithLinuxNodeAgentReady())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Agent component is not initialized")
	})

	t.Run("nil agent and auxiliary workload", func(t *testing.T) {
		require.NoError(t, WaitForKubernetesAgentReady(context.Background(), cluster, nil,
			WithPodsReady("agent", "datadog-a", "app=agent", 1),
			WithAgentReadinessTimeout(time.Second),
		))
	})

	t.Run("empty node selector", func(t *testing.T) {
		// It must not select no nodes, hence no pods, and succeed.
		err := WaitForKubernetesAgentReady(context.Background(), cluster, agent,
			WithLinuxNodeAgentReady(),
			WithLinuxNodeAgentNodeSelector(""),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "linux node agent: a node selector or a positive minimum pod count is required")
	})
}

func TestWaitForAgentReadyAtTimeout(t *testing.T) {
	t.Run("last check completed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			generator := inWorkloadNamespace(readyPod("generator-1", "generator", 0))
			env := newReadinessTestEnvironment(linuxNode("node-1"), readyPod("agent-1", "agent", 0), generator)
			// The second check, at 10 s (the default poll interval), outlasts the 15-s timeout
			// and the next tick: the list of the pods of the workload, listed first, lasts 12 s.
			// The list of the Agent pods that follows, unready by then, fails if the check is
			// bounded by the timeout.
			fakeClient := env.KubernetesCluster.KubernetesClient.K8sClient.(*fake.Clientset)
			env.KubernetesCluster.KubernetesClient.K8sClient = contextAwareClientset{fakeClient}
			start := time.Now()
			checks := 0
			fakeClient.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
				switch {
				case action.GetNamespace() == "workload":
					checks++
					if time.Since(start) >= 10*time.Second {
						time.Sleep(12 * time.Second)
					}
				case time.Since(start) >= 10*time.Second:
					return true, &corev1.PodList{Items: []corev1.Pod{*unreadyPod("agent-1", "agent", 0)}}, nil
				}
				return false, nil, nil
			})

			err := env.WaitForAgentReady(context.Background(),
				WithLinuxNodeAgentReady(),
				WithPodsReady("workload generator", "workload", "app=generator", 1),
				WithAgentReadinessStableFor(time.Minute),
				WithAgentReadinessTimeout(15*time.Second),
			)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "container agent in pod agent-1 is not ready")
			assert.NotContains(t, err.Error(), "deadline")
			assert.Equal(t, 2, checks, "no check starts after the timeout")
		})
	})

	t.Run("ready at the last check", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			env := newReadinessTestEnvironment(linuxNode("node-1"), unreadyPod("agent-1", "agent", 0))
			start := time.Now()
			env.KubernetesCluster.KubernetesClient.K8sClient.(*fake.Clientset).PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
				if time.Since(start) < 10*time.Second {
					return false, nil, nil
				}
				return true, &corev1.PodList{Items: []corev1.Pod{*readyPod("agent-1", "agent", 0)}}, nil
			})

			err := env.WaitForAgentReady(context.Background(),
				WithLinuxNodeAgentReady(),
				WithAgentReadinessStableFor(time.Minute),
				WithAgentReadinessTimeout(15*time.Second),
			)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "pods did not remain ready for 1m0s")
			assert.NotContains(t, err.Error(), "not ready")
		})
	})
}

func TestWithPodsReady(t *testing.T) {
	option := WithPodsReady("workload generator", "workload", "app=generator", 1)

	t.Run("ready", func(t *testing.T) {
		env := newReadinessTestEnvironment(inWorkloadNamespace(readyPod("generator-1", "generator", 0)))
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

	t.Run("no minimum count", func(t *testing.T) {
		env := newReadinessTestEnvironment()
		err := env.WaitForAgentReady(context.Background(), WithPodsReady("workload generator", "workload", "app=generator", 0))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workload generator: a node selector or a positive minimum pod count is required")
	})

	t.Run("restarted", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			env := newReadinessTestEnvironment(inWorkloadNamespace(readyPod("generator-1", "generator", 1)))
			err := env.WaitForAgentReady(context.Background(), option, WithAgentReadinessTimeout(time.Second))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "container generator in pod generator-1 restarted 1 time(s)")
		})
	})
}

func TestWithAgentRestartsAllowed(t *testing.T) {
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
			generator := inWorkloadNamespace(readyPod("generator-1", "generator", 1))
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

// contextAwareClientset fails the lists of pods whose context is done, like a real clientset
// does, unlike the fake one.
type contextAwareClientset struct{ kubernetes.Interface }

func (c contextAwareClientset) CoreV1() typedcorev1.CoreV1Interface {
	return contextAwareCoreV1{c.Interface.CoreV1()}
}

type contextAwareCoreV1 struct{ typedcorev1.CoreV1Interface }

func (c contextAwareCoreV1) Pods(namespace string) typedcorev1.PodInterface {
	return contextAwarePods{c.CoreV1Interface.Pods(namespace)}
}

type contextAwarePods struct{ typedcorev1.PodInterface }

func (p contextAwarePods) List(ctx context.Context, opts metav1.ListOptions) (*corev1.PodList, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return p.PodInterface.List(ctx, opts)
}

// inWorkloadNamespace moves pod to the namespace of the auxiliary workload of the tests.
func inWorkloadNamespace(pod *corev1.Pod) *corev1.Pod {
	pod.Namespace = "workload"
	return pod
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

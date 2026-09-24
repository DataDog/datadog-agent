// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package environments

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

const agentNamespace = "datadog"

type podReadinessTarget struct {
	name          string
	namespace     string
	labelSelector string
	nodeSelector  string
	minimumCount  int
}

// KubernetesAgentReadinessOption configures WaitForAgentReady.
type KubernetesAgentReadinessOption func(*kubernetesAgentReadinessParams)

type kubernetesAgentReadinessParams struct {
	timeout       time.Duration
	pollInterval  time.Duration
	stableFor     time.Duration
	targets       []podReadinessTarget
	configuration []string
}

// WithAgentReadinessTimeout sets the maximum time WaitForAgentReady waits.
func WithAgentReadinessTimeout(timeout time.Duration) KubernetesAgentReadinessOption {
	return func(params *kubernetesAgentReadinessParams) {
		params.timeout = timeout
	}
}

// WithAgentReadinessStableFor requires all selected pods to remain ready for the given duration.
// This is useful for suites that need the Agent's asynchronous subsystems to finish warming up.
func WithAgentReadinessStableFor(duration time.Duration) KubernetesAgentReadinessOption {
	return func(params *kubernetesAgentReadinessParams) {
		params.stableFor = duration
	}
}

// WithLinuxNodeAgentReady waits for one ready Linux Node Agent pod per non-Fargate Linux node.
func WithLinuxNodeAgentReady() KubernetesAgentReadinessOption {
	return func(params *kubernetesAgentReadinessParams) {
		params.configuration = append(params.configuration, "linux node agent")
	}
}

// WithWindowsNodeAgentReady waits for one ready Windows Node Agent pod per Windows node.
func WithWindowsNodeAgentReady() KubernetesAgentReadinessOption {
	return func(params *kubernetesAgentReadinessParams) {
		params.configuration = append(params.configuration, "windows node agent")
	}
}

// WithClusterAgentReady waits for at least one ready Cluster Agent pod.
func WithClusterAgentReady() KubernetesAgentReadinessOption {
	return func(params *kubernetesAgentReadinessParams) {
		params.configuration = append(params.configuration, "cluster agent")
	}
}

// WithClusterChecksReady waits for at least one ready Cluster Checks Runner pod.
func WithClusterChecksReady() KubernetesAgentReadinessOption {
	return func(params *kubernetesAgentReadinessParams) {
		params.configuration = append(params.configuration, "cluster checks runner")
	}
}

// WithPodsReadyForNodes waits for one ready pod matching labelSelector per node matching nodeSelector.
// It is intended for auxiliary DaemonSets that should be ready alongside the Agent.
func WithPodsReadyForNodes(name, namespace, labelSelector, nodeSelector string) KubernetesAgentReadinessOption {
	return func(params *kubernetesAgentReadinessParams) {
		params.targets = append(params.targets, podReadinessTarget{
			name:          name,
			namespace:     namespace,
			labelSelector: labelSelector,
			nodeSelector:  nodeSelector,
		})
	}
}

// WaitForAgentReady waits for the selected Agent workloads to have their expected pod count,
// with every init and regular container ready and never restarted. At least one workload option
// such as WithLinuxNodeAgentReady or WithClusterAgentReady must be provided.
func (e *Kubernetes) WaitForAgentReady(ctx context.Context, options ...KubernetesAgentReadinessOption) error {
	params := kubernetesAgentReadinessParams{
		timeout:      10 * time.Minute,
		pollInterval: 10 * time.Second,
	}
	for _, option := range options {
		option(&params)
	}

	if e.KubernetesCluster == nil || e.KubernetesCluster.KubernetesClient == nil {
		return fmt.Errorf("Kubernetes client is not initialized")
	}
	if e.Agent == nil {
		return fmt.Errorf("Agent component is not initialized")
	}

	linuxNodeSelector := fields.AndSelectors(
		fields.OneTermEqualSelector("kubernetes.io/os", "linux"),
		fields.OneTermNotEqualSelector("eks.amazonaws.com/compute-type", "fargate"),
	).String()
	windowsNodeSelector := fields.OneTermEqualSelector("kubernetes.io/os", "windows").String()

	for _, component := range params.configuration {
		switch component {
		case "linux node agent":
			params.targets = append(params.targets, podReadinessTarget{
				name:          component,
				namespace:     agentNamespace,
				labelSelector: fields.OneTermEqualSelector("app", e.Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
				nodeSelector:  linuxNodeSelector,
			})
		case "windows node agent":
			params.targets = append(params.targets, podReadinessTarget{
				name:          component,
				namespace:     agentNamespace,
				labelSelector: fields.OneTermEqualSelector("app", e.Agent.WindowsNodeAgent.LabelSelectors["app"]).String(),
				nodeSelector:  windowsNodeSelector,
			})
		case "cluster agent":
			params.targets = append(params.targets, podReadinessTarget{
				name:          component,
				namespace:     agentNamespace,
				labelSelector: fields.OneTermEqualSelector("app", e.Agent.LinuxClusterAgent.LabelSelectors["app"]).String(),
				minimumCount:  1,
			})
		case "cluster checks runner":
			params.targets = append(params.targets, podReadinessTarget{
				name:          component,
				namespace:     agentNamespace,
				labelSelector: fields.OneTermEqualSelector("app", e.Agent.LinuxClusterChecks.LabelSelectors["app"]).String(),
				minimumCount:  1,
			})
		}
	}
	if len(params.targets) == 0 {
		return fmt.Errorf("no Agent workloads selected for readiness checking")
	}
	if params.timeout <= 0 {
		return fmt.Errorf("Agent readiness timeout must be positive")
	}
	if params.pollInterval <= 0 {
		return fmt.Errorf("Agent readiness poll interval must be positive")
	}

	deadline := time.NewTimer(params.timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(params.pollInterval)
	defer ticker.Stop()

	var stableSince time.Time
	var lastErr error
	for {
		err := e.checkAgentPodsReady(ctx, params.targets)
		if err == nil {
			if params.stableFor <= 0 {
				return nil
			}
			if stableSince.IsZero() {
				stableSince = time.Now()
			}
			if time.Since(stableSince) >= params.stableFor {
				return nil
			}
		} else {
			lastErr = err
			stableSince = time.Time{}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for Agent readiness: %w", ctx.Err())
		case <-deadline.C:
			if lastErr == nil {
				lastErr = fmt.Errorf("pods did not remain ready for %s", params.stableFor)
			}
			return fmt.Errorf("Agent did not become ready within %s: %w", params.timeout, lastErr)
		case <-ticker.C:
		}
	}
}

func (e *Kubernetes) checkAgentPodsReady(ctx context.Context, targets []podReadinessTarget) error {
	client := e.KubernetesCluster.Client()
	var problems []string

	for _, target := range targets {
		expectedCount := target.minimumCount
		if target.nodeSelector != "" {
			nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: target.nodeSelector})
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: list nodes: %v", target.name, err))
				continue
			}
			expectedCount = len(nodes.Items)
		}

		pods, err := client.CoreV1().Pods(target.namespace).List(ctx, metav1.ListOptions{LabelSelector: target.labelSelector})
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: list pods: %v", target.name, err))
			continue
		}
		if target.nodeSelector != "" && len(pods.Items) != expectedCount {
			problems = append(problems, fmt.Sprintf("%s: got %d pod(s), want %d", target.name, len(pods.Items), expectedCount))
		} else if target.nodeSelector == "" && len(pods.Items) < expectedCount {
			problems = append(problems, fmt.Sprintf("%s: got %d pod(s), want at least %d", target.name, len(pods.Items), expectedCount))
		}

		for _, pod := range pods.Items {
			statuses := append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...)
			statuses = append(statuses, pod.Status.ContainerStatuses...)
			if len(statuses) == 0 {
				problems = append(problems, fmt.Sprintf("%s: pod %s has no container statuses", target.name, pod.Name))
				continue
			}
			for _, status := range statuses {
				if !status.Ready {
					problems = append(problems, fmt.Sprintf("%s: container %s in pod %s is not ready", target.name, status.Name, pod.Name))
				}
				if status.RestartCount != 0 {
					problems = append(problems, fmt.Sprintf("%s: container %s in pod %s restarted %d time(s)", target.name, status.Name, pod.Name, status.RestartCount))
				}
			}
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

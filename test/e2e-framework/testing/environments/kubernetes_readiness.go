// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package environments

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

const agentNamespace = "datadog"

type podReadinessTarget struct {
	name          string
	namespace     string
	labelSelector string
	nodeSelector  string
	minimumCount  int
	// restartsAllowed accepts pods whose containers restarted, as long as they are ready.
	restartsAllowed bool
}

// KubernetesAgentReadinessOption configures WaitForAgentReady.
type KubernetesAgentReadinessOption func(*kubernetesAgentReadinessParams)

type kubernetesAgentReadinessParams struct {
	timeout           time.Duration
	pollInterval      time.Duration
	stableFor         time.Duration
	linuxNodeSelector string
	// agentRestartsAllowed is the restartsAllowed of the targets of the Agent workloads.
	agentRestartsAllowed bool
	targets              []podReadinessTarget
	configuration        []string
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

// WithLinuxNodeAgentNodeSelector replaces the label selector of the nodes WithLinuxNodeAgentReady
// expects one Linux Node Agent pod per (non-Fargate Linux nodes by default), for an Agent
// installation pinned to a subset of the nodes.
func WithLinuxNodeAgentNodeSelector(nodeSelector string) KubernetesAgentReadinessOption {
	return func(params *kubernetesAgentReadinessParams) {
		params.linuxNodeSelector = nodeSelector
	}
}

// WithAgentRestartsAllowed accepts Agent pods whose containers restarted, as long as they are
// ready, e.g. Node Agents restarting once while their node finishes starting. It doesn't apply
// to the auxiliary workloads of WithPodsReady and WithPodsReadyForNodes, which must still
// never restart.
func WithAgentRestartsAllowed() KubernetesAgentReadinessOption {
	return func(params *kubernetesAgentReadinessParams) {
		params.agentRestartsAllowed = true
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

// WithPodsReady waits for at least minimumCount ready pods matching labelSelector in namespace.
// It is intended for auxiliary workloads, e.g. a Deployment generating the workload the Agent
// monitors, that should be ready alongside the Agent.
func WithPodsReady(name, namespace, labelSelector string, minimumCount int) KubernetesAgentReadinessOption {
	return func(params *kubernetesAgentReadinessParams) {
		params.targets = append(params.targets, podReadinessTarget{
			name:          name,
			namespace:     namespace,
			labelSelector: labelSelector,
			minimumCount:  minimumCount,
		})
	}
}

// WaitForAgentReady waits for the selected Agent workloads to have their expected pod count,
// with every init and regular container ready and never restarted, unless WithAgentRestartsAllowed
// is given. At least one workload option such as WithLinuxNodeAgentReady or WithClusterAgentReady
// must be provided.
func (e *Kubernetes) WaitForAgentReady(ctx context.Context, options ...KubernetesAgentReadinessOption) error {
	return waitForAgentReady(ctx, e.KubernetesCluster, e.Agent, agentNamespace, options...)
}

// WaitForKubernetesAgentReady is like Kubernetes.WaitForAgentReady, for an Agent installation
// that isn't the Agent of a Kubernetes environment, e.g. one of several installations of the
// same cluster. Its workloads are looked up in the namespace of the installation.
func WaitForKubernetesAgentReady(ctx context.Context, cluster *components.KubernetesCluster, agent *components.KubernetesAgent, options ...KubernetesAgentReadinessOption) error {
	if agent == nil {
		return fmt.Errorf("Agent component is not initialized")
	}
	return waitForAgentReady(ctx, cluster, agent, agent.LinuxNodeAgent.Namespace, options...)
}

// waitForAgentReady waits for the selected workloads of agent, installed in namespace of
// cluster, to be ready.
func waitForAgentReady(ctx context.Context, cluster *components.KubernetesCluster, agent *components.KubernetesAgent, namespace string, options ...KubernetesAgentReadinessOption) error {
	params := kubernetesAgentReadinessParams{
		timeout:      10 * time.Minute,
		pollInterval: 10 * time.Second,
		linuxNodeSelector: fields.AndSelectors(
			fields.OneTermEqualSelector("kubernetes.io/os", "linux"),
			fields.OneTermNotEqualSelector("eks.amazonaws.com/compute-type", "fargate"),
		).String(),
	}
	for _, option := range options {
		option(&params)
	}

	if cluster == nil || cluster.KubernetesClient == nil {
		return fmt.Errorf("Kubernetes client is not initialized")
	}
	if agent == nil {
		return fmt.Errorf("Agent component is not initialized")
	}

	windowsNodeSelector := fields.OneTermEqualSelector("kubernetes.io/os", "windows").String()

	for _, component := range params.configuration {
		switch component {
		case "linux node agent":
			params.targets = append(params.targets, podReadinessTarget{
				name:            component,
				namespace:       namespace,
				labelSelector:   fields.OneTermEqualSelector("app", agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
				nodeSelector:    params.linuxNodeSelector,
				restartsAllowed: params.agentRestartsAllowed,
			})
		case "windows node agent":
			params.targets = append(params.targets, podReadinessTarget{
				name:            component,
				namespace:       namespace,
				labelSelector:   fields.OneTermEqualSelector("app", agent.WindowsNodeAgent.LabelSelectors["app"]).String(),
				nodeSelector:    windowsNodeSelector,
				restartsAllowed: params.agentRestartsAllowed,
			})
		case "cluster agent":
			params.targets = append(params.targets, podReadinessTarget{
				name:            component,
				namespace:       namespace,
				labelSelector:   fields.OneTermEqualSelector("app", agent.LinuxClusterAgent.LabelSelectors["app"]).String(),
				minimumCount:    1,
				restartsAllowed: params.agentRestartsAllowed,
			})
		case "cluster checks runner":
			params.targets = append(params.targets, podReadinessTarget{
				name:            component,
				namespace:       namespace,
				labelSelector:   fields.OneTermEqualSelector("app", agent.LinuxClusterChecks.LabelSelectors["app"]).String(),
				minimumCount:    1,
				restartsAllowed: params.agentRestartsAllowed,
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

	waitCtx, cancel := context.WithTimeout(ctx, params.timeout)
	defer cancel()
	ticker := time.NewTicker(params.pollInterval)
	defer ticker.Stop()

	var stableSince time.Time
	var lastErr error
	for checked := false; ; checked = true {
		err := checkAgentPodsReady(waitCtx, cluster, params.targets)
		if err == nil {
			lastErr = nil
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
			lastErr = latestReadinessError(lastErr, err, checked && waitCtx.Err() != nil)
			stableSince = time.Time{}
		}

		select {
		case <-waitCtx.Done():
			if ctx.Err() != nil {
				return fmt.Errorf("waiting for Agent readiness: %w", ctx.Err())
			}
			if lastErr == nil {
				lastErr = fmt.Errorf("pods did not remain ready for %s", params.stableFor)
			}
			return fmt.Errorf("Agent did not become ready within %s: %w", params.timeout, lastErr)
		case <-ticker.C:
		}
	}
}

// readinessError is the result of a readiness check that failed.
type readinessError struct {
	// problems are those of the pods.
	problems []string
	// requestErrors are the errors of the requests to the API server.
	requestErrors []string
}

func (e *readinessError) Error() string {
	return strings.Join(append(append([]string{}, e.problems...), e.requestErrors...), "; ")
}

// latestReadinessError returns the error to report for the latest failed check, err, given
// that of the previous check, previous. The errors of the requests of a check cut short by
// the deadline, e.g. those of the rate limiter of the client, say nothing about the pods:
// then only the problems of the pods it found replace the result of the previous check.
func latestReadinessError(previous, err error, cutShort bool) error {
	if !cutShort {
		return err
	}
	var readiness *readinessError
	if errors.As(err, &readiness) && len(readiness.problems) > 0 {
		return &readinessError{problems: readiness.problems}
	}
	return previous
}

func checkAgentPodsReady(ctx context.Context, cluster *components.KubernetesCluster, targets []podReadinessTarget) error {
	client := cluster.Client()
	var problems, requestErrors []string

	for _, target := range targets {
		expectedCount := target.minimumCount
		if target.nodeSelector != "" {
			nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: target.nodeSelector})
			if err != nil {
				requestErrors = append(requestErrors, fmt.Sprintf("%s: list nodes: %v", target.name, err))
				continue
			}
			expectedCount = len(nodes.Items)
		}

		pods, err := client.CoreV1().Pods(target.namespace).List(ctx, metav1.ListOptions{LabelSelector: target.labelSelector})
		if err != nil {
			requestErrors = append(requestErrors, fmt.Sprintf("%s: list pods: %v", target.name, err))
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
				if status.RestartCount != 0 && !target.restartsAllowed {
					problems = append(problems, fmt.Sprintf("%s: container %s in pod %s restarted %d time(s)", target.name, status.Name, pod.Name, status.RestartCount))
				}
			}
		}
	}

	if len(problems) > 0 || len(requestErrors) > 0 {
		return &readinessError{problems: problems, requestErrors: requestErrors}
	}
	return nil
}

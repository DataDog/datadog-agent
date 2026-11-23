// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package environments

import (
	"cmp"
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// agentNamespace is the namespace of the Agent workloads whose namespace is unknown.
const agentNamespace = "datadog"

// readinessCheckTimeout bounds a readiness check, which lists the nodes and the pods of every
// target, in case the API server hangs.
const readinessCheckTimeout = time.Minute

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

// WithAgentReadinessTimeout sets the time after which WaitForAgentReady starts no new check. The
// check in progress at the timeout completes, within a minute, and its result stands, even a
// success: it reports the state of the pods rather than requests interrupted by the timeout.
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

// WithPodsReady waits for at least minimumCount pods matching labelSelector in namespace, all of
// them ready, like the Cluster Agent pods of WithClusterAgentReady: an unready pod of a rollout
// still fails the check. It is intended for auxiliary workloads, e.g. a Deployment generating the
// workload the Agent monitors, that should be ready alongside the Agent.
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
	return WaitForKubernetesAgentReady(ctx, e.KubernetesCluster, e.Agent, options...)
}

// WaitForKubernetesAgentReady is like Kubernetes.WaitForAgentReady, for an Agent installation
// that isn't the Agent of a Kubernetes environment, e.g. one of several installations of the
// same cluster. agent may be nil when only auxiliary workloads are selected.
func WaitForKubernetesAgentReady(ctx context.Context, cluster *components.KubernetesCluster, agent *components.KubernetesAgent, options ...KubernetesAgentReadinessOption) error {
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
	if agent == nil && len(params.configuration) > 0 {
		return fmt.Errorf("Agent component is not initialized")
	}

	windowsNodeSelector := fields.OneTermEqualSelector("kubernetes.io/os", "windows").String()

	for _, component := range params.configuration {
		target := podReadinessTarget{name: component, restartsAllowed: params.agentRestartsAllowed}
		var workload kubernetes.KubernetesObjRefOutput
		switch component {
		case "linux node agent":
			workload, target.nodeSelector = agent.LinuxNodeAgent, params.linuxNodeSelector
		case "windows node agent":
			workload, target.nodeSelector = agent.WindowsNodeAgent, windowsNodeSelector
		case "cluster agent":
			workload, target.minimumCount = agent.LinuxClusterAgent, 1
		case "cluster checks runner":
			workload, target.minimumCount = agent.LinuxClusterChecks, 1
		}
		// Each workload is looked up in the namespace it was installed in, when known.
		target.namespace = cmp.Or(workload.Namespace, agentNamespace)
		target.labelSelector = fields.OneTermEqualSelector("app", workload.LabelSelectors["app"]).String()
		params.targets = append(params.targets, target)
	}
	if len(params.targets) == 0 {
		return fmt.Errorf("no Agent workloads selected for readiness checking")
	}
	for _, target := range params.targets {
		// Without a node selector nor a positive minimum count, a target would be satisfied
		// by no pods at all.
		if target.nodeSelector == "" && target.minimumCount < 1 {
			return fmt.Errorf("%s: a node selector or a positive minimum pod count is required", target.name)
		}
	}
	if params.timeout <= 0 {
		return fmt.Errorf("Agent readiness timeout must be positive")
	}
	if params.pollInterval <= 0 {
		return fmt.Errorf("Agent readiness poll interval must be positive")
	}

	// waitCtx only bounds the wait between the checks.
	waitCtx, cancel := context.WithTimeout(ctx, params.timeout)
	defer cancel()
	ticker := time.NewTicker(params.pollInterval)
	defer ticker.Stop()

	var stableSince time.Time
	for {
		// A check isn't bounded by what remains before the timeout: one cut short by the timeout
		// would report the errors of its requests, e.g. "client rate limiter Wait returned an
		// error: context deadline exceeded", instead of the problems of the pods.
		checkCtx, cancelCheck := context.WithTimeout(ctx, readinessCheckTimeout)
		err := checkAgentPodsReady(checkCtx, cluster, params.targets)
		cancelCheck()
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
			stableSince = time.Time{}
		}

		select {
		case <-waitCtx.Done():
		case <-ticker.C:
		}
		// The cancellation of ctx prevails over the timeout, which prevails over a tick that a
		// check outlasting both left pending.
		if ctx.Err() != nil {
			return fmt.Errorf("waiting for Agent readiness: %w", ctx.Err())
		}
		if waitCtx.Err() != nil {
			if err == nil {
				err = fmt.Errorf("pods did not remain ready for %s", params.stableFor)
			}
			return fmt.Errorf("Agent did not become ready within %s: %w", params.timeout, err)
		}
	}
}

func checkAgentPodsReady(ctx context.Context, cluster *components.KubernetesCluster, targets []podReadinessTarget) error {
	client := cluster.Client()
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
				if status.RestartCount != 0 && !target.restartsAllowed {
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

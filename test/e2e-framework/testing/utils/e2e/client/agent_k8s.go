// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package client

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

type agentK8sexecutor struct {
	pod           corev1.Pod
	clusterClient *KubernetesClient
}

var _ agentCommandExecutor = &agentK8sexecutor{}

const agentNamespace = "datadog"

// AgentSelectorAnyPod creates a selector for any pod that runs the agent of the given type.
// For example, you can pass Agent.LinuxNodeAgent to select any pod that runs the node agent.
func AgentSelectorAnyPod(agentType kubernetes.KubernetesObjRefOutput) metav1.ListOptions {
	return metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", agentType.LabelSelectors["app"]).String(),
		Limit:         1,
	}
}

// newAgentK8sExecutor creates a new agentK8sexecutor for the given selector and cluster client. Note that
// the selector must return a single pod, otherwise this function will panic.
func newAgentK8sExecutor(selector metav1.ListOptions, clusterClient *KubernetesClient) (*agentK8sexecutor, error) {
	// Find this specific pod object in the cluster
	pods, err := clusterClient.K8sClient.CoreV1().Pods(agentNamespace).List(context.Background(), selector)
	if err != nil {
		return nil, err
	}

	// We might have 0 pods returned but a continue token, so we need to query more pods
	// again until we get a single pod or we run out of pods.
	for len(pods.Items) == 0 && pods.Continue != "" {
		selector.Continue = pods.Continue
		pods, err = clusterClient.K8sClient.CoreV1().Pods(agentNamespace).List(context.Background(), selector)
		if err != nil {
			return nil, err
		}
	}

	if len(pods.Items) != 1 {
		return nil, fmt.Errorf("Expected to find a single pod, got %d", len(pods.Items))
	}

	pod := pods.Items[0]

	return &agentK8sexecutor{
		pod:           pod,
		clusterClient: clusterClient,
	}, nil
}

func (ae agentK8sexecutor) execute(arguments []string) (string, error) {
	// We consider that in a container, "agent" is always in path and is the single entrypoint.
	// It's mostly incorrect but it's what we have ATM.
	// TODO: Support all agents and Windows
	arguments = append([]string{"agent"}, arguments...)
	stdout, stderr, err := ae.clusterClient.PodExec(agentNamespace, ae.pod.Name, "agent", arguments)

	if err != nil {
		return "", err
	}

	// Return joined stdout and stderr, same as Docker.ExecuteCommandWithErr
	return stdout + " " + stderr, nil
}

func (ae agentK8sexecutor) restart() error {
	return ae.clusterClient.K8sClient.CoreV1().Pods(agentNamespace).Delete(context.Background(), ae.pod.Name, metav1.DeleteOptions{})
}

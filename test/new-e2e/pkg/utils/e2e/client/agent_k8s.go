// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package client

import (
	"context"

	"github.com/DataDog/test-infra-definitions/components/kubernetes"

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
const podSelectorField = "app"

func newAgentK8sExecutor(k8sAgentPod *kubernetes.KubernetesObjRefOutput, clusterClient *KubernetesClient) *agentK8sexecutor {
	// Find this specific pod object in the cluster
	pods, err := clusterClient.K8sClient.CoreV1().Pods(agentNamespace).List(context.Background(), metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", k8sAgentPod.Name).String(),
		Limit:         1,
	})
	if err != nil {
		panic(err)
	}

	if len(pods.Items) != 1 {
		panic("Expected to find a single pod")
	}

	pod := pods.Items[0]

	return &agentK8sexecutor{
		pod:           pod,
		clusterClient: clusterClient,
	}
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

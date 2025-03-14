// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"context"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/kubernetes"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/clientcmd"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/k8sexecuteparams"
)

type agentK8sExecutor struct {
	client             *KubernetesClient
	agentPodName       string
	agentNamespace     string
	agentLinuxSelector map[string]string
	envVar             map[string]string
}

var _ agentCommandExecutor = &agentK8sExecutor{}

func newAgentK8sExecutor(_ common.Context, kubernetesClusterOutput kubernetes.ClusterOutput, k8sAgentOutput agent.KubernetesAgentOutput) *agentK8sExecutor {
	k8sConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubernetesClusterOutput.KubeConfig))
	if err != nil {
		panic(err)
	}
	k8sClient, err := NewKubernetesClient(k8sConfig)
	if err != nil {
		panic(err)
	}
	return &agentK8sExecutor{
		client:             k8sClient,
		agentPodName:       k8sAgentOutput.LinuxNodeAgent.Name,
		agentNamespace:     k8sAgentOutput.LinuxNodeAgent.Namespace,
		agentLinuxSelector: k8sAgentOutput.LinuxNodeAgent.LabelSelectors,
	}
}

func (ae agentK8sExecutor) execute(arguments []string) (string, error) {
	// We consider that in a container, "agent" is always in path and is the single entrypoint.
	// It's mostly incorrect but it's what we have ATM.
	// TODO: Support all agents and Windows
	arguments = append([]string{"agent"}, arguments...)
	agentPods, err := ae.client.K8sClient.CoreV1().Pods(ae.agentNamespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", ae.agentLinuxSelector["app"]).String(),
	})
	if err != nil {
		return "", err
	}
	combinedStdout := ""
	for _, pod := range agentPods.Items {
		stdout, _, err := ae.client.PodExec(ae.agentNamespace, pod.Name, "agent", arguments, k8sexecuteparams.WithEnvVariables(ae.envVar))
		if err != nil {
			return "", err
		}
		combinedStdout += stdout
	}
	if err != nil {
		return "", err
	}
	return combinedStdout, nil
}

func (ae *agentK8sExecutor) useEnvVars(envVars map[string]string) {
	ae.envVar = envVars
}

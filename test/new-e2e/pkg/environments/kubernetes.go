// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package environments

import (
	"context"
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/fields"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

// Kubernetes is an environment that contains a Kubernetes cluster, the Agent and a FakeIntake.
type Kubernetes struct {
	// Components
	KubernetesCluster *components.KubernetesCluster
	FakeIntake        *components.FakeIntake
	Agent             *components.KubernetesAgent
}

// Init initializes the Kubernetes environment.
func (e *Kubernetes) Init(ctx common.Context) error {
	var err error
	if e.Agent != nil {
		e.Agent.Client, err = client.NewK8sAgentClient(ctx, e.KubernetesCluster.ClusterOutput, e.Agent.KubernetesAgentOutput)
		if err != nil {
			return fmt.Errorf("failed to create k8s agent client: %w", err)
		}
	}
	return nil
}

// SetupCoverage generates the coverage and returns the path to the coverage folder.
func (e *Kubernetes) SetupCoverage() (string, error) {
	if e.Agent == nil || e.KubernetesCluster == nil {
		return "", fmt.Errorf("Agent or KubernetesCluster component is not initialized, cannot create coverage folder")
	}
	stdout, err := e.Agent.Client.CoverageWithError(agentclient.WithArgs([]string{"generate"}))
	if err != nil {
		return "", fmt.Errorf("failed to setup coverage: %w", err)
	}

	// find coverage folder in command output
	re := regexp.MustCompile(`(?m)Coverage written to (.+)$`)
	matches := re.FindStringSubmatch(stdout)
	if len(matches) < 2 {
		return "", fmt.Errorf("output does not contain the path to the coverage folder, output: %s", stdout)
	}
	e.Agent.Client.UseEnvVars(map[string]string{
		"GOCOVERDIR": matches[1],
	})
	return matches[1], nil
}

// Coverage collects the coverage from the Agent pods and saves it to the given directory.
func (e *Kubernetes) Coverage(remoteCoverageDir, outputDir string) error {
	if e.Agent == nil || e.KubernetesCluster == nil {
		return fmt.Errorf("Agent or KubernetesCluster component is not initialized, cannot create coverage folder")
	}
	_, err := e.Agent.Client.CoverageWithError(agentclient.WithArgs([]string{"generate"}))
	if err != nil {
		return fmt.Errorf("failed to generate coverage: %w", err)
	}

	agentPods, err := e.KubernetesCluster.Client().CoreV1().Pods(e.Agent.LinuxNodeAgent.Namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", e.Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
	})
	if err != nil {
		return fmt.Errorf("failed to list agent pods: %w", err)
	}
	for _, pod := range agentPods.Items {
		err = e.KubernetesCluster.KubernetesClient.DownloadFromPod(e.Agent.KubernetesAgentOutput.LinuxNodeAgent.Namespace, pod.Name, "agent", remoteCoverageDir, outputDir)
		if err != nil {
			return fmt.Errorf("failed to download coverage: %w", err)
		}
	}
	return nil
}

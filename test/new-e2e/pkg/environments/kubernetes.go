// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package environments

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/common"
)

// Kubernetes is an environment that contains a Kubernetes cluster, the Agent and a FakeIntake.
type Kubernetes struct {
	// Components
	KubernetesCluster *components.KubernetesCluster
	FakeIntake        *components.FakeIntake
	Agent             *components.KubernetesAgent
}

var _ common.Diagnosable = (*Kubernetes)(nil)

// Diagnose generates a diagnose for the Kubernetes environment, creating a flare for each agent and cluster-agent pod.
func (e *Kubernetes) Diagnose(outputDir string) (string, error) {
	fmt.Println("Kubernetes Diagnose will be written to", outputDir)
	diagnoseOutput := []string{"==== Kubernetes Diagnose ===="}
	if e.KubernetesCluster == nil {
		return "", fmt.Errorf("KubernetesCluster component is not initialized")
	}
	if e.Agent == nil {
		return "", fmt.Errorf("Agent component is not initialized")
	}
	if e.KubernetesCluster.KubernetesClient == nil {
		return "", fmt.Errorf("KubernetesClient component is not initialized")
	}
	ctx := context.Background()

	diagnoseOutput = append(diagnoseOutput, "==== Linux pods ====")
	linuxPods, err := e.KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", e.Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
	})
	if err != nil {
		diagnoseOutput = append(diagnoseOutput, fmt.Sprintf("Failed to list linux pods: %s\n", err.Error()))
	} else {
		if len(linuxPods.Items) == 0 {
			diagnoseOutput = append(diagnoseOutput, "No linux pods found")
			return strings.Join(diagnoseOutput, "\n"), nil
		}

		for _, pod := range linuxPods.Items {
			diagnoseOutput = append(diagnoseOutput, fmt.Sprintf("Pod %s:\n", pod.Name))
			flarePath, err := e.generateAndDownloadAgentFlare("agent", pod, "agent", outputDir)
			if err != nil {
				diagnoseOutput = append(diagnoseOutput, fmt.Sprintf("Failed to generate and download agent flare: %s\n", err.Error()))
				continue
			}
			diagnoseOutput = append(diagnoseOutput, fmt.Sprintf("Downloaded flare: %s", flarePath))
		}
	}

	diagnoseOutput = append(diagnoseOutput, "==== Windows pods ====")
	windowsPods, err := e.KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", e.Agent.WindowsNodeAgent.LabelSelectors["app"]).String(),
	})
	if err != nil {
		diagnoseOutput = append(diagnoseOutput, fmt.Sprintf("Failed to list windows pods: %s\n", err.Error()))
	} else {
		if len(windowsPods.Items) == 0 {
			diagnoseOutput = append(diagnoseOutput, "No windows pods found")
		}

		for _, pod := range windowsPods.Items {
			diagnoseOutput = append(diagnoseOutput, fmt.Sprintf("Pod %s:\n", pod.Name))
			flarePath, err := e.generateAndDownloadAgentFlare("agent.exe", pod, "agent", outputDir)
			if err != nil {
				diagnoseOutput = append(diagnoseOutput, fmt.Sprintf("Failed to generate and download agent flare: %s\n", err.Error()))
				continue
			}
			diagnoseOutput = append(diagnoseOutput, fmt.Sprintf("Downloaded flare: %s", flarePath))
		}
	}

	diagnoseOutput = append(diagnoseOutput, "==== Cluster Agent pod ====")
	cluserAgentPods, err := e.KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", e.Agent.LinuxClusterAgent.LabelSelectors["app"]).String(),
	})
	if err != nil {
		diagnoseOutput = append(diagnoseOutput, fmt.Sprintf("Failed to list cluster agent pods: %s\n", err.Error()))
	} else {
		if len(cluserAgentPods.Items) == 0 {
			diagnoseOutput = append(diagnoseOutput, "No cluster agent pods found")
		}

		for _, pod := range cluserAgentPods.Items {
			diagnoseOutput = append(diagnoseOutput, fmt.Sprintf("Pod %s:\n", pod.Name))
			flarePath, err := e.generateAndDownloadAgentFlare("datadog-cluster-agent", pod, "cluster-agent", outputDir)
			if err != nil {
				diagnoseOutput = append(diagnoseOutput, fmt.Sprintf("Failed to generate and download cluster agent flare: %s\n", err.Error()))
				continue
			}
			diagnoseOutput = append(diagnoseOutput, fmt.Sprintf("Downloaded flare: %s", flarePath))
		}
	}

	return strings.Join(diagnoseOutput, "\n"), nil
}

func (e *Kubernetes) generateAndDownloadAgentFlare(agentBinary string, pod v1.Pod, container string, outputDir string) (string, error) {
	stdout, stderr, err := e.KubernetesCluster.KubernetesClient.PodExec(pod.Namespace, pod.Name, container, []string{agentBinary, "flare", "--email", "e2e-tests@datadog-agent", "--send"})
	flareOutput := strings.Join([]string{stdout, stderr}, "\n")
	if err != nil {
		flareOutput = fmt.Sprintf("%s\n%s", flareOutput, err.Error())
	}
	// find <path to flare>.zip in flare command output
	// (?m) is a flag that allows ^ and $ to match the beginning and end of each line
	re := regexp.MustCompile(`(?m)^(.+\.zip) is going to be uploaded to Datadog$`)
	matches := re.FindStringSubmatch(flareOutput)
	if len(matches) < 2 {
		return "", fmt.Errorf("Failed to find flare path in output: %s", flareOutput)
	}
	flarePath := matches[1]
	err = e.KubernetesCluster.KubernetesClient.DownloadFromPod(pod.Namespace, pod.Name, container, flarePath, strings.Join([]string{outputDir, pod.Name}, "/"))
	if err != nil {
		return "", fmt.Errorf("failed to download flare archive: %w", err)
	}
	return flarePath, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package environments

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

// Kubernetes is an environment that contains a Kubernetes cluster, the Agent and a FakeIntake.
type Kubernetes struct {
	// Components
	KubernetesCluster *components.KubernetesCluster
	FakeIntake        *components.FakeIntake
	Agent             *components.KubernetesAgent
}

// Ensure Kubernetes implements the KubernetesOutputs interface
var _ outputs.KubernetesOutputs = (*Kubernetes)(nil)

var _ common.Diagnosable = (*Kubernetes)(nil)

// KubernetesClusterOutput implements eks.KubernetesOutputs
func (e *Kubernetes) KubernetesClusterOutput() *kubernetes.ClusterOutput {
	if e.KubernetesCluster == nil {
		e.KubernetesCluster = &components.KubernetesCluster{}
	}
	return &e.KubernetesCluster.ClusterOutput
}

// FakeIntakeOutput implements eks.KubernetesOutputs
func (e *Kubernetes) FakeIntakeOutput() *fakeintake.FakeintakeOutput {
	if e.FakeIntake == nil {
		e.FakeIntake = &components.FakeIntake{}
	}
	return &e.FakeIntake.FakeintakeOutput
}

// KubernetesAgentOutput implements eks.KubernetesOutputs
func (e *Kubernetes) KubernetesAgentOutput() *agent.KubernetesAgentOutput {
	if e.Agent == nil {
		e.Agent = &components.KubernetesAgent{}
	}
	return &e.Agent.KubernetesAgentOutput
}

// DisableFakeIntake implements eks.KubernetesOutputs
func (e *Kubernetes) DisableFakeIntake() {
	e.FakeIntake = nil
}

// DisableAgent implements eks.KubernetesOutputs
func (e *Kubernetes) DisableAgent() {
	e.Agent = nil
}

// Diagnose generates a diagnose for the Kubernetes environment, creating a flare for each agent and cluster-agent pod.
func (e *Kubernetes) Diagnose(outputDir string) (string, error) {
	fmt.Println("Kubernetes Diagnose will be written to", outputDir)
	diagnoseOutput := []string{"==== Kubernetes Diagnose ===="}
	if e.KubernetesCluster == nil {
		return "", errors.New("KubernetesCluster component is not initialized")
	}
	if e.Agent == nil {
		return "", errors.New("Agent component is not initialized")
	}
	if e.KubernetesCluster.KubernetesClient == nil {
		return "", errors.New("KubernetesClient component is not initialized")
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
			diagnoseOutput = append(diagnoseOutput, "Downloaded flare: "+flarePath)
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
			diagnoseOutput = append(diagnoseOutput, "Downloaded flare: "+flarePath)
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
			diagnoseOutput = append(diagnoseOutput, "Downloaded flare: "+flarePath)
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

type podType int

const (
	podTypeLinux podType = iota
	podTypeWindows
	podTypeClusterAgent
)

// Should return the coverage commands for each pod and each container
func (e *Kubernetes) getAgentCoverageCommands(podType podType) []CoverageTargetSpec {
	if podType == podTypeWindows {
		return []CoverageTargetSpec{
			{
				AgentName:       "agent",
				CoverageCommand: []string{"agent.exe", "coverage", "generate"},
				Required:        true,
			},
			{
				AgentName:       "trace-agent",
				CoverageCommand: []string{"trace-agent.exe", "coverage", "generate"},
				Required:        false,
			},
			{
				AgentName:       "process-agent",
				CoverageCommand: []string{"process-agent.exe", "coverage", "generate"},
				Required:        false,
			},
			{
				AgentName:       "security-agent",
				CoverageCommand: []string{"security-agent.exe", "coverage", "generate"},
				Required:        false,
			},
			{
				AgentName:       "system-probe",
				CoverageCommand: []string{"system-probe.exe", "coverage", "generate"},
				Required:        false,
			},
		}
	} else if podType == podTypeClusterAgent {
		return []CoverageTargetSpec{
			{
				AgentName:       "cluster-agent",
				CoverageCommand: []string{"datadog-cluster-agent", "coverage", "generate"},
				Required:        true,
			},
		}
	}
	return []CoverageTargetSpec{
		{
			AgentName:       "agent",
			CoverageCommand: []string{"agent", "coverage", "generate"},
			Required:        true,
		},
		{
			AgentName:       "trace-agent",
			CoverageCommand: []string{"trace-agent", "coverage", "generate", "-c", "/etc/datadog-agent/datadog.yaml"},
			Required:        false,
		},
		{
			AgentName:       "process-agent",
			CoverageCommand: []string{"process-agent", "coverage", "generate"},
			Required:        false,
		},
		{
			AgentName:       "security-agent",
			CoverageCommand: []string{"security-agent", "coverage", "generate"},
			Required:        false,
		},
		{
			AgentName:       "system-probe",
			CoverageCommand: []string{"system-probe", "coverage", "generate"},
			Required:        false,
		},
	}
}

// Coverage generates a coverage report for each pod and container
func (e *Kubernetes) Coverage(outputDir string) (string, error) {
	if e.KubernetesCluster == nil {
		return "KubernetesCluster component is not initialized, skipping coverage", nil
	}
	if e.Agent == nil {
		return "Agent component is not initialized, skipping coverage", nil
	}
	if e.KubernetesCluster.KubernetesClient == nil {
		return "KubernetesClient component is not initialized, skipping coverage", nil
	}

	errs := []error{}
	ctx := context.Background()
	outStr := []string{"===== Coverage ====="}

	// Process Linux pods
	if e.Agent.LinuxNodeAgent.LabelSelectors == nil {
		outStr = append(outStr, "LinuxNodeAgent not initialized, skipping Linux pod coverage")
	} else if linuxPods, err := e.KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", e.Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
	}); err != nil {
		outStr = append(outStr, "Failed to list linux pods: "+err.Error())
	} else {
		if len(linuxPods.Items) >= 1 {
			outStr = append(outStr, "==== Linux pods ====")
		}
		for _, pod := range linuxPods.Items {
			outStr = append(outStr, fmt.Sprintf("Pod %s:", pod.Name))
			result, err := e.generateAndDownloadCoverageForPod(pod, podTypeLinux, outputDir)
			if err != nil {
				errs = append(errs, err)
			}
			outStr = append(outStr, result)
		}
	}

	// Process Windows pods
	if e.Agent.WindowsNodeAgent.LabelSelectors == nil {
		outStr = append(outStr, "WindowsNodeAgent not initialized, skipping Windows pod coverage")
	} else if windowsPods, err := e.KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", e.Agent.WindowsNodeAgent.LabelSelectors["app"]).String(),
	}); err != nil {
		outStr = append(outStr, "Failed to list windows pods: "+err.Error())
	} else {
		if len(windowsPods.Items) >= 1 {
			outStr = append(outStr, "==== Windows pods ====")
		}
		for _, pod := range windowsPods.Items {
			result, err := e.generateAndDownloadCoverageForPod(pod, podTypeWindows, outputDir)
			if err != nil {
				errs = append(errs, err)
			}
			outStr = append(outStr, result)
		}
	}

	// Process Cluster Agent pods
	if e.Agent.LinuxClusterAgent.LabelSelectors == nil {
		outStr = append(outStr, "LinuxClusterAgent not initialized, skipping Cluster Agent pod coverage")
	} else if clusterAgentPods, err := e.KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", e.Agent.LinuxClusterAgent.LabelSelectors["app"]).String(),
	}); err != nil {
		outStr = append(outStr, "Failed to list cluster agent pods: "+err.Error())
	} else {
		if len(clusterAgentPods.Items) >= 1 {
			outStr = append(outStr, "==== Cluster Agent pods ====")
		}
		for _, pod := range clusterAgentPods.Items {
			result, err := e.generateAndDownloadCoverageForPod(pod, podTypeClusterAgent, outputDir)
			if err != nil {
				errs = append(errs, err)
			}
			outStr = append(outStr, result)
		}
	}

	if len(errs) > 0 {
		return strings.Join(outStr, "\n"), errors.Join(errs...)
	}
	return strings.Join(outStr, "\n"), nil
}

func (e *Kubernetes) generateAndDownloadCoverageForPod(pod v1.Pod, podType podType, outputDir string) (string, error) {
	commandCoverages := e.getAgentCoverageCommands(podType)
	outStr := []string{}
	errs := []error{}
	for _, target := range commandCoverages {
		outStr = append(outStr, fmt.Sprintf("Container %s:", target.AgentName))
		stdout, stderr, err := e.KubernetesCluster.KubernetesClient.PodExec(pod.Namespace, pod.Name, target.AgentName, target.CoverageCommand)
		output := strings.Join([]string{stdout, stderr}, "\n")
		if err != nil {
			outStr, errs = updateErrorOutput(target, outStr, errs, err.Error())
			continue
		}
		// find coverage folder in command output
		re := regexp.MustCompile(`(?m)Coverage written to (.+)$`)
		matches := re.FindStringSubmatch(output)
		if len(matches) < 2 {
			outStr, errs = updateErrorOutput(target, outStr, errs, "output does not contain the path to the coverage folder, output: "+output)
			continue
		}
		err = e.KubernetesCluster.KubernetesClient.DownloadFromPod(pod.Namespace, pod.Name, target.AgentName, matches[1], outputDir+"/coverage")
		if err != nil {
			outStr, errs = updateErrorOutput(target, outStr, errs, err.Error())
			continue
		}
		outStr = append(outStr, "Downloaded coverage folder: "+matches[1])
	}
	if len(errs) > 0 {
		return strings.Join(outStr, "\n"), errors.Join(errs...)
	}
	return strings.Join(outStr, "\n"), nil
}

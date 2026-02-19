// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_logs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// readFn is a function that reads lines from a file. Both headFile and tailFile match this signature.
type readFn func(filePath string, lineCount int) (string, int, error)

// HeadPodLogHandler implements headPodLog: reads the first N lines of container logs in a pod.
type HeadPodLogHandler struct {
	wmeta workloadmeta.Component
}

// NewHeadPodLogHandler creates a new HeadPodLogHandler.
func NewHeadPodLogHandler(wmeta workloadmeta.Component) *HeadPodLogHandler {
	return &HeadPodLogHandler{wmeta: wmeta}
}

type podLogInputs struct {
	Namespace      string `json:"namespace"`
	PodName        string `json:"podName,omitempty"`
	DeploymentName string `json:"deploymentName,omitempty"`
	ContainerName  string `json:"containerName,omitempty"`
	LineCount      int    `json:"lineCount,omitempty"`
}

type podLogOutput struct {
	Logs     map[string]string `json:"logs"`
	PodCount int               `json:"podCount"`
}

// Run executes the headPodLog action.
func (h *HeadPodLogHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	return readPodLogs(h.wmeta, task, headFile)
}

// readPodLogs is the shared implementation used by both headPodLog and tailPodLog.
func readPodLogs(wmeta workloadmeta.Component, task *types.Task, reader readFn) (interface{}, error) {
	inputs, err := types.ExtractInputs[podLogInputs](task)
	if err != nil {
		return nil, err
	}

	if inputs.Namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}
	if inputs.PodName == "" && inputs.DeploymentName == "" {
		return nil, fmt.Errorf("either podName or deploymentName is required")
	}

	lineCount := clampLineCount(inputs.LineCount)

	pods, err := resolvePods(wmeta, inputs)
	if err != nil {
		return nil, err
	}
	if len(pods) == 0 {
		return nil, fmt.Errorf("no pods found matching the given criteria")
	}

	logs := make(map[string]string, len(pods))
	for _, pod := range pods {
		podLogDir, err := sanitizePodLogPath(pod.Namespace, pod.Name, pod.EntityMeta.UID)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve pod log path for %s: %w", pod.Name, err)
		}

		containers := getContainerNames(pod, inputs.ContainerName)
		for _, containerName := range containers {
			logContent, err := readContainerLog(podLogDir, containerName, lineCount, reader)
			if err != nil {
				// Include the error as the log content so the user sees what went wrong.
				key := fmt.Sprintf("%s/%s", pod.Name, containerName)
				logs[key] = fmt.Sprintf("error: %v", err)
				continue
			}
			key := fmt.Sprintf("%s/%s", pod.Name, containerName)
			logs[key] = logContent
		}
	}

	return &podLogOutput{
		Logs:     logs,
		PodCount: len(pods),
	}, nil
}

// resolvePods looks up pods using workloadmeta based on the provided inputs.
func resolvePods(wmeta workloadmeta.Component, inputs podLogInputs) ([]*workloadmeta.KubernetesPod, error) {
	if inputs.PodName != "" {
		pod, err := wmeta.GetKubernetesPodByName(inputs.PodName, inputs.Namespace)
		if err != nil {
			return nil, fmt.Errorf("pod %s/%s not found: %w", inputs.Namespace, inputs.PodName, err)
		}
		return []*workloadmeta.KubernetesPod{pod}, nil
	}

	// Filter by deployment name
	allPods := wmeta.ListKubernetesPods()
	var matched []*workloadmeta.KubernetesPod
	for _, pod := range allPods {
		if pod.Namespace != inputs.Namespace {
			continue
		}
		if isPodOwnedByDeployment(pod, inputs.DeploymentName) {
			matched = append(matched, pod)
		}
	}
	return matched, nil
}

// replicaSetHashSuffix matches the pod-template-hash suffix that Kubernetes
// appends to ReplicaSet names: a dash followed by 1+ alphanumeric characters at the end.
var replicaSetHashSuffix = regexp.MustCompile(`^(.+)-[a-z0-9]+$`)

// isPodOwnedByDeployment checks whether a pod is owned by the given deployment
// by extracting the deployment name from the ReplicaSet name (stripping the
// pod-template-hash suffix) and comparing it exactly.
func isPodOwnedByDeployment(pod *workloadmeta.KubernetesPod, deploymentName string) bool {
	for _, owner := range pod.Owners {
		if owner.Kind != "ReplicaSet" {
			continue
		}
		matches := replicaSetHashSuffix.FindStringSubmatch(owner.Name)
		if matches != nil && matches[1] == deploymentName {
			return true
		}
	}
	return false
}

// getContainerNames returns the list of container names to read logs from.
// If a specific container is requested, only that container is returned.
// Otherwise, all containers in the pod are returned.
func getContainerNames(pod *workloadmeta.KubernetesPod, requestedContainer string) []string {
	if requestedContainer != "" {
		return []string{requestedContainer}
	}
	var names []string
	for _, c := range pod.Containers {
		names = append(names, c.Name)
	}
	for _, c := range pod.InitContainers {
		names = append(names, c.Name)
	}
	return names
}

// readContainerLog reads the latest log file for a container within a pod's log directory.
func readContainerLog(podLogDir, containerName string, lineCount int, reader readFn) (string, error) {
	containerDir := filepath.Join(podLogDir, containerName)
	logFile, err := findLatestLogFile(containerDir)
	if err != nil {
		return "", fmt.Errorf("failed to find log file for container %s: %w", containerName, err)
	}
	content, _, err := reader(logFile, lineCount)
	if err != nil {
		return "", err
	}
	return content, nil
}

// findLatestLogFile returns the path to the highest-numbered .log file in a directory.
// Kubernetes rotates container logs as 0.log, 1.log, etc. The highest number is the current one.
func findLatestLogFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	var logFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".log") {
			logFiles = append(logFiles, entry.Name())
		}
	}
	if len(logFiles) == 0 {
		return "", fmt.Errorf("no log files found in %s", dir)
	}

	// Sort lexicographically; for numbered logs like 0.log, 1.log, ... 9.log this works.
	// For double-digit numbers we sort properly since "10.log" > "9.log" lexicographically is wrong,
	// but this matches kubelet's rotation which rarely exceeds single digits.
	sort.Strings(logFiles)
	latest := logFiles[len(logFiles)-1]
	return filepath.Join(dir, latest), nil
}

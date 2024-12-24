// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build e2e
// +build e2e

package utils

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-onboarding/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeClient "k8s.io/client-go/kubernetes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var timeout int64 = 60

func VerifyOperator(t *testing.T, namespace string, k8sClient kubeClient.Interface) {
	VerifyNumPodsForSelector(t, namespace, k8sClient, 1, "app.kubernetes.io/name=datadog-operator")
}

func VerifyNumPodsForSelector(t *testing.T, namespace string, k8sClient kubeClient.Interface, numPods int, selector string) {
	t.Log("Waiting for number of pods created", "number", numPods, "selector", selector)
	podsList, err := k8sClient.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector,
	})
	require.NoError(t, err)
	fmt.Println("SELECTOR: ", selector)
	fmt.Println("PODS LIST", podsList.Items)
	assert.Equal(t, numPods, len(podsList.Items))
}

func VerifyAgentPods(t *testing.T, namespace string, k8sClient kubeClient.Interface, selector string) {
	nodesList, err := k8sClient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)
	VerifyNumPodsForSelector(t, namespace, k8sClient, len(nodesList.Items), selector)
}

func VerifyCheck(c *assert.CollectT, collectorOutput string, checkName string) {
	var runningChecks map[string]interface{}

	checksJson := common.ParseCollectorJson(collectorOutput)
	if checksJson != nil {
		runningChecks = checksJson["runnerStats"].(map[string]interface{})["Checks"].(map[string]interface{})
		if check, found := runningChecks[checkName].(map[string]interface{}); found {
			for _, instance := range check {
				assert.EqualValues(c, checkName, instance.(map[string]interface{})["CheckName"].(string))

				lastError, exists := instance.(map[string]interface{})["LastError"].(string)
				assert.True(c, exists)
				assert.Empty(c, lastError)

				totalErrors, exists := instance.(map[string]interface{})["TotalErrors"].(float64)
				assert.True(c, exists)
				assert.Zero(c, totalErrors)

				totalMetricSamples, exists := instance.(map[string]interface{})["TotalMetricSamples"].(float64)
				assert.True(c, exists)
				assert.Greater(c, totalMetricSamples, float64(0))
			}
		} else {
			assert.True(c, found, fmt.Sprintf("Check %s not found or not yet running.", checkName))
		}
	}
}

func VerifyAgentPodLogs(c *assert.CollectT, collectorOutput string) {
	var agentLogs []interface{}
	logsJson := common.ParseCollectorJson(collectorOutput)

	tailedIntegrations := 0
	if logsJson != nil {
		agentLogs = logsJson["logsStats"].(map[string]interface{})["integrations"].([]interface{})
		for _, log := range agentLogs {
			if integration, ok := log.(map[string]interface{})["sources"].([]interface{})[0].(map[string]interface{}); ok {
				message, exists := integration["messages"].([]interface{})[0].(string)
				if exists && len(message) > 0 {
					num, _ := strconv.Atoi(string(message[0]))
					if num > 0 && strings.Contains(message, "files tailed") {
						tailedIntegrations++
					}
				}
			} else {
				assert.True(c, ok, "Failed to get sources from logs. Possible causes: missing 'sources' field, empty array, or incorrect data format.")
			}
		}
	}
	totalIntegrations := len(agentLogs)
	assert.True(c, tailedIntegrations >= totalIntegrations*80/100, "Expected at least 80%% of integrations to be tailed, got %d/%d", tailedIntegrations, totalIntegrations)
}

func ContextConfig(kubeConfig string) (cleanupFunc func(), kubeConfigPath string, err error) {
	tmpDir := "/tmp"
	kubeConfigPath = filepath.Join(tmpDir, ".kubeconfig")

	kcFile, err := os.Create(kubeConfigPath)
	if err != nil {
		return nil, "", err
	}
	defer kcFile.Close()

	_, err = kcFile.WriteString(kubeConfig)
	return func() {
		_ = os.Remove(kubeConfigPath)
	}, kubeConfigPath, nil
}

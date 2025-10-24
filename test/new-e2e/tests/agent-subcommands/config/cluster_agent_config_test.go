// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config contains helpers and e2e tests for cluster agent config subcommand
package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
)

type clusterAgentConfigSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestClusterAgentConfigSuite(t *testing.T) {
	e2e.Run(t, &clusterAgentConfigSuite{}, e2e.WithProvisioner(
		awskubernetes.KindProvisioner(
			awskubernetes.WithoutFakeIntake(),
		)))
}

func (v *clusterAgentConfigSuite) getClusterAgentPod() (corev1.Pod, error) {
	ctx := context.Background()
	pods, err := v.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, v1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", v.Env().Agent.LinuxClusterAgent.LabelSelectors["app"]).String(),
	})
	if err != nil {
		return corev1.Pod{}, err
	}
	require.NotEmpty(v.T(), pods.Items, "No cluster-agent pods found")
	return pods.Items[0], nil
}

// TestClusterAgentConfigDefault tests the default config command (without --all flag)
// This should return config without default values after the fix
func (v *clusterAgentConfigSuite) TestClusterAgentConfigDefault() {
	pod, err := v.getClusterAgentPod()
	require.NoError(v.T(), err, "Failed to get cluster-agent pod")

	// First, check the version to see what we're testing
	versionStdout, _, versionErr := v.Env().KubernetesCluster.KubernetesClient.PodExec(
		"datadog",
		pod.Name,
		"cluster-agent",
		[]string{"datadog-cluster-agent", "version"},
	)
	if versionErr == nil {
		v.T().Logf("Cluster-agent version:\n%s", versionStdout)
	}

	// Execute: datadog-cluster-agent config
	stdout, stderr, err := v.Env().KubernetesCluster.KubernetesClient.PodExec(
		"datadog",
		pod.Name,
		"cluster-agent",
		[]string{"datadog-cluster-agent", "config"},
	)

	// fmt.Println("stdout: ", stdout)
	// fmt.Println("stderr: ", stderr)
	// fmt.Println("error: ", err)

	require.NoError(v.T(), err, "cluster-agent config command should not return an error")
	assert.Empty(v.T(), stderr, "Standard error should be empty")
	assert.NotEmpty(v.T(), stdout, "Config output should not be empty")

	assert.NotContains(v.T(), stdout, "setting without-defaults not found", "Should not contain error about missing endpoint")
	assert.NotContains(v.T(), stdout, "exit code 255", "Should not exit with code 255")
}

// TestClusterAgentConfigWithAll tests the config command with --all flag
// This should return config including default values
func (v *clusterAgentConfigSuite) TestClusterAgentConfigWithAll() {
	pod, err := v.getClusterAgentPod()
	require.NoError(v.T(), err, "Failed to get cluster-agent pod")

	stdout, stderr, err := v.Env().KubernetesCluster.KubernetesClient.PodExec(
		"datadog",
		pod.Name,
		"cluster-agent",
		[]string{"datadog-cluster-agent", "config", "--all"},
	)

	// Assert no errors
	require.NoError(v.T(), err, "cluster-agent config --all command should not return an error")
	assert.Empty(v.T(), stderr, "Standard error should be empty")
	assert.NotEmpty(v.T(), stdout, "Config output should not be empty")
}

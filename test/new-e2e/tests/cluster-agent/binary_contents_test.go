// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clusteragent contains E2E tests for the cluster agent
package clusteragent

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	scenariokindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awskindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type clusterAgentBinarySuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestClusterAgentBinarySuite(t *testing.T) {
	e2e.Run(t, &clusterAgentBinarySuite{},
		e2e.WithProvisioner(
			awskindvm.Provisioner(
				awskindvm.WithRunOptions(
					scenariokindvm.WithoutFakeIntake(),
				),
			),
		),
	)
}

// TestSecretGenericConnectorPresenceAndPermissions verifies that the
// secret-generic-connector binary is present at the expected path in the
// cluster agent image with the correct permissions (550) and ownership
// (root:secret-manager).
func (s *clusterAgentBinarySuite) TestSecretGenericConnectorPresenceAndPermissions() {
	t := s.T()

	pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.TODO(), v1.ListOptions{})
	require.NoError(t, err)

	var clusterAgentPod corev1.Pod
	found := false
	for _, pod := range pods.Items {
		if strings.Contains(pod.Name, "cluster-agent") {
			clusterAgentPod = pod
			found = true
			break
		}
	}
	require.True(t, found, "cluster-agent pod not found in datadog namespace")

	// stat -c '%a %U %G' prints: <octal-perms> <owner> <group>
	stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.
		PodExec("datadog", clusterAgentPod.Name, "datadog-cluster-agent",
			[]string{"stat", "-c", "%a %U %G", "/opt/datadog-agent/bin/secret-generic-connector"})
	require.NoError(t, err, "stat failed (binary may be missing): %s", stderr)

	parts := strings.Fields(strings.TrimSpace(stdout))
	require.Len(t, parts, 3, "expected 'perms owner group' from stat, got: %q", stdout)

	assert.Equal(t, "550", parts[0], "unexpected permissions")
	assert.Equal(t, "root", parts[1], "unexpected owner")
	assert.Equal(t, "secret-manager", parts[2], "unexpected group")
}

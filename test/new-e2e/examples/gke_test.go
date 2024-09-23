// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package examples

import (
	"context"
	gcpkubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/gcp/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
)

type gkeSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestGKESuite(t *testing.T) {
	e2e.Run(t, &gkeSuite{}, e2e.WithProvisioner(gcpkubernetes.GKEProvisioner()))
}

func (v *gkeSuite) TestGKE() {
	v.T().Log("Running GKE test")
	res, _ := v.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.TODO(), v1.ListOptions{})
	var clusterAgent corev1.Pod
	containsClusterAgent := false
	for _, pod := range res.Items {
		if strings.Contains(pod.Name, "cluster-agent") {
			containsClusterAgent = true
			clusterAgent = pod
			break
		}
	}
	assert.True(v.T(), containsClusterAgent, "Cluster Agent not found")

	stdout, stderr, err := v.Env().KubernetesCluster.KubernetesClient.
		PodExec("datadog", clusterAgent.Name, "cluster-agent", []string{"ls"})
	require.NoError(v.T(), err)
	assert.Empty(v.T(), stderr)
	assert.NotEmpty(v.T(), stdout)
}

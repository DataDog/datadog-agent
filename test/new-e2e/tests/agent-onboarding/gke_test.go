// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package agent_onboarding

import (
	"context"
	gcpkubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/gcp/kubernetes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
)

type exampleGkeSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestExampleGKESuite(t *testing.T) {
	e2e.Run(t, &exampleGkeSuite{}, e2e.WithProvisioner(gcpkubernetes.GKEProvisioner()), e2e.WithDevMode(), e2e.WithSkipDeleteOnFailure())
}

func (v *exampleGkeSuite) TestExampleGKE() {
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

	v.Assert().EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := v.Env().FakeIntake.Client().GetMetricNames()
		require.NoError(v.T(), err, "Failed to get metric names from fake intake")
		v.T().Log("Metrics received from fake intake:", metrics)
		require.NotEmpty(v.T(), metrics, "No metrics received from fake intake")
	}, 300*time.Second, 15*time.Second, "Could not validate operator pod in time")

	stdout, stderr, err := v.Env().KubernetesCluster.KubernetesClient.
		PodExec("datadog", clusterAgent.Name, "cluster-agent", []string{"ls"})
	assert.NoError(v.T(), err)
	assert.Empty(v.T(), stderr)
	assert.NotEmpty(v.T(), stdout)
	assert.False(v.T(), true)
}

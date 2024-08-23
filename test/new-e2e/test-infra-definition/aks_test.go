// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinfradefinition

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/test-infra-definitions/scenarios/azure/fakeintake"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	azurekubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/azure/kubernetes"
)

type aksSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestAKSSuite(t *testing.T) {
	e2e.Run(t, &aksSuite{}, e2e.WithProvisioner(azurekubernetes.AKSProvisioner(azurekubernetes.WithFakeIntakeOptions(fakeintake.WithDDDevForwarding()))))
}

func (v *aksSuite) TestAKS() {
	v.T().Log("Running AKS test")
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

	assert.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		metrics, err := v.Env().FakeIntake.Client().GetMetricNames()
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
	}, 2*time.Minute, 10*time.Second)

}

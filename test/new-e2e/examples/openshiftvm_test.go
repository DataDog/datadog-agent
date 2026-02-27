// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package examples

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gcpopenshiftvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/gcp/kubernetes/openshiftvm"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

type openshiftvmExampleSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestOpenShiftVMExample(t *testing.T) {
	e2e.Run(t, &openshiftvmExampleSuite{}, e2e.WithProvisioner(gcpopenshiftvm.OpenshiftVMProvisioner()))
}

func (v *openshiftvmExampleSuite) TestOpenShiftVM() {
	v.T().Log("Running OpenShift VM test")
	res, _ := v.Env().KubernetesCluster.Client().CoreV1().Pods("datadog-openshift").List(context.TODO(), v1.ListOptions{})
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
		PodExec("datadog-openshift", clusterAgent.Name, "cluster-agent", []string{"ls"})
	require.NoError(v.T(), err)
	assert.Empty(v.T(), stderr)
	assert.NotEmpty(v.T(), stdout)
}

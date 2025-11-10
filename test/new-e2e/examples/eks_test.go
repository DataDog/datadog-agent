// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/test-infra-definitions/common/config"
	tifeks "github.com/DataDog/test-infra-definitions/scenarios/aws/eks"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"

	// "github.com/DataDog/test-infra-definitions/components/datadog/apps/dogstatsd"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/nginx"
	compkube "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type myEKSSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestMyEKSSuite(t *testing.T) {
	e2e.Run(t, &myEKSSuite{}, e2e.WithProvisioner(
		awskubernetes.EKSProvisioner(
			awskubernetes.WithEKSOptions(
				tifeks.WithLinuxNodeGroup(),
				tifeks.WithWindowsNodeGroup(),
				tifeks.WithBottlerocketNodeGroup(),
				tifeks.WithLinuxARMNodeGroup(),
			),
			awskubernetes.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*compkube.Workload, error) {
				return nginx.K8sAppDefinition(e, kubeProvider, "nginx", "", false, nil)
			}),
		)))
}

func (v *myEKSSuite) TestClusterAgentInstalled() {
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
		PodExec("datadog", clusterAgent.Name, "datadog-cluster-agent", []string{"ls"})
	require.NoError(v.T(), err)
	assert.Empty(v.T(), stderr)
	assert.NotEmpty(v.T(), stdout)

}

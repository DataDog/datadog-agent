// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinfradefinition

import (
	"context"
	"strings"
	"testing"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	compkube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	scenariokindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awskindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

type myKindSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestMyKindSuite(t *testing.T) {
	e2e.Run(t, &myKindSuite{},
		e2e.WithProvisioner(
			awskindvm.Provisioner(
				awskindvm.WithRunOptions(
					scenariokindvm.WithoutFakeIntake(),
					scenariokindvm.WithAgentOptions(kubernetesagentparams.WithClusterName(pulumi.String("kind-test"))),
					scenariokindvm.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*compkube.Workload, error) {
						return nginx.K8sAppDefinition(e, kubeProvider, "nginx", 80, "", false, nil)
					}),
				)),
		),
		e2e.WithSkipCoverage(),
	)
}

func (v *myKindSuite) TestClusterAgentInstalled() {
	res, _ := v.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.TODO(), v1.ListOptions{})
	containsClusterAgent := false
	for _, pod := range res.Items {
		if strings.Contains(pod.Name, "cluster-agent") {
			containsClusterAgent = true
			break
		}
	}
	assert.True(v.T(), containsClusterAgent, "Cluster Agent not found")
}

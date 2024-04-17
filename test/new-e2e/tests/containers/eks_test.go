// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/eks"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/tools/clientcmd"
)

type eksSuite struct {
	k8sSuite
}

func TestEKSSuite(t *testing.T) {
	t.Skip("incident-26767: skip as test is failing on dockerhub rate limit")
	suite.Run(t, &eksSuite{})
}

func (suite *eksSuite) SetupSuite() {
	ctx := context.Background()

	stackConfig := runner.ConfigMap{
		"ddagent:deploy":        auto.ConfigValue{Value: "true"},
		"ddagent:fakeintake":    auto.ConfigValue{Value: "true"},
		"ddtestworkload:deploy": auto.ConfigValue{Value: "true"},
		"dddogstatsd:deploy":    auto.ConfigValue{Value: "true"},
	}

	_, stackOutput, err := infra.GetStackManager().GetStackNoDeleteOnFailure(ctx, "eks-cluster", stackConfig, eks.Run, false, nil)
	if !suite.Assert().NoError(err) {
		stackName, err := infra.GetStackManager().GetPulumiStackName("eks-cluster")
		suite.Require().NoError(err)
		suite.T().Log(dumpEKSClusterState(ctx, stackName))
		if !runner.GetProfile().AllowDevMode() || !*keepStacks {
			infra.GetStackManager().DeleteStack(ctx, "eks-cluster", nil)
		}
		suite.T().FailNow()
	}

	fakeintake := &components.FakeIntake{}
	fiSerialized, err := json.Marshal(stackOutput.Outputs["dd-Fakeintake-aws-ecs"].Value)
	suite.Require().NoError(err)
	suite.Require().NoError(fakeintake.Import(fiSerialized, &fakeintake))
	suite.Require().NoError(fakeintake.Init(suite))
	suite.Fakeintake = fakeintake.Client()

	kubeCluster := &components.KubernetesCluster{}
	kubeSerialized, err := json.Marshal(stackOutput.Outputs["dd-Cluster-aws-eks"].Value)
	suite.Require().NoError(err)
	suite.Require().NoError(kubeCluster.Import(kubeSerialized, &kubeCluster))
	suite.Require().NoError(kubeCluster.Init(suite))
	suite.KubeClusterName = kubeCluster.ClusterName
	suite.K8sClient = kubeCluster.Client()
	suite.K8sConfig, err = clientcmd.RESTConfigFromKubeConfig([]byte(kubeCluster.KubeConfig))
	suite.Require().NoError(err)

	suite.AgentLinuxHelmInstallName = stackOutput.Outputs["agent-linux-helm-install-name"].Value.(string)
	suite.AgentWindowsHelmInstallName = stackOutput.Outputs["agent-windows-helm-install-name"].Value.(string)

	suite.k8sSuite.SetupSuite()
}

func (suite *eksSuite) TearDownSuite() {
	suite.k8sSuite.TearDownSuite()

	ctx := context.Background()
	stackName, err := infra.GetStackManager().GetPulumiStackName("eks-cluster")
	suite.Require().NoError(err)
	suite.T().Log(dumpEKSClusterState(ctx, stackName))
}

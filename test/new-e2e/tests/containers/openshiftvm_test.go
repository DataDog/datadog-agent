// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package containers

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	gcpfakeintake "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	gcpopenshiftvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/gcp/kubernetes/openshiftvm"

	"github.com/fatih/color"
)

type openShiftVMSuite struct {
	k8sSuite
}

func TestOpenShiftVMSuite(t *testing.T) {
	e2e.Run(t, &openShiftVMSuite{}, e2e.WithProvisioner(gcpopenshiftvm.OpenshiftVMProvisioner(
		gcpopenshiftvm.WithFakeIntakeOptions(
			gcpfakeintake.WithLoadBalancer(),
			gcpfakeintake.WithRetentionPeriod("1h"),
		),
		gcpopenshiftvm.WithAgentOptions(
			kubernetesagentparams.WithDualShipping(),
		),
		gcpopenshiftvm.WithDeployArgoRollout(),
	)))
}

func (suite *openShiftVMSuite) SetupSuite() {
	suite.k8sSuite.SetupSuite()
	suite.clusterName = suite.Env().KubernetesCluster.ClusterName
	suite.Fakeintake = suite.Env().FakeIntake.Client()
	suite.runtime = "cri-o"
}

func (suite *openShiftVMSuite) TearDownSuite() {
	suite.k8sSuite.TearDownSuite()

	color.NoColor = false
	c := color.New(color.Bold).SprintfFunc()
	suite.T().Log(c("The data produced and asserted by these tests can be viewed on this dashboard:"))
	c = color.New(color.Bold, color.FgBlue).SprintfFunc()
	suite.T().Log(c("https://dddev.datadoghq.com/dashboard/qcp-brm-ysc/e2e-tests-containers-k8s?refresh_mode=paused&tpl_var_kube_cluster_name%%5B0%%5D=%s&tpl_var_fake_intake_task_family%%5B0%%5D=%s-fakeintake-ecs&from_ts=%d&to_ts=%d&live=false",
		suite.clusterName,
		suite.clusterName,
		suite.StartTime().UnixMilli(),
		suite.EndTime().UnixMilli(),
	))
}

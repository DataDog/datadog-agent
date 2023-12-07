// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"context"
	"fmt"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
	dockervm "github.com/DataDog/test-infra-definitions/scenarios/aws/dockerVM"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/stretchr/testify/suite"
	"os"
	"testing"
	"time"
)

type DockerSuite struct {
	baseSuite
	fullAgentImagePath string
	stackName          string
}

func TestDockerSuite(t *testing.T) {

	agentFullImage := os.Getenv("AGENT_FULL_IMAGE")

	stackName := "docker-stack"

	if agentFullImage != "" {
		suite.Run(t, &DockerSuite{stackName: stackName, fullAgentImagePath: agentFullImage})
	} else {
		// Full Agent
		suite.Run(t, &DockerSuite{stackName: stackName, fullAgentImagePath: "gcr.io/datadoghq/agent:latest"})

		// DogstatsD Standalone
		suite.Run(t, &DockerSuite{stackName: stackName, fullAgentImagePath: "datadog/dogstatsd:latest"})
	}
}

func (suite *DockerSuite) SetupSuite() {
	ctx := context.Background()

	stackConfig := runner.ConfigMap{
		"ddagent:deploy":        auto.ConfigValue{Value: "true"},
		"ddagent:fakeintake":    auto.ConfigValue{Value: "true"},
		"ddagent:fullImagePath": auto.ConfigValue{Value: suite.fullAgentImagePath},
	}

	_, stackOutput, err := infra.GetStackManager().GetStackNoDeleteOnFailure(ctx, suite.stackName, stackConfig, dockervm.Run, false)

	if !suite.Assert().NoError(err) {
		suite.FailNow(ctx)
	}

	fakeintakeHost := stackOutput.Outputs["fakeintake-host"].Value.(string)
	suite.Fakeintake = fakeintake.NewClient(fmt.Sprintf("http://%s", fakeintakeHost))

	vmConnection := stackOutput.Outputs["vm-connection"].Value.(map[string]interface{})
	suite.clusterName = fmt.Sprintf("%s-%v", os.Getenv("USER"), vmConnection["host"])

	suite.Eventuallyf(
		func() bool { return suite.Fakeintake.GetServerHealth() == nil },
		1*time.Minute,
		5*time.Second,
		"Fakeintake is never healthy",
	)

	err = suite.Fakeintake.FlushServerAndResetAggregators()
	if !suite.Assert().NoError(err) {
		fmt.Println("Failed to flush/reset fakeintake")
		suite.FailNow(ctx)
	}

	suite.baseSuite.SetupSuite()
}

func (suite *DockerSuite) TearDownSuite() {
	suite.baseSuite.TearDownSuite()
}

func (suite *DockerSuite) FailNow(ctx context.Context) {
	_, err := infra.GetStackManager().GetPulumiStackName(suite.stackName)
	suite.Require().NoError(err)
	suite.T().Log(dumpKindClusterState(ctx, suite.stackName))
	if !runner.GetProfile().AllowDevMode() || !*keepStacks {
		infra.GetStackManager().DeleteStack(ctx, suite.stackName)
	}
	suite.T().FailNow()
}

func (suite *DockerSuite) TestDSDWithUDS() {
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "custom.metric",
			Tags: []string{
				`container_name:metric-sender-uds`,
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_name:metric-sender-uds$`,
				`^short_image`,
				`^image_tag`,
				`^docker_image`,
				`^git.commit.sha`,
				`^git.repository_url`,
				`^series`,
				`^image_name`,
				`^image_id`,
				`^container_id`,
			},
		},
	})
}

func (suite *DockerSuite) TestDSDWithUDP() {
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "custom.metric",
			Tags: []string{
				`container_name:metric-sender-udp`,
			},
		},

		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_name:metric-sender-udp$`,
				`^short_image`,
				`^image_tag`,
				`^docker_image`,
				`^git.commit.sha`,
				`^git.repository_url`,
				`^series`,
				`^image_name`,
				`^image_id`,
				`^container_id`,
			},
		},
	})
}

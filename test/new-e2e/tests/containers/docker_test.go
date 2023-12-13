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
)

type DockerSuite struct {
	baseSuite
	stackName string
}

func TestDockerSuite(t *testing.T) {
	stackName := "docker-stack"
	suite.Run(t, &DockerSuite{stackName: stackName})
}

func (suite *DockerSuite) SetupSuite() {
	ctx := context.Background()

	stackConfig := runner.ConfigMap{
		"ddagent:deploy":     auto.ConfigValue{Value: "true"},
		"ddagent:fakeintake": auto.ConfigValue{Value: "true"},
	}

	_, stackOutput, err := infra.GetStackManager().GetStackNoDeleteOnFailure(ctx, suite.stackName, stackConfig, dockervm.Run, false)

	if !suite.Assert().NoError(err) {
		suite.FailNow(ctx)
	}

	fakeintakeHost := stackOutput.Outputs["fakeintake-host"].Value.(string)
	suite.Fakeintake = fakeintake.NewClient(fmt.Sprintf("http://%s", fakeintakeHost))

	vmConnection := stackOutput.Outputs["vm-connection"].Value.(map[string]interface{})
	suite.clusterName = fmt.Sprintf("%s-%v", os.Getenv("USER"), vmConnection["host"])

	suite.baseSuite.SetupSuite()
}

func (suite *DockerSuite) FailNow(ctx context.Context) {
	_, err := infra.GetStackManager().GetPulumiStackName(suite.stackName)
	suite.Require().NoError(err)
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

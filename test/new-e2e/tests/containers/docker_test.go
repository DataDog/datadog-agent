// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/stretchr/testify/suite"
	"os"
	"testing"
)

type DockerSuite struct {
	baseSuite
}

func TestDockerSuite(t *testing.T) {
	suite.Run(t, &DockerSuite{})
}

func (suite *DockerSuite) SetupSuite() {
	ctx := context.Background()

	stackConfig := runner.ConfigMap{
		"ddagent:deploy":     auto.ConfigValue{Value: "true"},
		"ddagent:fakeintake": auto.ConfigValue{Value: "true"},
	}

	_, stackOutput, err := infra.GetStackManager().GetStack(ctx, "dockerstack", stackConfig, ec2.VMRunWithDocker, false)
	suite.Require().NoError(err)

	var fakeintake components.FakeIntake
	fiSerialized, err := json.Marshal(stackOutput.Outputs["dd-Fakeintake-aws-aws-vm"].Value)
	suite.Require().NoError(err)
	suite.Require().NoError(fakeintake.Import(fiSerialized, &fakeintake))
	suite.Require().NoError(fakeintake.Init(suite))
	suite.Fakeintake = fakeintake.Client()

	var host components.RemoteHost
	hostSerialized, err := json.Marshal(stackOutput.Outputs["dd-Host-aws-vm"].Value)
	suite.Require().NoError(err)
	suite.Require().NoError(host.Import(hostSerialized, &host))
	suite.Require().NoError(host.Init(suite))
	suite.clusterName = fmt.Sprintf("%s-%v", os.Getenv("USER"), host.Address)

	suite.baseSuite.SetupSuite()
}

func (suite *DockerSuite) TestDSDWithUDS() {
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "custom.metric",
			Tags: []string{
				`^container_name:metric-sender-uds$`,
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:metric-sender-uds$`,
				`^docker_image:ghcr\.io/datadog/apps-dogstatsd:main$`,
				`^git.commit.sha:`,
				`^git.repository_url:https://github\.com/DataDog/test-infra-definitions$`,
				`^image_id:sha256:`,
				`^image_name:ghcr\.io/datadog/apps-dogstatsd$`,
				`^image_tag:main$`,
				`^series:`,
				`^short_image:apps-dogstatsd$`,
			},
		},
	})
}

func (suite *DockerSuite) TestDSDWithUDP() {
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "custom.metric",
			Tags: []string{
				`^container_name:metric-sender-udp$`,
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:metric-sender-udp$`,
				`^docker_image:ghcr\.io/datadog/apps-dogstatsd:main$`,
				`^git.commit.sha:`,
				`^git.repository_url:https://github\.com/DataDog/test-infra-definitions$`,
				`^image_id:sha256:`,
				`^image_name:ghcr\.io/datadog/apps-dogstatsd$`,
				`^image_tag:main$`,
				`^series:`,
				`^short_image:apps-dogstatsd$`,
			},
		},
	})
}

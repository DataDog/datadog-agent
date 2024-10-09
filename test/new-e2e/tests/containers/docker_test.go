// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/docker"
)

type DockerSuite struct {
	baseSuite[environments.DockerHost]
}

func TestDockerSuite(t *testing.T) {
	e2e.Run(t, &DockerSuite{}, e2e.WithProvisioner(awsdocker.Provisioner(awsdocker.WithTestingWorkload())))
}

func (suite *DockerSuite) SetupSuite() {
	suite.baseSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/docker"
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

func (suite *DockerSuite) TestDockerMetrics() {
	for metric, extraTags := range map[string][]string{
		"docker.container.open_fds": {},
		"docker.cpu.limit":          {},
		"docker.cpu.shares":         {},
		"docker.cpu.system":         {},
		"docker.cpu.throttled":      {},
		"docker.cpu.throttled.time": {},
		"docker.cpu.usage":          {},
		"docker.cpu.user":           {},
		// "docker.io.read_bytes":       {`^device_name:`},
		// "docker.io.read_operations":  {`^device_name:`},
		// "docker.io.write_bytes":      {`^device_name:`},
		// "docker.io.write_operations": {`^device_name:`},
		"docker.kmem.usage":       {},
		"docker.mem.cache":        {},
		"docker.mem.failed_count": {},
		"docker.mem.rss":          {},
		"docker.mem.swap":         {},
		"docker.mem.working_set":  {},
		"docker.net.bytes_rcvd":   {`^docker_network:`},
		"docker.net.bytes_sent":   {`^docker_network:`},
		"docker.thread.count":     {},
		"docker.thread.limit":     {},
		"docker.uptime":           {},
	} {
		expectedTags := append([]string{
			`^container_id:`,
			`^container_name:redis$`,
			`^docker_image:public.ecr.aws/docker/library/redis:latest$`,
			`^image_id:sha256:`,
			`^image_name:public.ecr.aws/docker/library/redis$`,
			`^image_tag:latest$`,
			`^runtime:docker$`,
			`^short_image:redis$`,
		}, extraTags...)

		suite.testMetric(&testMetricArgs{
			Filter: testMetricFilterArgs{
				Name: metric,
				Tags: []string{
					`^container_name:redis$`,
				},
			},
			Expect: testMetricExpectArgs{
				Tags: &expectedTags,
			},
		})
	}
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

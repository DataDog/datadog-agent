// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"fmt"
	"math/rand"
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	scendocker "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
)

type DockerSuite struct {
	baseSuite[environments.DockerHost]
}

func TestDockerSuite(t *testing.T) {
	e2e.Run(t, &DockerSuite{}, e2e.WithProvisioner(awsdocker.Provisioner(
		awsdocker.WithRunOptions(
			scendocker.WithTestingWorkload(),
		),
	)))
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
			`^docker_image:ghcr\.io/datadog/redis:` + regexp.QuoteMeta(apps.Version) + `$`,
			`^git\.commit\.sha:[[:xdigit:]]{40}$`,                                      // org.opencontainers.image.revision docker image label
			`^git\.repository_url:https://github\.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source docker image label
			`^image_id:sha256:`,
			`^image_name:ghcr\.io/datadog/redis$`,
			`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
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

	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "docker.images.available",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{},
			Value: &testMetricExpectValueArgs{
				Min: 4,
				Max: 5,
			},
		},
	})

	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "docker.images.intermediate",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{},
			Value: &testMetricExpectValueArgs{
				Min: 0,
				Max: 0,
			},
		},
	})

	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "docker.containers.running",
			Tags: []string{`^short_image:redis$`},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^docker_image:ghcr\.io/datadog/redis:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^git\.commit\.sha:[[:xdigit:]]{40}$`,                                      // org.opencontainers.image.revision docker image label
				`^git\.repository_url:https://github\.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source docker image label
				`^image_id:sha256:`,
				`^image_name:ghcr\.io/datadog/redis$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^short_image:redis$`,
			},
			Value: &testMetricExpectValueArgs{
				Min: 1,
				Max: 1,
			},
		},
	})

	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "docker.containers.running.total",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{},
			Value: &testMetricExpectValueArgs{
				Min: 5,
				Max: 5,
			},
		},
	})

	const ctrNameSize = 12
	const ctrNameCharset = "abcdefghijklmnopqrstuvwxyz0123456789"

	ctrNameData := make([]byte, ctrNameSize)
	for i := range ctrNameSize {
		ctrNameData[i] = ctrNameCharset[rand.Intn(len(ctrNameCharset))]
	}
	ctrName := "exit_42_" + string(ctrNameData)

	suite.Env().RemoteHost.MustExecute(fmt.Sprintf("docker run -d --name \"%s\" public.ecr.aws/docker/library/busybox sh -c \"exit 42\"", ctrName))

	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "docker.containers.stopped",
			Tags: []string{`^short_image:busybox$`},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^docker_image:public.ecr.aws/docker/library/busybox:latest$`,
				`^image_name:public.ecr.aws/docker/library/busybox$`,
				`^image_tag:latest$`,
				`^short_image:busybox$`,
			},
			Value: &testMetricExpectValueArgs{
				Min: 1,
				Max: 10,
			},
		},
	})

	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "docker.containers.stopped.total",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{},
			Value: &testMetricExpectValueArgs{
				Min: 1,
				Max: 10,
			},
		},
	})
}

func (suite *DockerSuite) TestDockerEvents() {
	const ctrNameSize = 12
	const ctrNameCharset = "abcdefghijklmnopqrstuvwxyz0123456789"

	ctrNameData := make([]byte, ctrNameSize)
	for i := range ctrNameSize {
		ctrNameData[i] = ctrNameCharset[rand.Intn(len(ctrNameCharset))]
	}
	ctrName := "exit_42_" + string(ctrNameData)

	suite.Env().RemoteHost.MustExecute(fmt.Sprintf("docker run -d --name \"%s\" public.ecr.aws/docker/library/busybox sh -c \"exit 42\"", ctrName))

	suite.testEvent(&testEventArgs{
		Filter: testEventFilterArgs{
			Source: "docker",
			Tags: []string{
				`^container_name:` + regexp.QuoteMeta(ctrName) + `$`,
			},
		},
		Expect: testEventExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:` + regexp.QuoteMeta(ctrName) + `$`,
				`^docker_image:public.ecr.aws/docker/library/busybox$`,
				`^image_id:sha256:`,
				`^image_name:public.ecr.aws/docker/library/busybox$`,
				`^short_image:busybox$`,
			},
			Title:     `busybox .*1 die`,
			Text:      "DIE\t" + regexp.QuoteMeta(ctrName),
			Priority:  "normal",
			AlertType: "info",
		},
	})
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
				`^docker_image:ghcr\.io/datadog/apps-dogstatsd:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^git\.commit\.sha:[[:xdigit:]]{40}$`,
				`^git.repository_url:https://github\.com/DataDog/test-infra-definitions$`,
				`^image_id:sha256:`,
				`^image_name:ghcr\.io/datadog/apps-dogstatsd$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
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
				`^docker_image:ghcr\.io/datadog/apps-dogstatsd:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^git\.commit\.sha:[[:xdigit:]]{40}$`,
				`^git.repository_url:https://github\.com/DataDog/test-infra-definitions$`,
				`^image_id:sha256:`,
				`^image_name:ghcr\.io/datadog/apps-dogstatsd$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^series:`,
				`^short_image:apps-dogstatsd$`,
			},
		},
	})
}

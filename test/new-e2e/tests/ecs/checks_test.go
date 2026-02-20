// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecs

import (
	"regexp"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"

	scenecs "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"
	provecs "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/ecs"
)

type ecsChecksSuite struct {
	BaseSuite[environments.ECS]
	ecsClusterName string
}

func TestECSChecksSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &ecsChecksSuite{}, e2e.WithProvisioner(provecs.Provisioner(
		provecs.WithRunOptions(
			scenecs.WithECSOptions(
				scenecs.WithFargateCapacityProvider(),
				scenecs.WithLinuxNodeGroup(),
			),
			scenecs.WithTestingWorkload(),
		),
	)))
}

func (suite *ecsChecksSuite) SetupSuite() {
	suite.BaseSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
	suite.ecsClusterName = suite.Env().ECSCluster.ClusterName
	suite.ClusterName = suite.Env().ECSCluster.ClusterName
}

func (suite *ecsChecksSuite) TestNginxECS() {
	// `nginx` check is configured via docker labels
	// Test it is properly scheduled
	suite.AssertMetric(&TestMetricArgs{
		Filter: TestMetricFilterArgs{
			Name: "nginx.net.request_per_s",
			Tags: []string{"^ecs_launch_type:ec2$"},
		},
		Expect: TestMetricExpectArgs{
			Tags: &[]string{
				`^aws_account:[[:digit:]]{12}$`,
				`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^cluster_arn:arn:aws:ecs:us-east-1:[[:digit:]]{12}:cluster/` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^container_id:`,
				`^container_name:ecs-.*-nginx-ec2-`,
				`^docker_image:ghcr\.io/datadog/apps-nginx-server:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:nginx$`,
				`^ecs_launch_type:ec2$`,
				`^ecs_service:` + regexp.QuoteMeta(strings.TrimSuffix(suite.ecsClusterName, "-ecs")) + `-nginx-ec2$`,
				`^git\.commit\.sha:[[:xdigit:]]{40}$`,                                    // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:sha256:`,
				`^image_name:ghcr\.io/datadog/apps-nginx-server$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^nginx_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^region:us-east-1$`,
				`^service_arn:`,
				`^short_image:apps-nginx-server$`,
				`^task_arn:`,
				`^task_definition_arn:`,
				`^task_family:.*-nginx-ec2$`,
				`^task_name:.*-nginx-ec2$`,
				`^task_version:[[:digit:]]+$`,
			},
			AcceptUnexpectedTags: true,
		},
	})

	suite.AssertLog(&TestLogArgs{
		Filter: TestLogFilterArgs{
			Service: "nginx",
			Tags:    []string{"^ecs_launch_type:ec2$"},
		},
		Expect: TestLogExpectArgs{
			Tags: &[]string{
				`^aws_account:[[:digit:]]{12}$`,
				`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^cluster_arn:arn:aws:ecs:us-east-1:[[:digit:]]{12}:cluster/` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^container_id:`,
				`^container_name:ecs-.*-nginx-ec2-`,
				`^docker_image:ghcr\.io/datadog/apps-nginx-server:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:nginx$`,
				`^ecs_launch_type:ec2$`,
				`^ecs_service:` + regexp.QuoteMeta(strings.TrimSuffix(suite.ecsClusterName, "-ecs")) + `-nginx-ec2$`,
				`^env:e2e-test$`,
				`^git\.commit\.sha:[[:xdigit:]]{40}$`, // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:sha256:`,
				`^image_name:ghcr\.io/datadog/apps-nginx-server$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^region:us-east-1$`,
				`^service:nginx$`,
				`^service_arn:`,
				`^short_image:apps-nginx-server$`,
				`^task_arn:arn:`,
				`^task_definition_arn:`,
				`^task_family:.*-nginx-ec2$`,
				`^task_name:.*-nginx-ec2$`,
				`^task_version:[[:digit:]]+$`,
				`^version:1\.0$`,
			},
			Message: `GET / HTTP/1\.1`,
		},
	})
}

func (suite *ecsChecksSuite) TestRedisECS() {
	// `redis` check is auto-configured due to image name
	// Test it is properly scheduled
	suite.AssertMetric(&TestMetricArgs{
		Filter: TestMetricFilterArgs{
			Name: "redis.net.instantaneous_ops_per_sec",
			Tags: []string{"^ecs_launch_type:ec2$"},
		},
		Expect: TestMetricExpectArgs{
			Tags: &[]string{
				`^aws_account:[[:digit:]]{12}$`,
				`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^cluster_arn:arn:aws:ecs:us-east-1:[[:digit:]]{12}:cluster/` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^container_id:`,
				`^container_name:ecs-.*-redis-ec2-`,
				`^docker_image:ghcr\.io/datadog/redis:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:redis$`,
				`^ecs_service:` + regexp.QuoteMeta(strings.TrimSuffix(suite.ecsClusterName, "-ecs")) + `-redis-ec2$`,
				`^ecs_launch_type:ec2$`,
				`^git\.commit\.sha:[[:xdigit:]]{40}$`,                                    // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:sha256:`,
				`^image_name:ghcr\.io/datadog/redis$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^region:us-east-1$`,
				`^service_arn:`,
				`^short_image:redis$`,
				`^task_arn:`,
				`^task_definition_arn:`,
				`^task_family:.*-redis-ec2$`,
				`^task_name:.*-redis-ec2$`,
				`^task_version:[[:digit:]]+$`,
			},
			AcceptUnexpectedTags: true,
		},
	})

	suite.AssertLog(&TestLogArgs{
		Filter: TestLogFilterArgs{
			Service: "redis",
			Tags:    []string{"^ecs_launch_type:ec2$"},
		},
		Expect: TestLogExpectArgs{
			Tags: &[]string{
				`^aws_account:[[:digit:]]{12}$`,
				`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^cluster_arn:arn:aws:ecs:us-east-1:[[:digit:]]{12}:cluster/` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^container_id:`,
				`^container_name:ecs-.*-redis-ec2-`,
				`^docker_image:ghcr\.io/datadog/redis:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:redis$`,
				`^ecs_launch_type:ec2$`,
				`^ecs_service:` + regexp.QuoteMeta(strings.TrimSuffix(suite.ecsClusterName, "-ecs")) + `-redis-ec2$`,
				`^env:e2e-test$`,
				`^git\.commit\.sha:[[:xdigit:]]{40}$`, // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:sha256:`,
				`^image_name:ghcr\.io/datadog/redis$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^region:us-east-1$`,
				`^service:redis$`,
				`^service_arn:`,
				`^short_image:redis$`,
				`^task_arn:arn:`,
				`^task_definition_arn:`,
				`^task_family:.*-redis-ec2$`,
				`^task_name:.*-redis-ec2$`,
				`^task_version:[[:digit:]]+$`,
				`^version:1\.0$`,
			},
			Message: `Accepted`,
		},
	})
}

func (suite *ecsChecksSuite) TestNginxFargate() {
	// `nginx` check is configured via docker labels
	// Test it is properly scheduled
	suite.AssertMetric(&TestMetricArgs{
		Filter: TestMetricFilterArgs{
			Name: "nginx.net.request_per_s",
			Tags: []string{"^ecs_launch_type:fargate$"},
		},
		Expect: TestMetricExpectArgs{
			Tags: &[]string{
				`^aws_account:[[:digit:]]{12}$`,
				`^availability_zone:`,
				`^availability-zone:`,
				`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^cluster_arn:arn:aws:ecs:us-east-1:[[:digit:]]{12}:cluster/` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^container_id:`,
				`^container_name:nginx$`,
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:nginx$`,
				`^ecs_launch_type:fargate$`,
				`^image_id:sha256:`,
				`^image_name:ghcr\.io/datadog/apps-nginx-server$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^nginx_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^region:us-east-1$`,
				`^service_arn:`,
				`^short_image:apps-nginx-server$`,
				`^task_arn:`,
				`^task_definition_arn:`,
				`^task_family:.*-nginx-fg$`,
				`^task_name:.*-nginx-fg$`,
				`^task_version:[[:digit:]]+$`,
			},
			AcceptUnexpectedTags: true,
		},
	})
}

func (suite *ecsChecksSuite) TestRedisFargate() {
	// `redis` check is auto-configured due to image name
	// Test it is properly scheduled
	suite.AssertMetric(&TestMetricArgs{
		Filter: TestMetricFilterArgs{
			Name: "redis.net.instantaneous_ops_per_sec",
			Tags: []string{"^ecs_launch_type:fargate$"},
		},
		Expect: TestMetricExpectArgs{
			Tags: &[]string{
				`^aws_account:[[:digit:]]{12}$`,
				`^availability_zone:`,
				`^availability-zone:`,
				`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^cluster_arn:arn:aws:ecs:us-east-1:[[:digit:]]{12}:cluster/` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^container_id:`,
				`^container_name:redis$`,
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:redis$`,
				`^ecs_launch_type:fargate$`,
				`^image_id:sha256:`,
				`^image_name:ghcr\.io/datadog/redis$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^region:us-east-1$`,
				`^service_arn:`,
				`^short_image:redis$`,
				`^task_arn:`,
				`^task_definition_arn:`,
				`^task_family:.*-redis-fg$`,
				`^task_name:.*-redis-fg$`,
				`^task_version:[[:digit:]]+$`,
			},
			AcceptUnexpectedTags: true,
		},
	})
}

func (suite *ecsChecksSuite) TestPrometheus() {
	// Test Prometheus check
	suite.AssertMetric(&TestMetricArgs{
		Filter: TestMetricFilterArgs{
			Name: "prometheus.prom_gauge",
		},
		Expect: TestMetricExpectArgs{
			Tags: &[]string{
				`^aws_account:[[:digit:]]{12}$`,
				`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^cluster_arn:arn:aws:ecs:us-east-1:[[:digit:]]{12}:cluster/` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^container_id:`,
				`^container_name:ecs-.*-prometheus-ec2-`,
				`^docker_image:ghcr\.io/datadog/apps-prometheus:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:prometheus$`,
				`^ecs_service:` + regexp.QuoteMeta(strings.TrimSuffix(suite.ecsClusterName, "-ecs")) + `-prometheus$`,
				`^endpoint:http://.*:8080/metrics$`,
				`^git\.commit\.sha:[[:xdigit:]]{40}$`,                                    // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:sha256:`,
				`^image_name:ghcr\.io/datadog/apps-prometheus$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^region:us-east-1$`,
				`^series:`,
				`^service_arn:`,
				`^short_image:apps-prometheus$`,
				`^task_arn:`,
				`^task_definition_arn:`,
				`^task_family:.*-prometheus-ec2$`,
				`^task_name:.*-prometheus-ec2$`,
				`^task_version:[[:digit:]]+$`,
			},
		},
	})
}

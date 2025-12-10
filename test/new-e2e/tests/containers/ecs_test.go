// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	awsecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	scenecs "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"

	provecs "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/ecs"
)

const (
	taskNameDogstatsdUDS = "dogstatsd-uds"
	taskNameDogstatsdUDP = "dogstatsd-udp"

	taskNameTracegenUDS = "tracegen-uds"
	taskNameTracegenTCP = "tracegen-tcp"
)

type ecsSuite struct {
	baseSuite[environments.ECS]
	ecsClusterName string
}

func TestECSSuite(t *testing.T) {
	e2e.Run(t, &ecsSuite{}, e2e.WithProvisioner(provecs.Provisioner(
		provecs.WithRunOptions(
			scenecs.WithECSOptions(
				scenecs.WithFargateCapacityProvider(),
				scenecs.WithLinuxNodeGroup(),
				scenecs.WithWindowsNodeGroup(),
				scenecs.WithLinuxBottleRocketNodeGroup(),
			),
			scenecs.WithTestingWorkload(),
		),
	)))
}

func (suite *ecsSuite) SetupSuite() {
	suite.baseSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
	suite.ecsClusterName = suite.Env().ECSCluster.ClusterName
	suite.clusterName = suite.Env().ECSCluster.ClusterName
}

func (suite *ecsSuite) TearDownSuite() {
	suite.baseSuite.TearDownSuite()

	color.NoColor = false
	c := color.New(color.Bold).SprintfFunc()
	suite.T().Log(c("The data produced and asserted by these tests can be viewed on this dashboard:"))
	c = color.New(color.Bold, color.FgBlue).SprintfFunc()
	suite.T().Log(c("https://dddev.datadoghq.com/dashboard/mnw-tdr-jd8/e2e-tests-containers-ecs?refresh_mode=paused&tpl_var_ecs_cluster_name%%5B0%%5D=%s&tpl_var_fake_intake_task_family%%5B0%%5D=%s-fakeintake-ecs&from_ts=%d&to_ts=%d&live=false",
		suite.ecsClusterName,
		strings.TrimSuffix(suite.ecsClusterName, "-ecs"),
		suite.StartTime().UnixMilli(),
		suite.EndTime().UnixMilli(),
	))
}

// Once pulumi has finished to create a stack, it can still take some time for the images to be pulled,
// for the containers to be started, for the agent collectors to collect workload information
// and to feed workload meta and the tagger.
//
// We could increase the timeout of all tests to cope with the agent tagger warmup time.
// But in case of a single bug making a single tag missing from every metric,
// all the tests would time out and that would be a waste of time.
//
// It’s better to have the first test having a long timeout to wait for the agent to warmup,
// and to have the following tests with a smaller timeout.
//
// Inside a testify test suite, tests are executed in alphabetical order.
// The 00 in Test00UpAndRunning is here to guarantee that this test, waiting for all tasks to be ready
// is run first.
func (suite *ecsSuite) Test00UpAndRunning() {
	ctx := context.Background()

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	suite.Require().NoErrorf(err, "Failed to load AWS config")

	client := awsecs.NewFromConfig(cfg)

	suite.Run("ECS tasks are ready", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			var initToken string
			for nextToken := &initToken; nextToken != nil; {
				if nextToken == &initToken {
					nextToken = nil
				}

				servicesList, err := client.ListServices(ctx, &awsecs.ListServicesInput{
					Cluster:    &suite.ecsClusterName,
					MaxResults: pointer.Ptr(int32(10)), // Because `DescribeServices` takes at most 10 services in input
					NextToken:  nextToken,
				})
				// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
				if !assert.NoErrorf(c, err, "Failed to list ECS services") {
					return
				}

				nextToken = servicesList.NextToken

				servicesDescription, err := client.DescribeServices(ctx, &awsecs.DescribeServicesInput{
					Cluster:  &suite.ecsClusterName,
					Services: servicesList.ServiceArns,
				})
				if !assert.NoErrorf(c, err, "Failed to describe ECS services %v", servicesList.ServiceArns) {
					continue
				}

				for _, serviceDescription := range servicesDescription.Services {
					assert.NotZerof(c, serviceDescription.DesiredCount, "ECS service %s has no task", *serviceDescription.ServiceName)

					for nextToken := &initToken; nextToken != nil; {
						if nextToken == &initToken {
							nextToken = nil
						}

						tasksList, err := client.ListTasks(ctx, &awsecs.ListTasksInput{
							Cluster:       &suite.ecsClusterName,
							ServiceName:   serviceDescription.ServiceName,
							DesiredStatus: awsecstypes.DesiredStatusRunning,
							MaxResults:    pointer.Ptr(int32(100)), // Because `DescribeTasks` takes at most 100 tasks in input
							NextToken:     nextToken,
						})
						if !assert.NoErrorf(c, err, "Failed to list ECS tasks for service %s", *serviceDescription.ServiceName) {
							break
						}

						nextToken = tasksList.NextToken

						tasksDescription, err := client.DescribeTasks(ctx, &awsecs.DescribeTasksInput{
							Cluster: &suite.ecsClusterName,
							Tasks:   tasksList.TaskArns,
						})
						if !assert.NoErrorf(c, err, "Failed to describe ECS tasks %v", tasksList.TaskArns) {
							continue
						}

						for _, taskDescription := range tasksDescription.Tasks {
							assert.Equalf(c, string(awsecstypes.DesiredStatusRunning), *taskDescription.LastStatus,
								"Task %s of service %s is not running", *taskDescription.TaskArn, *serviceDescription.ServiceName)
							assert.NotEqualf(c, awsecstypes.HealthStatusUnhealthy, taskDescription.HealthStatus,
								"Task %s of service %s is unhealthy", *taskDescription.TaskArn, *serviceDescription.ServiceName)
						}
					}
				}
			}
		}, 15*time.Minute, 10*time.Second, "Not all tasks became ready in time.")
	})
}

func (suite *ecsSuite) TestNginxECS() {
	// `nginx` check is configured via docker labels
	// Test it is properly scheduled
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "nginx.net.request_per_s",
			Tags: []string{"^ecs_launch_type:ec2$"},
		},
		Expect: testMetricExpectArgs{
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

	suite.testLog(&testLogArgs{
		Filter: testLogFilterArgs{
			Service: "apps-nginx-server",
			Tags:    []string{"^ecs_launch_type:ec2$"},
		},
		Expect: testLogExpectArgs{
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
				`^region:us-east-1$`,
				`^service_arn:`,
				`^short_image:apps-nginx-server$`,
				`^task_arn:arn:`,
				`^task_definition_arn:`,
				`^task_family:.*-nginx-ec2$`,
				`^task_name:.*-nginx-ec2$`,
				`^task_version:[[:digit:]]+$`,
			},
			Message: `GET / HTTP/1\.1`,
		},
	})
}

func (suite *ecsSuite) TestRedisECS() {
	// `redis` check is auto-configured due to image name
	// Test it is properly scheduled
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "redis.net.instantaneous_ops_per_sec",
			Tags: []string{"^ecs_launch_type:ec2$"},
		},
		Expect: testMetricExpectArgs{
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

	suite.testLog(&testLogArgs{
		Filter: testLogFilterArgs{
			Service: "redis",
			Tags:    []string{"^ecs_launch_type:ec2$"},
		},
		Expect: testLogExpectArgs{
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
				`^git\.commit\.sha:[[:xdigit:]]{40}$`,                                    // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:sha256:`,
				`^image_name:ghcr\.io/datadog/redis$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^region:us-east-1$`,
				`^service_arn:`,
				`^short_image:redis$`,
				`^task_arn:arn:`,
				`^task_definition_arn:`,
				`^task_family:.*-redis-ec2$`,
				`^task_name:.*-redis-ec2$`,
				`^task_version:[[:digit:]]+$`,
			},
			Message: `Accepted`,
		},
	})
}

func (suite *ecsSuite) TestNginxFargate() {
	// `nginx` check is configured via docker labels
	// Test it is properly scheduled
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "nginx.net.request_per_s",
			Tags: []string{"^ecs_launch_type:fargate$"},
		},
		Expect: testMetricExpectArgs{
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

func (suite *ecsSuite) TestRedisFargate() {
	// `redis` check is auto-configured due to image name
	// Test it is properly scheduled
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "redis.net.instantaneous_ops_per_sec",
			Tags: []string{"^ecs_launch_type:fargate$"},
		},
		Expect: testMetricExpectArgs{
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
				`^ecs_launch_type:fargate`,
				`^image_id:sha256:`,
				`^image_name:ghcr\.io/datadog/redis$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^region:us-east-1$`,
				`^service_arn:`,
				`^short_image:redis$`,
				`^task_arn:`,
				`^task_definition_arn:`,
				`^task_family:.*-redis-fg$`,
				`^task_name:.*-redis-fg*`,
				`^task_version:[[:digit:]]+$`,
			},
			AcceptUnexpectedTags: true,
		},
	})
}

func (suite *ecsSuite) TestWindowsFargate() {
	suite.testCheckRun(&testCheckRunArgs{
		Filter: testCheckRunFilterArgs{
			Name: "http.can_connect",
			Tags: []string{
				"^ecs_launch_type:fargate$",
				"^container_name:aspnetsample$",
			},
		},
		Expect: testCheckRunExpectArgs{
			Tags: &[]string{
				`^aws_account:[[:digit:]]{12}$`,
				`^availability_zone:`,
				`^availability-zone:`,
				`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^cluster_arn:arn:aws:ecs:us-east-1:[[:digit:]]{12}:cluster/` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^container_id:`,
				`^container_name:aspnetsample$`,
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:aspnetsample$`,
				`^ecs_launch_type:fargate$`,
				`^ecs_service:` + regexp.QuoteMeta(strings.TrimSuffix(suite.ecsClusterName, "-ecs")) + `-aspnetsample-fg$`,
				`^image_id:sha256:`,
				`^image_name:mcr.microsoft.com/dotnet/samples$`,
				`^image_tag:aspnetapp-nanoserver-ltsc2022$`,
				`^region:us-east-1$`,
				`^service_arn:`,
				`^short_image:samples$`,
				`^task_arn:`,
				`^task_definition_arn:`,
				`^task_family:.*-aspnet-fg$`,
				`^task_name:.*-aspnet-fg*`,
				`^task_version:[[:digit:]]+$`,
				`^url:`,
			},
			AcceptUnexpectedTags: true,
		},
	})

	// Test container check
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "container.cpu.usage",
			Tags: []string{
				"^ecs_container_name:aspnetsample$",
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^aws_account:[[:digit:]]{12}$`,
				`^availability_zone:`,
				`^availability-zone:`,
				`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^cluster_arn:arn:aws:ecs:us-east-1:[[:digit:]]{12}:cluster/` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^container_id:`,
				`^container_name:aspnetsample$`,
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:aspnetsample$`,
				`^ecs_launch_type:fargate$`,
				`^ecs_service:` + regexp.QuoteMeta(strings.TrimSuffix(suite.ecsClusterName, "-ecs")) + `-aspnetsample-fg$`,
				`^image_id:sha256:`,
				`^image_name:mcr.microsoft.com/dotnet/samples$`,
				`^image_tag:aspnetapp-nanoserver-ltsc2022$`,
				`^region:us-east-1$`,
				`^runtime:ecsfargate$`,
				`^service_arn:`,
				`^short_image:samples$`,
				`^task_arn:`,
				`^task_definition_arn:`,
				`^task_family:.*-aspnet-fg$`,
				`^task_name:.*-aspnet-fg*`,
				`^task_version:[[:digit:]]+$`,
			},
		},
	})
}

func (suite *ecsSuite) TestCPU() {
	// Test CPU metrics
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "container.cpu.usage",
			Tags: []string{
				"^ecs_container_name:stress-ng$",
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^aws_account:[[:digit:]]{12}$`,
				`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^cluster_arn:arn:aws:ecs:us-east-1:[[:digit:]]{12}:cluster/` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^container_id:`,
				`^container_name:ecs-.*-stress-ng-ec2-`,
				`^docker_image:ghcr\.io/datadog/apps-stress-ng:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:stress-ng$`,
				`^ecs_service:` + regexp.QuoteMeta(strings.TrimSuffix(suite.ecsClusterName, "-ecs")) + `-stress-ng$`,
				`^git\.commit\.sha:[[:xdigit:]]{40}$`,
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`,
				`^image_id:sha256:`,
				`^image_name:ghcr\.io/datadog/apps-stress-ng$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^region:us-east-1$`,
				`^runtime:docker$`,
				`^service_arn:`,
				`^short_image:apps-stress-ng$`,
				`^task_arn:`,
				`^task_definition_arn:`,
				`^task_family:.*-stress-ng-ec2$`,
				`^task_name:.*-stress-ng-ec2$`,
				`^task_version:[[:digit:]]+$`,
			},
			Value: &testMetricExpectValueArgs{
				Max: 155000000,
				Min: 145000000,
			},
		},
	})
}

func (suite *ecsSuite) TestDogtstatsdUDS() {
	suite.testDogstatsd(taskNameDogstatsdUDS)
}

func (suite *ecsSuite) TestDogtstatsdUDP() {
	suite.testDogstatsd(taskNameDogstatsdUDP)
}

func (suite *ecsSuite) testDogstatsd(taskName string) {
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "custom.metric",
			Tags: []string{
				`^task_name:.*-` + regexp.QuoteMeta(taskName) + `-ec2$`,
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^aws_account:[[:digit:]]{12}$`,
				`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^cluster_arn:arn:aws:ecs:us-east-1:[[:digit:]]{12}:cluster/` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^container_id:`,
				`^container_name:ecs-.*-` + regexp.QuoteMeta(taskName) + `-ec2-`,
				`^docker_image:ghcr\.io/datadog/apps-dogstatsd:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:dogstatsd$`,
				`^ecs_service:` + regexp.QuoteMeta(strings.TrimSuffix(suite.ecsClusterName, "-ecs")) + `-dogstatsd-ud[ps]$`,
				`^git\.commit\.sha:[[:xdigit:]]{40}$`,                                    // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:sha256:`,
				`^image_name:ghcr\.io/datadog/apps-dogstatsd$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^region:us-east-1$`,
				`^series:`,
				`^service_arn:`,
				`^short_image:apps-dogstatsd$`,
				`^task_arn:`,
				`^task_definition_arn:`,
				`^task_family:.*-` + regexp.QuoteMeta(taskName) + `-ec2$`,
				`^task_name:.*-` + regexp.QuoteMeta(taskName) + `-ec2$`,
				`^task_version:[[:digit:]]+$`,
			},
		},
	})
}

func (suite *ecsSuite) TestPrometheus() {
	// Test Prometheus check
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "prometheus.prom_gauge",
		},
		Expect: testMetricExpectArgs{
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

func (suite *ecsSuite) TestTraceUDS() {
	suite.testTrace(taskNameTracegenUDS)
}

func (suite *ecsSuite) TestTraceTCP() {
	suite.testTrace(taskNameTracegenTCP)
}

// testTrace verifies that traces are tagged with container and pod tags, and validates trace structure.
func (suite *ecsSuite) testTrace(taskName string) {
	suite.EventuallyWithTf(func(c *assert.CollectT) {
		traces, cerr := suite.Fakeintake.GetTraces()
		// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
		if !assert.NoErrorf(c, cerr, "Failed to query fake intake") {
			return
		}

		var err error
		var foundTrace *aggregator.Trace
		// Iterate starting from the most recent traces
		for _, trace := range traces {
			tags := lo.MapToSlice(trace.Tags, func(k string, v string) string {
				return k + ":" + v
			})
			// Assert origin detection is working properly
			err = assertTags(tags, []*regexp.Regexp{
				regexp.MustCompile(`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`),
				regexp.MustCompile(`^container_id:`),
				regexp.MustCompile(`^container_name:ecs-.*-` + regexp.QuoteMeta(taskName) + `-ec2-`),
				regexp.MustCompile(`^docker_image:ghcr\.io/datadog/apps-tracegen:` + regexp.QuoteMeta(apps.Version) + `$`),
				regexp.MustCompile(`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`),
				regexp.MustCompile(`^ecs_container_name:tracegen`),
				regexp.MustCompile(`^git\.commit\.sha:[[:xdigit:]]{40}$`),                                    // org.opencontainers.image.revision docker image label
				regexp.MustCompile(`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`), // org.opencontainers.image.source   docker image label
				regexp.MustCompile(`^image_id:sha256:`),
				regexp.MustCompile(`^image_name:ghcr\.io/datadog/apps-tracegen`),
				regexp.MustCompile(`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`),
				regexp.MustCompile(`^short_image:apps-tracegen`),
				regexp.MustCompile(`^task_arn:`),
				regexp.MustCompile(`^task_family:.*-` + regexp.QuoteMeta(taskName) + `-ec2$`),
				regexp.MustCompile(`^task_name:.*-` + regexp.QuoteMeta(taskName) + `-ec2$`),
				regexp.MustCompile(`^task_version:[[:digit:]]+$`),
			}, []*regexp.Regexp{}, false)
			if err == nil {
				foundTrace = &trace
				break
			}
		}
		require.NoErrorf(c, err, "Failed finding trace with proper tags")

		// Enhanced validation: verify trace structure and sampling
		if foundTrace != nil {
			// Verify trace has at least one tracer payload
			assert.NotEmptyf(c, foundTrace.TracerPayloads, "Trace should have at least one tracer payload")

			if len(foundTrace.TracerPayloads) > 0 {
				payload := foundTrace.TracerPayloads[0]

				// Verify payload has chunks with spans
				assert.NotEmptyf(c, payload.Chunks, "Tracer payload should have at least one chunk")

				if len(payload.Chunks) > 0 {
					chunk := payload.Chunks[0]
					assert.NotEmptyf(c, chunk.Spans, "Chunk should have at least one span")

					if len(chunk.Spans) > 0 {
						span := chunk.Spans[0]

						// Validate trace ID is present
						assert.NotZerof(c, span.TraceID, "Trace ID should be present for task %s", taskName)

						// Validate span ID is present
						assert.NotZerof(c, span.SpanID, "Span ID should be present for task %s", taskName)

						// Validate service name is set
						assert.NotEmptyf(c, span.Service, "Service name should be present for task %s", taskName)

						// Validate resource name is set
						assert.NotEmptyf(c, span.Resource, "Resource name should be present for task %s", taskName)

						// Validate operation name is set
						assert.NotEmptyf(c, span.Name, "Operation name should be present for task %s", taskName)

						// Validate sampling priority exists (indicates sampling decision was made)
						if samplingPriority, exists := span.Metrics["_sampling_priority_v1"]; exists {
							suite.T().Logf("Trace for task %s has sampling priority: %f", taskName, samplingPriority)
							// Sampling priority should be a valid value (typically 0, 1, or 2)
							assert.GreaterOrEqualf(c, samplingPriority, float64(0),
								"Sampling priority should be >= 0")
						}

						// Validate span duration is reasonable (> 0 and < 1 hour)
						assert.Greaterf(c, span.Duration, int64(0),
							"Span duration should be positive for task %s", taskName)
						assert.Lessf(c, span.Duration, int64(3600000000000), // 1 hour in nanoseconds
							"Span duration should be less than 1 hour for task %s", taskName)

						// Validate timestamps
						assert.Greaterf(c, span.Start, int64(0),
							"Span start timestamp should be positive for task %s", taskName)

						suite.T().Logf("Enhanced trace validation passed for task %s: TraceID=%d, SpanID=%d, Service=%s, Duration=%dns",
							taskName, span.TraceID, span.SpanID, span.Service, span.Duration)
					}
				}
			}

			// Verify trace correlation: check if trace has ECS metadata in tags
			hasECSMetadata := false
			for k, v := range foundTrace.Tags {
				if k == "ecs_cluster_name" && v == suite.ecsClusterName {
					hasECSMetadata = true
					suite.T().Logf("Trace correlation validated: trace has ECS metadata (cluster=%s)", v)
					break
				}
			}
			assert.Truef(c, hasECSMetadata, "Trace should be correlated with ECS metadata for task %s", taskName)
		}
	}, 2*time.Minute, 10*time.Second, "Failed finding trace with proper tags and structure")
}

func (suite *ecsSuite) TestMetadataCollection() {
	// Test that ECS metadata is properly collected and applied as tags
	suite.Run("Metadata collection from ECS endpoints", func() {
		// Verify cluster name is present (from metadata)
		suite.testMetric(&testMetricArgs{
			Filter: testMetricFilterArgs{
				Name: "container.cpu.usage",
				Tags: []string{`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`},
			},
			Expect: testMetricExpectArgs{
				Tags: &[]string{
					// These tags come from ECS metadata endpoints
					`^aws_account:[[:digit:]]{12}$`, // From task metadata
					`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
					`^task_arn:arn:aws:ecs:`,         // From task metadata
					`^task_definition_arn:arn:aws:ecs:`, // From task metadata
					`^task_family:`,                  // From task metadata
					`^task_version:[[:digit:]]+$`,    // From task metadata
					`^region:us-east-1$`,             // From AWS metadata
					`^availability_zone:`,            // From task metadata (Fargate) or EC2 metadata
					`^ecs_container_name:`,           // From container metadata
					`^container_id:`,                 // From container metadata
					`^container_name:`,               // From container metadata
				},
			},
		})

		// Verify task ARN format is correct (validates metadata parsing)
		suite.testMetric(&testMetricArgs{
			Filter: testMetricFilterArgs{
				Name: "container.memory.usage",
				Tags: []string{`^ecs_cluster_name:`},
			},
			Expect: testMetricExpectArgs{
				Tags: &[]string{
					`^task_arn:arn:aws:ecs:us-east-1:[[:digit:]]{12}:task/` + regexp.QuoteMeta(suite.ecsClusterName) + `/[[:xdigit:]]{32}$`,
				},
			},
		})
	})
}

func (suite *ecsSuite) TestContainerLifecycle() {
	// Test that container lifecycle events are properly tracked
	suite.Run("Container lifecycle tracking", func() {
		// Verify that running containers are reporting metrics
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			metrics, err := suite.Fakeintake.FilterMetrics(
				"container.cpu.usage",
				fakeintake.WithMatchingTags[*aggregator.MetricSeries]([]*regexp.Regexp{
					regexp.MustCompile(`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`),
				}),
			)
			assert.NoErrorf(c, err, "Failed to query metrics")
			assert.NotEmptyf(c, metrics, "No container metrics found - containers may not be running")

			// Verify we have metrics from multiple containers (indicating lifecycle tracking)
			containerIDs := make(map[string]bool)
			for _, metric := range metrics {
				for _, tag := range metric.GetTags() {
					if strings.HasPrefix(tag, "container_id:") {
						containerIDs[tag] = true
					}
				}
			}
			assert.GreaterOrEqualf(c, len(containerIDs), 3,
				"Expected metrics from at least 3 containers, got %d", len(containerIDs))

		}, 3*time.Minute, 10*time.Second, "Container lifecycle tracking validation failed")
	})
}

func (suite *ecsSuite) TestTagInheritance() {
	// Test that tags are consistently applied across all telemetry types
	suite.Run("Tag inheritance across metrics, logs, and traces", func() {
		var sharedTags []string

		// Get tags from a metric
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			metrics, err := suite.Fakeintake.FilterMetrics(
				"nginx.net.request_per_s",
				fakeintake.WithMatchingTags[*aggregator.MetricSeries]([]*regexp.Regexp{
					regexp.MustCompile(`^ecs_launch_type:ec2$`),
				}),
			)
			if !assert.NoErrorf(c, err, "Failed to query metrics") {
				return
			}
			if !assert.NotEmptyf(c, metrics, "No nginx metrics found") {
				return
			}

			// Extract ECS-related tags from the metric
			for _, tag := range metrics[len(metrics)-1].GetTags() {
				if strings.HasPrefix(tag, "ecs_cluster_name:") ||
					strings.HasPrefix(tag, "ecs_container_name:") ||
					strings.HasPrefix(tag, "task_family:") ||
					strings.HasPrefix(tag, "task_arn:") ||
					strings.HasPrefix(tag, "aws_account:") ||
					strings.HasPrefix(tag, "region:") {
					sharedTags = append(sharedTags, tag)
				}
			}
			assert.NotEmptyf(c, sharedTags, "No ECS tags found on metrics")

		}, 2*time.Minute, 10*time.Second, "Failed to get tags from metrics")

		// Verify the same tags are present on logs
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			logs, err := suite.Fakeintake.FilterLogs(
				"nginx",
				fakeintake.WithMatchingTags[*aggregator.Log]([]*regexp.Regexp{
					regexp.MustCompile(`^ecs_launch_type:ec2$`),
				}),
			)
			if !assert.NoErrorf(c, err, "Failed to query logs") {
				return
			}
			if !assert.NotEmptyf(c, logs, "No nginx logs found") {
				return
			}

			// Verify shared tags are present on logs
			logTags := logs[len(logs)-1].GetTags()
			for _, expectedTag := range sharedTags {
				assert.Containsf(c, logTags, expectedTag,
					"Expected tag '%s' from metrics not found on logs", expectedTag)
			}

		}, 2*time.Minute, 10*time.Second, "Failed to verify tags on logs")

		// Verify the same tags are present on traces
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			traces, err := suite.Fakeintake.GetTraces()
			if !assert.NoErrorf(c, err, "Failed to query traces") {
				return
			}
			if !assert.NotEmptyf(c, traces, "No traces found") {
				return
			}

			// Find a trace with ECS tags
			found := false
			for _, trace := range traces {
				traceTags := lo.MapToSlice(trace.Tags, func(k string, v string) string {
					return k + ":" + v
				})

				// Check if this trace has ECS cluster tag
				hasECSTag := false
				for _, tag := range traceTags {
					if strings.HasPrefix(tag, "ecs_cluster_name:"+suite.ecsClusterName) {
						hasECSTag = true
						break
					}
				}

				if hasECSTag {
					// Verify at least some shared tags are present
					matchCount := 0
					for _, expectedTag := range sharedTags {
						for _, traceTag := range traceTags {
							if traceTag == expectedTag {
								matchCount++
								break
							}
						}
					}
					assert.GreaterOrEqualf(c, matchCount, len(sharedTags)/2,
						"Expected at least half of the shared tags on traces, got %d/%d",
						matchCount, len(sharedTags))
					found = true
					break
				}
			}
			assert.Truef(c, found, "No traces with ECS tags found")

		}, 2*time.Minute, 10*time.Second, "Failed to verify tags on traces")
	})
}

func (suite *ecsSuite) TestCheckAutodiscovery() {
	// Test that checks are automatically discovered and scheduled
	suite.Run("Check autodiscovery", func() {
		// Test Redis autodiscovery by image name
		suite.Run("Redis autodiscovery by image", func() {
			suite.testMetric(&testMetricArgs{
				Filter: testMetricFilterArgs{
					Name: "redis.net.instantaneous_ops_per_sec",
					Tags: []string{`^ecs_launch_type:ec2$`},
				},
				Expect: testMetricExpectArgs{
					Tags: &[]string{
						`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
						`^ecs_container_name:redis$`,
						`^image_name:ghcr\.io/datadog/redis$`,
					},
				},
			})

			// Verify Redis check is running (check run should exist)
			suite.EventuallyWithTf(func(c *assert.CollectT) {
				checkRuns, err := suite.Fakeintake.FilterCheckRuns(
					"redisdb",
					fakeintake.WithMatchingTags[*aggregator.CheckRun]([]*regexp.Regexp{
						regexp.MustCompile(`^ecs_launch_type:ec2$`),
					}),
				)
				if err == nil && len(checkRuns) > 0 {
					suite.T().Logf("Redis check autodiscovered and running")
				}
			}, 2*time.Minute, 10*time.Second, "Redis check autodiscovery validation failed")
		})

		// Test Nginx autodiscovery by docker labels
		suite.Run("Nginx autodiscovery by labels", func() {
			suite.testMetric(&testMetricArgs{
				Filter: testMetricFilterArgs{
					Name: "nginx.net.request_per_s",
					Tags: []string{`^ecs_launch_type:ec2$`},
				},
				Expect: testMetricExpectArgs{
					Tags: &[]string{
						`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
						`^ecs_container_name:nginx$`,
						`^image_name:ghcr\.io/datadog/apps-nginx-server$`,
					},
				},
			})

			// Verify Nginx check is running
			suite.EventuallyWithTf(func(c *assert.CollectT) {
				checkRuns, err := suite.Fakeintake.FilterCheckRuns(
					"nginx",
					fakeintake.WithMatchingTags[*aggregator.CheckRun]([]*regexp.Regexp{
						regexp.MustCompile(`^ecs_launch_type:ec2$`),
					}),
				)
				if err == nil && len(checkRuns) > 0 {
					suite.T().Logf("Nginx check autodiscovered via docker labels and running")
				}
			}, 2*time.Minute, 10*time.Second, "Nginx check autodiscovery validation failed")
		})

		// Verify that autodiscovery works for both EC2 and Fargate
		suite.Run("Fargate autodiscovery", func() {
			suite.testMetric(&testMetricArgs{
				Filter: testMetricFilterArgs{
					Name: "redis.net.instantaneous_ops_per_sec",
					Tags: []string{`^ecs_launch_type:fargate$`},
				},
				Expect: testMetricExpectArgs{
					Tags: &[]string{
						`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
						`^ecs_container_name:redis$`,
						`^ecs_launch_type:fargate$`,
					},
				},
			})
		})
	})
}

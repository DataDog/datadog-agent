// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecs

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	awsecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/stretchr/testify/assert"

	scenecs "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"
	provecs "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/ecs"
)

type ecsPlatformSuite struct {
	BaseSuite[environments.ECS]
	ecsClusterName string
}

func TestECSPlatformSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &ecsPlatformSuite{}, e2e.WithProvisioner(provecs.Provisioner(
		provecs.WithRunOptions(
			scenecs.WithECSOptions(
				scenecs.WithFargateCapacityProvider(),
				scenecs.WithLinuxNodeGroup(),
				scenecs.WithWindowsNodeGroup(),
			),
			scenecs.WithTestingWorkload(),
		),
	)))
}

func (suite *ecsPlatformSuite) SetupSuite() {
	suite.BaseSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
	suite.ecsClusterName = suite.Env().ECSCluster.ClusterName
	suite.ClusterName = suite.Env().ECSCluster.ClusterName
}

// Test00UpAndRunning is a foundation test that ensures all ECS tasks and services
// are in RUNNING state before other tests execute. The 00 prefix ensures it runs first.
func (suite *ecsPlatformSuite) Test00UpAndRunning() {
	ctx := suite.T().Context()

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

func (suite *ecsPlatformSuite) TestWindowsFargate() {
	suite.AssertCheckRun(&TestCheckRunArgs{
		Filter: TestCheckRunFilterArgs{
			Name: "http.can_connect",
			Tags: []string{
				"^ecs_launch_type:fargate$",
				"^container_name:aspnetsample$",
			},
		},
		Expect: TestCheckRunExpectArgs{
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
	suite.AssertMetric(&TestMetricArgs{
		Filter: TestMetricFilterArgs{
			Name: "container.cpu.usage",
			Tags: []string{
				"^ecs_container_name:aspnetsample$",
			},
		},
		Expect: TestMetricExpectArgs{
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

func (suite *ecsPlatformSuite) TestCPU() {
	// Test CPU metrics
	suite.AssertMetric(&TestMetricArgs{
		Filter: TestMetricFilterArgs{
			Name: "container.cpu.usage",
			Tags: []string{
				"^ecs_container_name:stress-ng$",
			},
		},
		Expect: TestMetricExpectArgs{
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
			Value: &TestMetricExpectValueArgs{
				Max: 160000000,
				Min: 120000000,
			},
		},
	})
}

func (suite *ecsPlatformSuite) TestContainerLifecycle() {
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

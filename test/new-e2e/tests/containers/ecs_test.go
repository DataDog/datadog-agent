// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ecs"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	awsecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/fatih/color"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ecsSuite struct {
	baseSuite

	ecsClusterName string
}

func TestECSSuite(t *testing.T) {
	suite.Run(t, &ecsSuite{})
}

func (suite *ecsSuite) SetupSuite() {
	ctx := context.Background()

	// Creating the stack
	stackConfig := runner.ConfigMap{
		"ddinfra:aws/ecs/linuxECSOptimizedNodeGroup": auto.ConfigValue{Value: "true"},
		"ddinfra:aws/ecs/linuxBottlerocketNodeGroup": auto.ConfigValue{Value: "true"},
		"ddinfra:aws/ecs/windowsLTSCNodeGroup":       auto.ConfigValue{Value: "true"},
		"ddagent:deploy":                             auto.ConfigValue{Value: "true"},
		"ddagent:fakeintake":                         auto.ConfigValue{Value: "true"},
		"ddtestworkload:deploy":                      auto.ConfigValue{Value: "true"},
	}

	_, stackOutput, err := infra.GetStackManager().GetStack(ctx, "ecs-cluster", stackConfig, ecs.Run, false)
	suite.Require().NoError(err)

	fakeintakeHost := stackOutput.Outputs["fakeintake-host"].Value.(string)
	suite.Fakeintake = fakeintake.NewClient(fmt.Sprintf("http://%s", fakeintakeHost))
	suite.ecsClusterName = stackOutput.Outputs["ecs-cluster-name"].Value.(string)
	suite.clusterName = suite.ecsClusterName

	suite.baseSuite.SetupSuite()
}

func (suite *ecsSuite) TearDownSuite() {
	suite.baseSuite.TearDownSuite()

	color.NoColor = false
	c := color.New(color.Bold).SprintfFunc()
	suite.T().Log(c("The data produced and asserted by these tests can be viewed on this dashboard:"))
	c = color.New(color.Bold, color.FgBlue).SprintfFunc()
	suite.T().Log(c("https://dddev.datadoghq.com/dashboard/mnw-tdr-jd8/e2e-tests-containers-ecs?refresh_mode=paused&tpl_var_ecs_cluster_name%%5B0%%5D=%s&from_ts=%d&to_ts=%d&live=false",
		suite.ecsClusterName,
		suite.startTime.UnixMilli(),
		suite.endTime.UnixMilli(),
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
		suite.EventuallyWithTf(func(collect *assert.CollectT) {
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
				if err != nil {
					collect.Errorf("Failed to list ECS services: %w", err)
					return
				}

				nextToken = servicesList.NextToken

				servicesDescription, err := client.DescribeServices(ctx, &awsecs.DescribeServicesInput{
					Cluster:  &suite.ecsClusterName,
					Services: servicesList.ServiceArns,
				})
				if err != nil {
					collect.Errorf("Failed to describe ECS services %v: %w", servicesList.ServiceArns, err)
					continue
				}

				for _, serviceDescription := range servicesDescription.Services {
					if serviceDescription.DesiredCount == 0 {
						collect.Errorf("ECS service %s has no task.", *serviceDescription.ServiceName)
					}

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
						if err != nil {
							collect.Errorf("Failed to list ECS tasks for service %s: %w", *serviceDescription.ServiceName, err)
							break
						}

						nextToken = tasksList.NextToken

						tasksDescription, err := client.DescribeTasks(ctx, &awsecs.DescribeTasksInput{
							Cluster: &suite.ecsClusterName,
							Tasks:   tasksList.TaskArns,
						})
						if err != nil {
							collect.Errorf("Failed to describe ECS tasks %v: %w", tasksList.TaskArns, err)
							continue
						}

						for _, taskDescription := range tasksDescription.Tasks {
							if *taskDescription.LastStatus != string(awsecstypes.DesiredStatusRunning) ||
								taskDescription.HealthStatus == awsecstypes.HealthStatusUnhealthy {
								collect.Errorf("Task %s of service %s is %s %s.",
									*taskDescription.TaskArn,
									*serviceDescription.ServiceName,
									*taskDescription.LastStatus,
									taskDescription.HealthStatus)
							}
						}
					}
				}
			}
		}, 5*time.Minute, 10*time.Second, "Not all tasks became ready in time.")
	})
}

func (suite *ecsSuite) TestNginx() {
	// `nginx` check is configured via docker labels
	// Test it is properly scheduled
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "nginx.net.request_per_s",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^container_id:`,
				`^container_name:ecs-.*-nginx-ec2-`,
				`^docker_image:ghcr.io/datadog/apps-nginx-server:main$`,
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:nginx$`,
				`^git.commit.sha:`,                                                       // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:sha256:`,
				`^image_name:ghcr.io/datadog/apps-nginx-server$`,
				`^image_tag:main$`,
				`^nginx_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^short_image:apps-nginx-server$`,
				`^task_arn:`,
				`^task_family:.*-nginx-ec2$`,
				`^task_name:.*-nginx-ec2$`,
				`^task_version:[[:digit:]]+$`,
			},
		},
	})
}

func (suite *ecsSuite) TestRedis() {
	// `redis` check is auto-configured due to image name
	// Test it is properly scheduled
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "redis.net.instantaneous_ops_per_sec",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^container_id:`,
				`^container_name:ecs-.*-redis-ec2-`,
				`^docker_image:redis:latest$`,
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:redis$`,
				`^image_id:sha256:`,
				`^image_name:redis$`,
				`^image_tag:latest$`,
				`^redis_host:`,
				`^redis_port:6379$`,
				`^redis_role:master$`,
				`^short_image:redis$`,
				`^task_arn:`,
				`^task_family:.*-redis-ec2$`,
				`^task_name:.*-redis-ec2$`,
				`^task_version:[[:digit:]]+$`,
			},
		},
	})
}

func (suite *ecsSuite) TestDogstatsd() {
	// Test dogstatsd origin detection with UDS
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "custom.metric",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^container_id:`,
				`^container_name:ecs-.*-dogstatsd-uds-ec2-`,
				`^docker_image:ghcr.io/datadog/apps-dogstatsd:main$`,
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:dogstatsd$`,
				`^git.commit.sha:`,                                                       // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:sha256:`,
				`^image_name:ghcr.io/datadog/apps-dogstatsd$`,
				`^image_tag:main$`,
				`^series:`,
				`^short_image:apps-dogstatsd$`,
				`^task_arn:`,
				`^task_family:.*-dogstatsd-uds-ec2$`,
				`^task_name:.*-dogstatsd-uds-ec2$`,
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
				`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^container_id:`,
				`^container_name:ecs-.*-prometheus-ec2-`,
				`^docker_image:ghcr.io/datadog/apps-prometheus:main$`,
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:prometheus$`,
				`^endpoint:http://.*:8080/metrics$`,
				`^git.commit.sha:`,                                                       // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:sha256:`,
				`^image_name:ghcr.io/datadog/apps-prometheus$`,
				`^image_tag:main$`,
				`^series:`,
				`^short_image:apps-prometheus$`,
				`^task_arn:`,
				`^task_family:.*-prometheus-ec2$`,
				`^task_name:.*-prometheus-ec2$`,
				`^task_version:[[:digit:]]+$`,
			},
		},
	})
}

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
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestECSSuite(t *testing.T) {
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
	require.NoError(t, err)

	t.Cleanup(func() {
		infra.GetStackManager().DeleteStack(ctx, "ecs-cluster")
	})

	fakeintakeHost := stackOutput.Outputs["fakeintake-host"].Value.(string)

	startTime := time.Now()

	suite.Run(t, &ecsSuite{
		baseSuite: baseSuite{
			Fakeintake: fakeintake.NewClient(fmt.Sprintf("http://%s", fakeintakeHost)),
		},
		ecsClusterName: stackOutput.Outputs["ecs-cluster-name"].Value.(string),
	})

	endTime := time.Now()

	color.NoColor = false
	c := color.New(color.Bold).SprintfFunc()
	t.Log(c("The data produced and asserted by these tests can be viewed on this dashboard:"))
	c = color.New(color.Bold, color.FgBlue).SprintfFunc()
	t.Log(c("https://dddev.datadoghq.com/dashboard/mnw-tdr-jd8/e2e-tests-containers-ecs?refresh_mode=paused&tpl_var_ecs_cluster_name%%5B0%%5D=%s&from_ts=%d&to_ts=%d&live=false",
		stackOutput.Outputs["ecs-cluster-name"].Value.(string),
		startTime.UnixMilli(),
		endTime.UnixMilli(),
	))
}

type ecsSuite struct {
	baseSuite

	ecsClusterName string
}

func (suite *ecsSuite) TestAgent() {
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
	suite.testMetric("nginx.net.request_per_s",
		[]string{},
		[]*regexp.Regexp{
			regexp.MustCompile(`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`),
			regexp.MustCompile(`^container_id:`),
			regexp.MustCompile(`^container_name:ecs-.*-nginx-ec2-`),
			regexp.MustCompile(`^docker_image:ghcr.io/datadog/apps-nginx-server:main$`),
			regexp.MustCompile(`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`),
			regexp.MustCompile(`^ecs_container_name:nginx$`),
			regexp.MustCompile(`^git.commit.sha:`),                                                       // org.opencontainers.image.revision docker image label
			regexp.MustCompile(`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`), // org.opencontainers.image.source   docker image label
			regexp.MustCompile(`^image_id:sha256:`),
			regexp.MustCompile(`^image_name:ghcr.io/datadog/apps-nginx-server$`),
			regexp.MustCompile(`^image_tag:main$`),
			regexp.MustCompile(`^nginx_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`),
			regexp.MustCompile(`^short_image:apps-nginx-server$`),
			regexp.MustCompile(`^task_arn:`),
			regexp.MustCompile(`^task_family:.*-nginx-ec2$`),
			regexp.MustCompile(`^task_name:.*-nginx-ec2$`),
			regexp.MustCompile(`^task_version:[[:digit:]]+$`),
		},
	)
}

func (suite *ecsSuite) TestRedis() {
	// `redis` check is auto-configured due to image name
	// Test it is properly scheduled
	suite.testMetric("redis.net.instantaneous_ops_per_sec",
		[]string{},
		[]*regexp.Regexp{
			regexp.MustCompile(`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`),
			regexp.MustCompile(`^container_id:`),
			regexp.MustCompile(`^container_name:ecs-.*-redis-ec2-`),
			regexp.MustCompile(`^docker_image:redis:latest$`),
			regexp.MustCompile(`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`),
			regexp.MustCompile(`^ecs_container_name:redis$`),
			regexp.MustCompile(`^image_id:sha256:`),
			regexp.MustCompile(`^image_name:redis$`),
			regexp.MustCompile(`^image_tag:latest$`),
			regexp.MustCompile(`^redis_host:`),
			regexp.MustCompile(`^redis_port:6379$`),
			regexp.MustCompile(`^redis_role:master$`),
			regexp.MustCompile(`^short_image:redis$`),
			regexp.MustCompile(`^task_arn:`),
			regexp.MustCompile(`^task_family:.*-redis-ec2$`),
			regexp.MustCompile(`^task_name:.*-redis-ec2$`),
			regexp.MustCompile(`^task_version:[[:digit:]]+$`),
		},
	)

}

func (suite *ecsSuite) TestDogstatsd() {
	// Test dogstatsd origin detection with UDS
	suite.testMetric("custom.metric",
		[]string{},
		[]*regexp.Regexp{
			regexp.MustCompile(`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`),
			regexp.MustCompile(`^container_id:`),
			regexp.MustCompile(`^container_name:ecs-.*-dogstatsd-uds-ec2-`),
			regexp.MustCompile(`^docker_image:ghcr.io/datadog/apps-dogstatsd:main$`),
			regexp.MustCompile(`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`),
			regexp.MustCompile(`^ecs_container_name:dogstatsd$`),
			regexp.MustCompile(`^git.commit.sha:`),                                                       // org.opencontainers.image.revision docker image label
			regexp.MustCompile(`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`), // org.opencontainers.image.source   docker image label
			regexp.MustCompile(`^image_id:sha256:`),
			regexp.MustCompile(`^image_name:ghcr.io/datadog/apps-dogstatsd$`),
			regexp.MustCompile(`^image_tag:main$`),
			regexp.MustCompile(`^series:`),
			regexp.MustCompile(`^short_image:apps-dogstatsd$`),
			regexp.MustCompile(`^task_arn:`),
			regexp.MustCompile(`^task_family:.*-dogstatsd-uds-ec2$`),
			regexp.MustCompile(`^task_name:.*-dogstatsd-uds-ec2$`),
			regexp.MustCompile(`^task_version:[[:digit:]]+$`),
		},
	)
}

func (suite *ecsSuite) TestPrometheus() {
	// Test Prometheus check
	suite.testMetric("prometheus.prom_gauge",
		[]string{},
		[]*regexp.Regexp{
			regexp.MustCompile(`^cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`),
			regexp.MustCompile(`^container_id:`),
			regexp.MustCompile(`^container_name:ecs-.*-prometheus-ec2-`),
			regexp.MustCompile(`^docker_image:ghcr.io/datadog/apps-prometheus:main$`),
			regexp.MustCompile(`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`),
			regexp.MustCompile(`^ecs_container_name:prometheus$`),
			regexp.MustCompile(`^endpoint:http://.*:8080/metrics$`),
			regexp.MustCompile(`^git.commit.sha:`),                                                       // org.opencontainers.image.revision docker image label
			regexp.MustCompile(`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`), // org.opencontainers.image.source   docker image label
			regexp.MustCompile(`^image_id:sha256:`),
			regexp.MustCompile(`^image_name:ghcr.io/datadog/apps-prometheus$`),
			regexp.MustCompile(`^image_tag:main$`),
			regexp.MustCompile(`^series:`),
			regexp.MustCompile(`^short_image:apps-prometheus$`),
			regexp.MustCompile(`^task_arn:`),
			regexp.MustCompile(`^task_family:.*-prometheus-ec2$`),
			regexp.MustCompile(`^task_name:.*-prometheus-ec2$`),
			regexp.MustCompile(`^task_version:[[:digit:]]+$`),
		},
	)
}

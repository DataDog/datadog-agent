// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"errors"
	"fmt"
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
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"

	provecs "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
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
	windowsEnabled bool
}

func TestECSSuite(t *testing.T) {
	suite := &ecsSuite{}

	ecsOptions := []scenecs.Option{
		scenecs.WithFargateCapacityProvider(),
		scenecs.WithLinuxNodeGroup(),
		scenecs.WithLinuxBottleRocketNodeGroup(),
	}

	runOptions := []scenecs.RunOption{
		scenecs.WithFakeIntakeOptions(
			// The task memory is shared with a datadog-agent sidecar. With 31m of retention, a smaller
			// task gets OOM-killed and replaced by one with a new IP, failing all remaining queries.
			fakeintake.WithMemory(4096),
			fakeintake.WithRetentionPeriod("31m"),
		),
		scenecs.WithTestingWorkload(),
	}

	skipWindows, err := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.SkipWindows, false)
	require.NoError(t, err, "failed to get %s parameter", parameters.SkipWindows)
	if !skipWindows {
		// WithWindowsNodeGroup is the dedicated ECS option to opt-in to Windows
		// infrastructure and workloads (Windows EC2 nodes + Windows Fargate apps).
		ecsOptions = append(ecsOptions, scenecs.WithWindowsNodeGroup())

		suite.windowsEnabled = true
	}

	runOptions = append(runOptions, scenecs.WithECSOptions(ecsOptions...))

	e2e.Run(t, suite, e2e.WithProvisioner(provecs.Provisioner(
		provecs.WithRunOptions(runOptions...),
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
				require.NoErrorf(c, err, "Failed to list ECS services")

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
						require.NoErrorf(c, err, "Failed to list ECS tasks for service %s", *serviceDescription.ServiceName)

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

// containerMainProcessesCommand prints the comm of each process in this container's mount
// namespace whose parent is outside it. That includes the main process and any ECS exec session,
// which the runtime starts beside it. With pid:host, /proc/1 is the host init, so it is not used.
const containerMainProcessesCommand = `sh -c 'echo SUPERVISOR_PROCS; self=$(readlink /proc/self/ns/mnt); for d in /proc/[0-9]*; do pid=${d##*/}; ns=$(readlink /proc/$pid/ns/mnt 2>/dev/null) || continue; test "$ns" = "$self" || continue; ppid=$(sed -n "s/^PPid:[[:space:]]*//p" /proc/$pid/status 2>/dev/null); test -n "$ppid" || continue; pns=$(readlink /proc/$ppid/ns/mnt 2>/dev/null || true); if test -z "$pns" || test "$pns" != "$self"; then cat /proc/$pid/comm 2>/dev/null; fi; done; echo SUPERVISOR_END'`

// Test01SupervisorIsProcmgr checks that each Linux Agent container is supervised by
// dd-procmgrd. s6-svscan or s6-supervise in that role fails the test.
func (suite *ecsSuite) Test01SupervisorIsProcmgr() {
	suite.EventuallyWithTf(func(c *assert.CollectT) {
		tasks, err := suite.linuxAgentTasks("ec2-linux-dd-agent")
		if !assert.NoError(c, err) || !assert.NotEmpty(c, tasks, "no running EC2 Agent daemon tasks") {
			return
		}
		for _, task := range tasks {
			out, err := suite.execMainProcesses(task.taskARN)
			if !assert.NoError(c, err) {
				return
			}
			procs, err := supervisorProcs(out)
			if !assert.NoErrorf(c, err, "exec into %s: %v\n%s", task.taskARN, err, out) {
				return
			}
			assert.Containsf(c, procs, "dd-procmgrd", "EC2 Agent task %s main processes: %v", task.taskARN, procs)
			assert.NotContainsf(c, procs, "s6-svscan", "EC2 Agent task %s is supervised by s6: %v", task.taskARN, procs)
			assert.NotContainsf(c, procs, "s6-supervise", "EC2 Agent task %s is supervised by s6: %v", task.taskARN, procs)
		}
	}, 5*time.Minute, 15*time.Second, "EC2 Agent tasks are not running under dd-procmgrd")

	// Fargate task definitions set pidMode=task. ECS exec cannot enter those
	// tasks, so the process list the Agent reports is the signal instead.
	suite.EventuallyWithTf(func(c *assert.CollectT) {
		nginx, err := suite.linuxAgentTasks("nginx-fg")
		if !assert.NoError(c, err) || !assert.NotEmpty(c, nginx, "no running nginx-fg Agent containers") {
			return
		}
		redis, err := suite.linuxAgentTasks("redis-fg")
		if !assert.NoError(c, err) || !assert.NotEmpty(c, redis, "no running redis-fg Agent containers") {
			return
		}
		suite.assertFargateAgentsUseProcmgr(c, append(nginx, redis...))
	}, 5*time.Minute, 10*time.Second, "Fargate Agent containers are not running under dd-procmgrd")
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
				"^docker_image:.*/apps-nginx-server:" + regexp.QuoteMeta(apps.Version) + "$",
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:nginx$`,
				`^ecs_launch_type:ec2$`,
				`^ecs_service:` + regexp.QuoteMeta(strings.TrimSuffix(suite.ecsClusterName, "-ecs")) + `-nginx-ec2$`,
				`^git\.commit\.sha:[[:xdigit:]]{40}$`,                                    // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:sha256:`,
				"^image_name:.*/apps-nginx-server$",
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
				"^docker_image:.*/apps-nginx-server:" + regexp.QuoteMeta(apps.Version) + "$",
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:nginx$`,
				`^ecs_launch_type:ec2$`,
				`^ecs_service:` + regexp.QuoteMeta(strings.TrimSuffix(suite.ecsClusterName, "-ecs")) + `-nginx-ec2$`,
				`^git\.commit\.sha:[[:xdigit:]]{40}$`,                                    // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:sha256:`,
				"^image_name:.*/apps-nginx-server$",
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
				"^docker_image:.*/redis:" + regexp.QuoteMeta(apps.Version) + "$",
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:redis$`,
				`^ecs_service:` + regexp.QuoteMeta(strings.TrimSuffix(suite.ecsClusterName, "-ecs")) + `-redis-ec2$`,
				`^ecs_launch_type:ec2$`,
				`^git\.commit\.sha:[[:xdigit:]]{40}$`,                                    // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:sha256:`,
				"^image_name:.*/redis$",
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
				"^docker_image:.*/redis:" + regexp.QuoteMeta(apps.Version) + "$",
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:redis$`,
				`^ecs_launch_type:ec2$`,
				`^ecs_service:` + regexp.QuoteMeta(strings.TrimSuffix(suite.ecsClusterName, "-ecs")) + `-redis-ec2$`,
				`^git\.commit\.sha:[[:xdigit:]]{40}$`,                                    // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:sha256:`,
				"^image_name:.*/redis$",
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
				"^image_name:.*/apps-nginx-server$",
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
				"^image_name:.*/redis$",
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
	if !suite.windowsEnabled {
		suite.T().Skip("Skipping Windows test: WithWindowsNodeGroup() not set")
	}

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
				"^docker_image:.*/apps-stress-ng:" + regexp.QuoteMeta(apps.Version) + "$",
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:stress-ng$`,
				`^ecs_service:` + regexp.QuoteMeta(strings.TrimSuffix(suite.ecsClusterName, "-ecs")) + `-stress-ng$`,
				`^git\.commit\.sha:[[:xdigit:]]{40}$`,
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`,
				`^image_id:sha256:`,
				"^image_name:.*/apps-stress-ng$",
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
				Max:          160000000,
				Min:          140000000,
				RecentWindow: time.Minute,
			},
		},
		Timeout: 6 * time.Minute,
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
				"^docker_image:.*/apps-dogstatsd:" + regexp.QuoteMeta(apps.Version) + "$",
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:dogstatsd$`,
				`^ecs_service:` + regexp.QuoteMeta(strings.TrimSuffix(suite.ecsClusterName, "-ecs")) + `-dogstatsd-ud[ps]$`,
				`^git\.commit\.sha:[[:xdigit:]]{40}$`,                                    // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:sha256:`,
				"^image_name:.*/apps-dogstatsd$",
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
				"^docker_image:.*/apps-prometheus:" + regexp.QuoteMeta(apps.Version) + "$",
				`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`,
				`^ecs_container_name:prometheus$`,
				`^ecs_service:` + regexp.QuoteMeta(strings.TrimSuffix(suite.ecsClusterName, "-ecs")) + `-prometheus$`,
				`^endpoint:http://.*:8080/metrics$`,
				`^git\.commit\.sha:[[:xdigit:]]{40}$`,                                    // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:sha256:`,
				"^image_name:.*/apps-prometheus$",
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

// testTrace verifies that traces are tagged with container and pod tags.
func (suite *ecsSuite) testTrace(taskName string) {
	suite.EventuallyWithTf(func(c *assert.CollectT) {
		traces, cerr := suite.Fakeintake.GetTraces()
		require.NoErrorf(c, cerr, "Failed to query fake intake")

		var err error
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
				regexp.MustCompile("^docker_image:.*/apps-tracegen:" + regexp.QuoteMeta(apps.Version) + "$"),
				regexp.MustCompile(`^ecs_cluster_name:` + regexp.QuoteMeta(suite.ecsClusterName) + `$`),
				regexp.MustCompile(`^ecs_container_name:tracegen`),
				regexp.MustCompile(`^git\.commit\.sha:[[:xdigit:]]{40}$`),                                    // org.opencontainers.image.revision docker image label
				regexp.MustCompile(`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`), // org.opencontainers.image.source   docker image label
				regexp.MustCompile(`^image_id:sha256:`),
				regexp.MustCompile("^image_name:.*/apps-tracegen"),
				regexp.MustCompile(`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`),
				regexp.MustCompile(`^short_image:apps-tracegen`),
				regexp.MustCompile(`^task_arn:`),
				regexp.MustCompile(`^task_family:.*-` + regexp.QuoteMeta(taskName) + `-ec2$`),
				regexp.MustCompile(`^task_name:.*-` + regexp.QuoteMeta(taskName) + `-ec2$`),
				regexp.MustCompile(`^task_version:[[:digit:]]+$`),
			}, []*regexp.Regexp{}, false)
			if err == nil {
				break
			}
		}
		require.NoErrorf(c, err, "Failed finding trace with proper tags")
	}, 2*time.Minute, 10*time.Second, "Failed finding trace with proper tags")
}

func (suite *ecsSuite) TestHostTags() {
	// tag keys that are expected to be found on this docker env
	args := &testHostTags{
		ExpectedTags: []string{},
	}

	suite.testHostTags(args)
}

type ecsAgentTask struct {
	service   string
	taskARN   string
	runtimeID string
}

// linuxAgentTasks returns running datadog-agent containers for services whose name ends in
// serviceSuffix. Callers omit the Windows Fargate service, which does not use dd-procmgrd.
func (suite *ecsSuite) linuxAgentTasks(serviceSuffix string) ([]ecsAgentTask, error) {
	ctx := suite.T().Context()
	client := suite.Env().ECSCluster.ECSClient
	if client == nil {
		return nil, errors.New("ECS client is not initialized")
	}

	var tasks []ecsAgentTask
	var serviceToken *string
	for {
		services, err := client.ListServices(ctx, &awsecs.ListServicesInput{
			Cluster:    &suite.ecsClusterName,
			MaxResults: pointer.Ptr(int32(10)),
			NextToken:  serviceToken,
		})
		if err != nil {
			return nil, err
		}
		serviceToken = services.NextToken
		if len(services.ServiceArns) == 0 {
			if serviceToken == nil {
				break
			}
			continue
		}

		described, err := client.DescribeServices(ctx, &awsecs.DescribeServicesInput{
			Cluster:  &suite.ecsClusterName,
			Services: services.ServiceArns,
		})
		if err != nil {
			return nil, err
		}
		for _, service := range described.Services {
			if service.ServiceName == nil || !strings.HasSuffix(*service.ServiceName, serviceSuffix) {
				continue
			}
			running, err := suite.runningAgentTasks(*service.ServiceName)
			if err != nil {
				return nil, err
			}
			tasks = append(tasks, running...)
		}
		if serviceToken == nil {
			break
		}
	}
	return tasks, nil
}

func (suite *ecsSuite) runningAgentTasks(service string) ([]ecsAgentTask, error) {
	ctx := suite.T().Context()
	client := suite.Env().ECSCluster.ECSClient

	var tasks []ecsAgentTask
	var taskToken *string
	for {
		listed, err := client.ListTasks(ctx, &awsecs.ListTasksInput{
			Cluster:       &suite.ecsClusterName,
			ServiceName:   &service,
			DesiredStatus: awsecstypes.DesiredStatusRunning,
			MaxResults:    pointer.Ptr(int32(100)),
			NextToken:     taskToken,
		})
		if err != nil {
			return nil, err
		}
		taskToken = listed.NextToken
		if len(listed.TaskArns) > 0 {
			described, err := client.DescribeTasks(ctx, &awsecs.DescribeTasksInput{
				Cluster: &suite.ecsClusterName,
				Tasks:   listed.TaskArns,
			})
			if err != nil {
				return nil, err
			}
			for _, task := range described.Tasks {
				if task.TaskArn == nil {
					continue
				}
				runtimeID := ""
				for _, container := range task.Containers {
					if container.Name != nil && *container.Name == "datadog-agent" && container.RuntimeId != nil {
						runtimeID = *container.RuntimeId
					}
				}
				tasks = append(tasks, ecsAgentTask{
					service:   service,
					taskARN:   *task.TaskArn,
					runtimeID: runtimeID,
				})
			}
		}
		if taskToken == nil {
			break
		}
	}
	return tasks, nil
}

func (suite *ecsSuite) execMainProcesses(taskARN string) (string, error) {
	ctx := suite.T().Context()
	client := suite.Env().ECSCluster.ECSClient
	described, err := client.DescribeTasks(ctx, &awsecs.DescribeTasksInput{
		Cluster: &suite.ecsClusterName,
		Tasks:   []string{taskARN},
	})
	if err != nil {
		return "", err
	}
	if len(described.Tasks) == 0 {
		return "", fmt.Errorf("task %s not found", taskARN)
	}

	ready := false
	for _, container := range described.Tasks[0].Containers {
		if container.Name == nil || *container.Name != "datadog-agent" {
			continue
		}
		if len(container.ManagedAgents) == 0 || container.ManagedAgents[0].LastStatus == nil {
			return "", fmt.Errorf("exec agent not registered on %s", taskARN)
		}
		if *container.ManagedAgents[0].LastStatus != "RUNNING" {
			return "", fmt.Errorf("exec agent status %s on %s", *container.ManagedAgents[0].LastStatus, taskARN)
		}
		ready = true
	}
	if !ready {
		return "", fmt.Errorf("datadog-agent container not found on %s", taskARN)
	}
	return client.ExecCommand(taskARN, "datadog-agent", containerMainProcessesCommand)
}

func (suite *ecsSuite) assertFargateAgentsUseProcmgr(c *assert.CollectT, tasks []ecsAgentTask) {
	payloads, err := suite.Fakeintake.GetProcesses()
	if !assert.NoError(c, err) {
		return
	}

	for _, task := range tasks {
		host := "fargate_task:" + task.taskARN
		ids := map[string]struct{}{}
		if task.runtimeID != "" {
			ids[task.runtimeID] = struct{}{}
		}
		for _, payload := range payloads {
			if payload.HostName != host {
				continue
			}
			for _, container := range payload.Containers {
				if container == nil || container.Id == "" || !tagsIdentifyAgentTask(container.Tags, task) {
					continue
				}
				ids[container.Id] = struct{}{}
			}
		}

		comms := map[string]struct{}{}
		procmgr := false
		s6 := false
		// A PID can be reused between process check runs, so match the supervisor and the
		// core Agent within one run. A run can span several payloads that share a GroupId.
		type pidSets struct {
			procmgrPIDs     map[int32]struct{}
			agentParentPIDs map[int32]struct{}
		}
		groups := map[int32]*pidSets{}
		for _, payload := range payloads {
			if payload.HostName != host {
				continue
			}
			group := groups[payload.GroupId]
			if group == nil {
				group = &pidSets{
					procmgrPIDs:     map[int32]struct{}{},
					agentParentPIDs: map[int32]struct{}{},
				}
				groups[payload.GroupId] = group
			}
			for _, proc := range payload.Processes {
				if proc == nil || proc.Command == nil {
					continue
				}
				// A process without a container ID cannot be tied to the Agent container.
				if proc.ContainerId == "" || !containerIDKnown(proc.ContainerId, ids) {
					continue
				}
				name := processCommandName(proc.Command.Comm, proc.Command.Exe, proc.Command.Args)
				if name == "" {
					continue
				}
				comms[name] = struct{}{}
				switch name {
				case "dd-procmgrd":
					if proc.Pid > 0 {
						group.procmgrPIDs[proc.Pid] = struct{}{}
					}
				case "agent":
					if proc.Command.Ppid > 0 {
						group.agentParentPIDs[proc.Command.Ppid] = struct{}{}
					}
				case "s6-svscan", "s6-supervise":
					s6 = true
				}
			}
		}
		// dd-procmgrd starts the core Agent directly, so it is the Agent's parent.
		for _, group := range groups {
			for ppid := range group.agentParentPIDs {
				if _, ok := group.procmgrPIDs[ppid]; ok {
					procmgr = true
					break
				}
			}
			if procmgr {
				break
			}
		}
		assert.Truef(c, procmgr, "Fargate Agent task %s (%s) has no dd-procmgrd supervising the agent process, saw %v", task.taskARN, task.service, comms)
		assert.Falsef(c, s6, "Fargate Agent task %s (%s) is supervised by s6, saw %v", task.taskARN, task.service, comms)
	}
}

func tagsIdentifyAgentTask(tags []string, task ecsAgentTask) bool {
	taskID := task.taskARN
	if i := strings.LastIndex(task.taskARN, "/"); i >= 0 {
		taskID = task.taskARN[i+1:]
	}
	nameOK := false
	taskOK := false
	for _, tag := range tags {
		if tag == "ecs_container_name:datadog-agent" {
			nameOK = true
		}
		if taskID != "" && strings.HasPrefix(tag, "task_arn:") && strings.Contains(tag, taskID) {
			taskOK = true
		}
	}
	return nameOK && taskOK
}

func containerIDKnown(containerID string, ids map[string]struct{}) bool {
	if containerID == "" {
		return false
	}
	for id := range ids {
		if sameContainerID(containerID, id) {
			return true
		}
	}
	return false
}

func sameContainerID(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	// Fargate IDs are "<32 hex>-<decimal>". Containers in one task share the
	// hex prefix, so a prefix comparison matches the wrong container.
	if strings.Contains(a, "-") || strings.Contains(b, "-") {
		return false
	}
	shorter, longer := a, b
	if len(a) > len(b) {
		shorter, longer = b, a
	}
	return len(shorter) >= 12 && strings.HasPrefix(longer, shorter)
}

// supervisorProcs returns the comm lines between the script's markers.
// Interactive exec output uses CR LF, so each line is trimmed.
func supervisorProcs(out string) ([]string, error) {
	var procs []string
	started := false
	ended := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			continue
		}
		if !started {
			if line == "SUPERVISOR_PROCS" {
				started = true
			}
			continue
		}
		if line == "SUPERVISOR_END" {
			ended = true
			break
		}
		procs = append(procs, line)
	}
	if !started || !ended {
		return nil, errors.New("exec output missing supervisor markers")
	}
	return procs, nil
}

func processCommandName(comm, exe string, args []string) string {
	if comm != "" {
		return comm
	}
	if exe != "" {
		return pathBase(exe)
	}
	if len(args) > 0 {
		return pathBase(args[0])
	}
	return ""
}

func pathBase(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

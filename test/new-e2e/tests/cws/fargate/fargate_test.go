// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fargate contains e2e tests for fargate
package fargate

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ecs"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ssm"
	"github.com/pulumi/pulumi-awsx/sdk/go/awsx/awsx"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	awsecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	ecsx "github.com/pulumi/pulumi-awsx/sdk/go/awsx/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	sdkconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	configCommon "github.com/DataDog/test-infra-definitions/common/config"
	agentComp "github.com/DataDog/test-infra-definitions/components/datadog/agent"
	awsResources "github.com/DataDog/test-infra-definitions/resources/aws"
	ecsResources "github.com/DataDog/test-infra-definitions/resources/aws/ecs"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
	e2elib "github.com/DataDog/datadog-agent/test/new-e2e/tests/cws/lib"
)

const (
	// Pulumi exports keys
	ecsClusterNameKey = "ecs-cluster-name"
	ecsClusterArnKey  = "ecs-cluster-arn"
	fgTaskDefArnKey   = "fargate-task-arn"
	// Tests constants
	ddHostnamePrefix    = "cws-tests-ecs-fg-task"
	selfTestsPolicyName = "selftests.policy"
	execRuleID          = "selftest_exec"
	openRuleID          = "selftest_open"
	execFilePath        = "/usr/bin/date"
	openFilePath        = "/tmp/open.test"
	// CWSInstrumentationConfig
	DDCWSInstrumentationNamespace              = "cwsinstrumentation"
	DDCWSInstrumentationFullImagePathParamName = "fullImagePath"
)

type ddCWSInstrumentationConfig struct {
	CWSInstrumentationConfig *sdkconfig.Config
	*configCommon.CommonEnvironment
}

func newCWSInstrumentationConfig(e *configCommon.CommonEnvironment) ddCWSInstrumentationConfig {
	return ddCWSInstrumentationConfig{
		sdkconfig.New(e.Ctx, DDCWSInstrumentationNamespace),
		e,
	}
}

func (c *ddCWSInstrumentationConfig) getFullImagePath() string {
	return c.GetStringWithDefault(c.CWSInstrumentationConfig, DDCWSInstrumentationFullImagePathParamName, "docker.io/datadog/cws-instrumentation-dev:safchain-custom-cws-inst")
}

type ECSFargateSuite struct {
	suite.Suite
	ctx       context.Context
	stackName string
	testID    string

	apiClient      *e2elib.APIClient
	ddHostname     string
	ecsClusterArn  string
	ecsClusterName string
	fgTaskDefArn   string
}

func TestECSFargate(t *testing.T) {
	suite.Run(t, &ECSFargateSuite{
		ctx:       context.Background(),
		stackName: "cws-tests-ecs-fg",
		testID:    e2elib.RandomString(4),
	})
}

func (s *ECSFargateSuite) SetupSuite() {
	s.apiClient = e2elib.NewAPIClient()

	ruleDefs := []*e2elib.TestRuleDefinition{
		{
			ID:         execRuleID,
			Expression: fmt.Sprintf(`exec.file.path == \"%s\"`, execFilePath),
		},
		{
			ID:         openRuleID,
			Expression: fmt.Sprintf(`open.file.path == \"%s\"`, openFilePath),
		},
	}
	selftestsPolicy, err := e2elib.GetPolicyContent(ruleDefs)
	s.Require().NoError(err)

	_, result, err := infra.GetStackManager().GetStack(s.ctx, s.stackName, nil, func(ctx *pulumi.Context) error {
		ddHostname := fmt.Sprintf("%s-%s", ddHostnamePrefix, s.testID)
		awsEnv, err := awsResources.NewEnvironment(ctx)
		if err != nil {
			return err
		}
		cwsCfg := newCWSInstrumentationConfig(awsEnv.CommonEnvironment)

		// Create cluster
		ecsCluster, err := ecsResources.CreateEcsCluster(awsEnv, "cws-cluster")
		if err != nil {
			return err
		}

		// Export cluster’s properties
		ctx.Export(ecsClusterNameKey, ecsCluster.Name)
		ctx.Export(ecsClusterArnKey, ecsCluster.Arn)

		// Associate Fargate capacity provider to the cluster
		_, err = ecsResources.NewClusterCapacityProvider(awsEnv, "cws-cluster-capacity-provider", ecsCluster.Name, pulumi.StringArray{pulumi.String("FARGATE")})
		if err != nil {
			return err
		}

		// Setup agent API key
		apiKeyParam, err := ssm.NewParameter(ctx, awsEnv.Namer.ResourceName("agent-apikey"), &ssm.ParameterArgs{
			Name:  awsEnv.CommonNamer.DisplayName(1011, pulumi.String("agent-apikey")),
			Type:  ssm.ParameterTypeSecureString,
			Value: awsEnv.AgentAPIKey(),
		}, awsEnv.WithProviders(configCommon.ProviderAWS, configCommon.ProviderAWSX))
		if err != nil {
			return err
		}

		// Create task definition
		taskDef, err := ecsx.NewFargateTaskDefinition(ctx, "cws-task", &ecsx.FargateTaskDefinitionArgs{
			Containers: map[string]ecsx.TaskDefinitionContainerDefinitionArgs{
				"datadog-agent": {
					Cpu:   pulumi.IntPtr(0),
					Name:  pulumi.String("datadog-agent"),
					Image: pulumi.String(agentComp.DockerAgentFullImagePath(awsEnv.CommonEnvironment, "docker.io/datadog/agent-dev", "safchain-custom-cws-inst-py3")),
					Command: pulumi.ToStringArray([]string{
						"sh",
						"-c",
						fmt.Sprintf("echo \"%s\" > /etc/datadog-agent/runtime-security.d/%s ; /bin/entrypoint.sh", selftestsPolicy, selfTestsPolicyName),
					}),
					Essential: pulumi.BoolPtr(true),
					Environment: ecsx.TaskDefinitionKeyValuePairArray{
						ecsx.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("DD_HOSTNAME"),
							Value: pulumi.StringPtr(ddHostname),
						},
						ecsx.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("ECS_FARGATE"),
							Value: pulumi.StringPtr("true"),
						},
						ecsx.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("DD_RUNTIME_SECURITY_CONFIG_ENABLED"),
							Value: pulumi.StringPtr("true"),
						},
						ecsx.TaskDefinitionKeyValuePairArgs{
							Name:  pulumi.StringPtr("DD_RUNTIME_SECURITY_CONFIG_EBPFLESS_ENABLED"),
							Value: pulumi.StringPtr("true"),
						},
					},
					Secrets: ecsx.TaskDefinitionSecretArray{
						ecsx.TaskDefinitionSecretArgs{
							Name:      pulumi.String("DD_API_KEY"),
							ValueFrom: apiKeyParam.Name,
						},
					},
					HealthCheck: &ecsx.TaskDefinitionHealthCheckArgs{
						Retries:     pulumi.IntPtr(2),
						Command:     pulumi.ToStringArray([]string{"CMD-SHELL", "/probe.sh"}),
						StartPeriod: pulumi.IntPtr(60),
						Interval:    pulumi.IntPtr(30),
						Timeout:     pulumi.IntPtr(5),
					},
					LogConfiguration: ecsx.TaskDefinitionLogConfigurationArgs{
						LogDriver: pulumi.String("awsfirelens"),
						Options: pulumi.StringMap{
							"Name":           pulumi.String("datadog"),
							"Host":           pulumi.String("http-intake.logs.datadoghq.com"),
							"TLS":            pulumi.String("on"),
							"dd_service":     pulumi.String(ddHostnamePrefix),
							"dd_source":      pulumi.String("datadog-agent"),
							"dd_message_key": pulumi.String("log"),
							"provider":       pulumi.String("ecs"),
						},
						SecretOptions: ecsx.TaskDefinitionSecretArray{
							ecsx.TaskDefinitionSecretArgs{
								Name:      pulumi.String("apikey"),
								ValueFrom: apiKeyParam.Name,
							},
						},
					},
				},
				"ubuntu-with-tracer": {
					Cpu:       pulumi.IntPtr(0),
					Name:      pulumi.String("ubuntu-with-tracer"),
					Image:     pulumi.String("docker.io/ubuntu:22.04"),
					Essential: pulumi.BoolPtr(true),
					EntryPoint: pulumi.ToStringArray([]string{
						"/cws-instrumentation-volume/cws-instrumentation",
						"trace",
						"--verbose",
						"--",
						"/cws-instrumentation-volume/cws-instrumentation",
						"trace",
						"selftests",
						"--exec",
						fmt.Sprintf("--exec.path=%s", execFilePath),
						"--open",
						fmt.Sprintf("--open.path=%s", openFilePath),
					}),
					DependsOn: ecsx.TaskDefinitionContainerDependencyArray{
						ecsx.TaskDefinitionContainerDependencyArgs{
							Condition:     pulumi.String("HEALTHY"),
							ContainerName: pulumi.String("datadog-agent"),
						},
						ecsx.TaskDefinitionContainerDependencyArgs{
							Condition:     pulumi.String("SUCCESS"),
							ContainerName: pulumi.String("cws-instrumentation-init"),
						},
					},
					LinuxParameters: &ecsx.TaskDefinitionLinuxParametersArgs{
						Capabilities: &ecsx.TaskDefinitionKernelCapabilitiesArgs{
							Add: pulumi.StringArray{
								pulumi.String("SYS_PTRACE"),
							},
						},
					},
					MountPoints: ecsx.TaskDefinitionMountPointArray{
						ecsx.TaskDefinitionMountPointArgs{
							SourceVolume:  pulumi.String("cws-instrumentation-volume"),
							ContainerPath: pulumi.String("/cws-instrumentation-volume"),
							ReadOnly:      pulumi.Bool(true),
						},
					},
					LogConfiguration: ecsx.TaskDefinitionLogConfigurationArgs{
						LogDriver: pulumi.String("awsfirelens"),
						Options: pulumi.StringMap{
							"Name":           pulumi.String("datadog"),
							"Host":           pulumi.String("http-intake.logs.datadoghq.com"),
							"TLS":            pulumi.String("on"),
							"dd_service":     pulumi.String(ddHostnamePrefix),
							"dd_source":      pulumi.String("ubuntu-with-tracer"),
							"dd_message_key": pulumi.String("log"),
							"provider":       pulumi.String("ecs"),
						},
						SecretOptions: ecsx.TaskDefinitionSecretArray{
							ecsx.TaskDefinitionSecretArgs{
								Name:      pulumi.String("apikey"),
								ValueFrom: apiKeyParam.Name,
							},
						},
					},
				},
				"log_router": {
					Cpu:       pulumi.IntPtr(0),
					User:      pulumi.StringPtr("0"),
					Name:      pulumi.String("log_router"),
					Image:     pulumi.String("amazon/aws-for-fluent-bit:latest"),
					Essential: pulumi.BoolPtr(true),
					FirelensConfiguration: ecsx.TaskDefinitionFirelensConfigurationArgs{
						Type: pulumi.String("fluentbit"),
						Options: pulumi.StringMap{
							"enable-ecs-log-metadata": pulumi.String("true"),
						},
					},
				},
				"cws-instrumentation-init": {
					Cpu:       pulumi.IntPtr(0),
					Name:      pulumi.String("cws-instrumentation-init"),
					Image:     pulumi.String(cwsCfg.getFullImagePath()),
					Essential: pulumi.BoolPtr(false),
					Command: pulumi.ToStringArray([]string{
						"/cws-instrumentation",
						"setup",
						"--cws-volume-mount",
						"/cws-instrumentation-volume",
					}),
					MountPoints: ecsx.TaskDefinitionMountPointArray{
						ecsx.TaskDefinitionMountPointArgs{
							SourceVolume:  pulumi.String("cws-instrumentation-volume"),
							ContainerPath: pulumi.String("/cws-instrumentation-volume"),
							ReadOnly:      pulumi.Bool(false),
						},
					},
				},
			},
			Cpu:    pulumi.StringPtr("2048"),
			Memory: pulumi.StringPtr("4096"),
			Volumes: ecs.TaskDefinitionVolumeArray{
				ecs.TaskDefinitionVolumeArgs{
					Name: pulumi.String("cws-instrumentation-volume"),
				},
			},
			ExecutionRole: &awsx.DefaultRoleWithPolicyArgs{
				RoleArn: pulumi.StringPtr(awsEnv.ECSTaskExecutionRole()),
			},
			TaskRole: &awsx.DefaultRoleWithPolicyArgs{
				RoleArn: pulumi.StringPtr(awsEnv.ECSTaskRole()),
			},
			Family: awsEnv.CommonNamer.DisplayName(255, pulumi.String("cws-task")),
		}, awsEnv.WithProviders(configCommon.ProviderAWS, configCommon.ProviderAWSX))
		if err != nil {
			return err
		}

		_, err = ecsResources.FargateService(awsEnv, "cws-service", ecsCluster.Arn, taskDef.TaskDefinition.Arn())
		if err != nil {
			return err
		}

		// Export task definition's properties
		ctx.Export(fgTaskDefArnKey, taskDef.TaskDefinition.Arn())

		s.ddHostname = ddHostname
		return nil
	}, false)
	s.Require().NoError(err)

	s.ecsClusterArn = result.Outputs[ecsClusterArnKey].Value.(string)
	s.ecsClusterName = result.Outputs[ecsClusterNameKey].Value.(string)
	s.fgTaskDefArn = result.Outputs[fgTaskDefArnKey].Value.(string)
}

func (s *ECSFargateSuite) TearDownSuite() {
	err := infra.GetStackManager().DeleteStack(s.ctx, s.stackName, nil)
	s.Assert().NoError(err)
}

func (s *ECSFargateSuite) Test00ECSFargateReady() {
	cfg, err := awsconfig.LoadDefaultConfig(s.ctx)
	s.Require().NoErrorf(err, "Failed to load AWS config")

	client := awsecs.NewFromConfig(cfg)

	s.Run("cluster-ready", func() {
		ready := s.EventuallyWithTf(func(collect *assert.CollectT) {
			var listServicesToken string
			listServicesMaxResults := int32(100)
			for nextToken := &listServicesToken; nextToken != nil; {
				clustersList, err := client.ListClusters(s.ctx, &awsecs.ListClustersInput{
					MaxResults: &listServicesMaxResults,
					NextToken:  nextToken,
				})
				if !assert.NoErrorf(collect, err, "Failed to list ECS clusters") {
					return
				}
				nextToken = clustersList.NextToken
				for _, clusterArn := range clustersList.ClusterArns {
					if clusterArn != s.ecsClusterArn {
						continue
					}
					clusters, err := client.DescribeClusters(s.ctx, &awsecs.DescribeClustersInput{
						Clusters: []string{clusterArn},
					})
					if !assert.NoErrorf(collect, err, "Failed to describe ECS cluster %s", clusterArn) {
						return
					}
					if !assert.Len(collect, clusters.Clusters, 1) {
						return
					}
					if !assert.NotNil(collect, clusters.Clusters[0].Status) {
						return
					}
					_ = assert.Equal(collect, "ACTIVE", *(clusters.Clusters[0].Status))
					return
				}
			}
			assert.Fail(collect, "Failed to find cluster")
		}, 5*time.Minute, 20*time.Second, "Failed to wait for ecs cluster to become ready (name:%s arn:%s)", s.ecsClusterName, s.ecsClusterArn)
		s.Require().True(ready, "Cluster isn't ready, stopping tests here")
	})

	s.Run("tasks-ready", func() {
		ready := s.EventuallyWithTf(func(collect *assert.CollectT) {
			taskReady := false
			var listServicesToken string
			listServicesMaxResults := int32(10)
			for nextServicesToken := &listServicesToken; nextServicesToken != nil; {
				servicesList, err := client.ListServices(s.ctx, &awsecs.ListServicesInput{
					Cluster:    &s.ecsClusterArn,
					MaxResults: &listServicesMaxResults,
					NextToken:  nextServicesToken,
				})
				if !assert.NoErrorf(collect, err, "Failed to list ECS services of cluster %s", s.ecsClusterArn) {
					return
				}
				nextServicesToken = servicesList.NextToken
				serviceDescriptions, err := client.DescribeServices(s.ctx, &awsecs.DescribeServicesInput{
					Cluster:  &s.ecsClusterName,
					Services: servicesList.ServiceArns,
				})
				if !assert.NoErrorf(collect, err, "Failed to describe ECS services %v", servicesList.ServiceArns) {
					return
				}
				for _, service := range serviceDescriptions.Services {
					var listTasksToken string
					listTasksMaxResults := int32(100)
					for nextTasksToken := &listTasksToken; nextTasksToken != nil; {
						tasksList, err := client.ListTasks(s.ctx, &awsecs.ListTasksInput{
							Cluster:       &s.ecsClusterArn,
							ServiceName:   service.ServiceName,
							DesiredStatus: awsecstypes.DesiredStatusRunning,
							MaxResults:    &listTasksMaxResults,
							NextToken:     nextTasksToken,
						})
						if !assert.NoErrorf(collect, err, "Failed to list ECS tasks of cluster %s and service %s", s.ecsClusterArn, *service.ServiceName) {
							return
						}
						nextTasksToken = tasksList.NextToken

						tasks, err := client.DescribeTasks(s.ctx, &awsecs.DescribeTasksInput{
							Cluster: &s.ecsClusterArn,
							Tasks:   tasksList.TaskArns,
						})
						if !assert.NoErrorf(collect, err, "Failed to describe ECS tasks %v", tasksList.TaskArns) {
							return
						}
						for _, task := range tasks.Tasks {
							running := assert.Equal(collect, string(awsecstypes.DesiredStatusRunning), *task.LastStatus)
							notUnhealthy := assert.NotEqual(collect, awsecstypes.HealthStatusUnhealthy, task.HealthStatus,
								"Task %s of service %s is unhealthy", *task.TaskArn, *service.ServiceName)
							if task.TaskDefinitionArn != nil && *task.TaskDefinitionArn == s.fgTaskDefArn {
								taskReady = running && notUnhealthy
							}
						}
					}
				}
			}
			assert.True(collect, taskReady, "Failed to validate the state of task %s", s.fgTaskDefArn)
		}, 5*time.Minute, 10*time.Second, "Failed to wait for fargate tasks to become ready")
		s.Require().True(ready, "Tasks aren't ready, stopping tests here")
	})
}

func (s *ECSFargateSuite) Test01RulesetLoaded() {
	query := fmt.Sprintf("host:%s rule_id:ruleset_loaded @policies.name:%s", s.ddHostname, selfTestsPolicyName)
	result, err := e2elib.WaitAppLogs(s.apiClient, query)
	s.Require().NoError(err, "could not get new ruleset_loaded event log")
	agentContext, ok := result.Attributes["agent"].(map[string]interface{})
	s.Require().True(ok, "unexpected agent context")
	s.Require().EqualValues("ruleset_loaded", agentContext["rule_id"], "unexpected agent rule ID")
}

func (s *ECSFargateSuite) Test02ExecRule() {
	query := fmt.Sprintf("host:%s rule_id:%s", s.ddHostname, execRuleID)
	result, err := e2elib.WaitAppLogs(s.apiClient, query)
	s.Require().NoError(err, "could not get the exec rule event log")
	agentContext, ok := result.Attributes["agent"].(map[string]interface{})
	s.Require().True(ok, "unexpected agent context")
	s.Require().EqualValues(execRuleID, agentContext["rule_id"], "unexpected agent rule ID")
}

func (s *ECSFargateSuite) Test03OpenRule() {
	query := fmt.Sprintf("host:%s rule_id:%s", s.ddHostname, openRuleID)
	result, err := e2elib.WaitAppLogs(s.apiClient, query)
	s.Require().NoError(err, "could not get the open rule event log")
	agentContext, ok := result.Attributes["agent"].(map[string]interface{})
	s.Require().True(ok, "unexpected agent context")
	s.Require().EqualValues(openRuleID, agentContext["rule_id"], "unexpected agent rule ID")
}

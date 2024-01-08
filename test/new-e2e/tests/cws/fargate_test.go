// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"text/template"

	"github.com/google/uuid"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ecs"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ssm"
	"github.com/pulumi/pulumi-awsx/sdk/go/awsx/awsx"

	ecsx "github.com/pulumi/pulumi-awsx/sdk/go/awsx/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/suite"

	configCommon "github.com/DataDog/test-infra-definitions/common/config"
	agentComp "github.com/DataDog/test-infra-definitions/components/datadog/agent"
	awsResources "github.com/DataDog/test-infra-definitions/resources/aws"
	ecsResources "github.com/DataDog/test-infra-definitions/resources/aws/ecs"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/cws/api"
)

const (
	ddHostnamePrefix    = "cws-tests-ecs-fg-task"
	selfTestsPolicyName = "selftests.policy"
	execRuleID          = "selftest_exec"
	openRuleID          = "selftest_open"
	execFilePath        = "/usr/bin/date"
	openFilePath        = "/tmp/open.test"
)

type ECSFargateSuite struct {
	suite.Suite
	ctx       context.Context
	stackName string
	testID    string

	apiClient  *api.Client
	ddHostname string
}

func TestECSFargate(t *testing.T) {
	suite.Run(t, &ECSFargateSuite{
		ctx:       context.Background(),
		stackName: "cws-tests-ecs-fg",
		testID:    uuid.NewString()[:4],
	})
}

func (s *ECSFargateSuite) SetupSuite() {
	s.apiClient = api.NewClient()

	ruleDefs := []*testRuleDefinition{
		{
			ID:         execRuleID,
			Expression: fmt.Sprintf(`exec.file.path == \"%s\"`, execFilePath),
		},
		{
			ID:         openRuleID,
			Expression: fmt.Sprintf(`open.file.path == \"%s\"`, openFilePath),
		},
	}
	selftestsPolicy, err := getPolicyContent(ruleDefs)
	s.Require().NoError(err)

	_, _, err = infra.GetStackManager().GetStack(s.ctx, s.stackName, nil, func(ctx *pulumi.Context) error {
		ddHostname := fmt.Sprintf("%s-%s", ddHostnamePrefix, s.testID)
		awsEnv, err := awsResources.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		// Create cluster
		ecsCluster, err := ecsResources.CreateEcsCluster(awsEnv, "cws-cluster")
		if err != nil {
			return err
		}

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
				"log_router": *ecsResources.FargateFirelensContainerDefinition(),
				"cws-instrumentation-init": {
					Cpu:       pulumi.IntPtr(0),
					Name:      pulumi.String("cws-instrumentation-init"),
					Image:     pulumi.String(getCWSInstrumentationFullImagePath(awsEnv.CommonEnvironment)),
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

		s.ddHostname = ddHostname
		return nil
	}, false)
	s.Require().NoError(err)
}

func (s *ECSFargateSuite) TearDownSuite() {
	err := infra.GetStackManager().DeleteStack(s.ctx, s.stackName, nil)
	s.Assert().NoError(err)
}

func (s *ECSFargateSuite) TestRulesetLoaded() {
	query := fmt.Sprintf("host:%s rule_id:ruleset_loaded @policies.name:%s", s.ddHostname, selfTestsPolicyName)
	result, err := api.WaitAppLogs(s.apiClient, query)
	s.Require().NoError(err, "could not get new ruleset_loaded event log")
	agentContext, ok := result.Attributes["agent"].(map[string]interface{})
	s.Require().True(ok, "unexpected agent context")
	s.Require().EqualValues("ruleset_loaded", agentContext["rule_id"], "unexpected agent rule ID")
}

func (s *ECSFargateSuite) TestExecRule() {
	query := fmt.Sprintf("host:%s rule_id:%s", s.ddHostname, execRuleID)
	result, err := api.WaitAppLogs(s.apiClient, query)
	s.Require().NoError(err, "could not get the exec rule event log")
	agentContext, ok := result.Attributes["agent"].(map[string]interface{})
	s.Require().True(ok, "unexpected agent context")
	s.Require().EqualValues(execRuleID, agentContext["rule_id"], "unexpected agent rule ID")
}

func (s *ECSFargateSuite) TestOpenRule() {
	query := fmt.Sprintf("host:%s rule_id:%s", s.ddHostname, openRuleID)
	result, err := api.WaitAppLogs(s.apiClient, query)
	s.Require().NoError(err, "could not get the open rule event log")
	agentContext, ok := result.Attributes["agent"].(map[string]interface{})
	s.Require().True(ok, "unexpected agent context")
	s.Require().EqualValues(openRuleID, agentContext["rule_id"], "unexpected agent rule ID")
}

const (
	cwsInstrumentationFullImagePathParamName = "cwsinstrumentation:fullImagePath"
	cwsInstrumentationDefaultImagePath       = "docker.io/datadog/cws-instrumentation-dev:safchain-custom-cws-inst"
)

func getCWSInstrumentationFullImagePath(e *configCommon.CommonEnvironment) string {
	if fullImagePath, ok := e.Ctx.GetConfig(cwsInstrumentationFullImagePathParamName); ok {
		return fullImagePath
	}
	return cwsInstrumentationDefaultImagePath
}

// testRuleDefinition defines a rule used in a test policy
type testRuleDefinition struct {
	ID         string
	Version    string
	Expression string
}

const testPolicyTemplate = `---
version: 1.2.3

rules:
{{range $Rule := .Rules}}
  - id: {{$Rule.ID}}
    version: {{$Rule.Version}}
    expression: >-
      {{$Rule.Expression}}
{{end}}
`

// getPolicyContent returns the policy content from the given test rule definitions
func getPolicyContent(rules []*testRuleDefinition) (string, error) {
	tmpl, err := template.New("policy").Parse(testPolicyTemplate)
	if err != nil {
		return "", err
	}

	buffer := new(bytes.Buffer)
	if err := tmpl.Execute(buffer, map[string]interface{}{
		"Rules": rules,
	}); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

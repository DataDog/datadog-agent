// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ecs"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ssm"
	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/awsx"
	ecsx "github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configCommon "github.com/DataDog/test-infra-definitions/common/config"
	awsResources "github.com/DataDog/test-infra-definitions/resources/aws"
	ecsResources "github.com/DataDog/test-infra-definitions/resources/aws/ecs"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/cws/api"
)

const (
	ecsFgHostnamePrefix = "cws-e2e-ecs-fg-task"
	selfTestsPolicyName = "selftests.policy"
	execRuleID          = "selftest_exec"
	openRuleID          = "selftest_open"
	execFilePath        = "/usr/bin/date"
	openFilePath        = "/tmp/open.test"
)

// this env struct is empty for now but will eventually contain a component for an ECS test environment
type ecsFargateEnv struct{}

type ecsFargateSuite struct {
	e2e.BaseSuite[ecsFargateEnv]
	apiClient  *api.Client
	ddHostname string
}

func TestECSFargate(t *testing.T) {
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
	policy, err := getPolicyContent(ruleDefs)
	require.NoErrorf(t, err, "failed generate policy from test rules: %v", err)

	ddHostname := fmt.Sprintf("%s-%s", ecsFgHostnamePrefix, uuid.NewString()[:4])

	e2e.Run[ecsFargateEnv](t, &ecsFargateSuite{ddHostname: ddHostname},
		e2e.WithUntypedPulumiProvisioner(func(ctx *pulumi.Context) error {
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
						Image: pulumi.String(getAgentFullImagePath(awsEnv.CommonEnvironment)),
						Command: pulumi.ToStringArray([]string{
							"sh",
							"-c",
							fmt.Sprintf("echo \"%s\" > /etc/datadog-agent/runtime-security.d/%s ; /bin/entrypoint.sh", policy, selfTestsPolicyName),
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
						LogConfiguration: ecsResources.GetFirelensLogConfiguration(pulumi.String("datadog-agent"), pulumi.String(ecsFgHostnamePrefix), apiKeyParam.Name),
					},
					"ubuntu-with-tracer": {
						Cpu:       pulumi.IntPtr(0),
						Name:      pulumi.String("ubuntu-with-tracer"),
						Image:     pulumi.String("public.ecr.aws/lts/ubuntu:22.04"),
						Essential: pulumi.BoolPtr(false),
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
						LogConfiguration: ecsResources.GetFirelensLogConfiguration(pulumi.String("ubuntu-with-tracer"), pulumi.String(ecsFgHostnamePrefix), apiKeyParam.Name),
					},
					"cws-instrumentation-init": {
						Cpu:       pulumi.IntPtr(0),
						Name:      pulumi.String("cws-instrumentation-init"),
						Image:     pulumi.String(getCWSInstrumentationFullImagePath()),
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
						LogConfiguration: ecsResources.GetFirelensLogConfiguration(pulumi.String("cws-instrumentation-init"), pulumi.String(ecsFgHostnamePrefix), apiKeyParam.Name),
						User:             pulumi.StringPtr("0"),
					},
					"log_router": *ecsResources.FargateFirelensContainerDefinition(),
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

			_, err = ecsResources.FargateService(awsEnv, "cws-service", ecsCluster.Arn, taskDef.TaskDefinition.Arn(), nil)
			if err != nil {
				return err
			}
			return nil
		}, nil),
	)
}

func (s *ecsFargateSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	s.apiClient = api.NewClient()
}

func (s *ecsFargateSuite) Hostname() string {
	return s.ddHostname
}

func (s *ecsFargateSuite) Client() *api.Client {
	return s.apiClient
}

func (s *ecsFargateSuite) TestRulesetLoaded() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		testRulesetLoaded(c, s, "file", selfTestsPolicyName)
	}, 1*time.Minute, 5*time.Second)
}

func (s *ecsFargateSuite) TestExecRule() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		testRuleEvent(c, s, execRuleID)
	}, 1*time.Minute, 5*time.Second)
}

func (s *ecsFargateSuite) TestOpenRule() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		testRuleEvent(c, s, openRuleID)
	}, 1*time.Minute, 5*time.Second)
}

const (
	cwsInstrumentationFullImagePathParamName = "cwsinstrumentation:fullImagePath"
	cwsInstrumentationDefaultImagePath       = "public.ecr.aws/datadog/cws-instrumentation:rc"
	agentDefaultImagePath                    = "public.ecr.aws/datadog/agent:7.51.0-rc.1"
)

func getCWSInstrumentationFullImagePath() string {
	if fullImagePath := os.Getenv("CWS_INSTRUMENTATION_FULLIMAGEPATH"); fullImagePath != "" {
		return fullImagePath
	}
	return cwsInstrumentationDefaultImagePath
}

func getAgentFullImagePath(e *configCommon.CommonEnvironment) string {
	if fullImagePath := e.AgentFullImagePath(); fullImagePath != "" {
		return fullImagePath
	}

	if e.PipelineID() != "" && e.CommitSHA() != "" {
		return fmt.Sprintf("669783387624.dkr.ecr.us-east-1.amazonaws.com/agent:%s-%s", e.PipelineID(), e.CommitSHA())
	}

	return agentDefaultImagePath
}

// testRuleDefinition defines a rule used in a test policy
type testRuleDefinition struct {
	ID         string
	Expression string
}

const testPolicyTemplate = `---
version: 1.2.3

rules:
{{range $Rule := .Rules}}
  - id: {{$Rule.ID}}
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

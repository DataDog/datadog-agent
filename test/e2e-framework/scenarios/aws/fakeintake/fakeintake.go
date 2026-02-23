// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fakeintake

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ecs"

	classicECS "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ecs"
	clb "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/lb"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ssm"
	awsxEcs "github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/cenkalti/backoff/v5"
)

const (
	sleepInterval = 1 * time.Second
	maxRetries    = 120
	containerName = "fakeintake"
	httpPort      = 80
	httpsPort     = 443
)

func NewECSFargateInstance(e aws.Environment, name string, option ...Option) (*fakeintake.Fakeintake, error) {
	params, paramsErr := NewParams(option...)
	if paramsErr != nil {
		return nil, paramsErr
	}

	return components.NewComponent(&e, e.Namer.ResourceName(name), func(fi *fakeintake.Fakeintake) error {
		namer := e.Namer.WithPrefix("fakeintake").WithPrefix(name)
		opts := []pulumi.ResourceOption{pulumi.Parent(fi)}

		apiKeyParam, err := ssm.NewParameter(e.Ctx(), namer.ResourceName("agent", "apikey"), &ssm.ParameterArgs{
			Name:      e.CommonNamer().DisplayName(1011, pulumi.String(name), pulumi.String("apikey")),
			Type:      ssm.ParameterTypeSecureString,
			Value:     e.AgentAPIKey(),
			Overwrite: pulumi.Bool(true),
		}, utils.MergeOptions(opts, e.WithProviders(config.ProviderAWS))...)
		if err != nil {
			return err
		}

		taskDef, err := ecs.FargateTaskDefinitionWithAgent(e,
			namer.ResourceName("taskdef"),
			pulumi.String("fakeintake-ecs"),
			params.CPU, params.Memory,
			map[string]awsxEcs.TaskDefinitionContainerDefinitionArgs{"fakeintake": *fargateLinuxContainerDefinition(apiKeyParam.Name, params)},
			apiKeyParam.Name,
			nil,
			"public.ecr.aws/datadog/agent:latest",
			opts...,
		)
		if err != nil {
			return err
		}
		e.Ctx().Log.Info(fmt.Sprintf("Fakeintake dashboard available at: https://dddev.datadoghq.com/dashboard/xzy-ybs-wz4/e2e-tests--fake-intake?fromUser=true&tpl_var_fake_intake_task_family[0]=%s-fakeintake-ecs", e.Ctx().Stack()), nil)
		useLoadBalancer := false
		if params.LoadBalancerEnabled {
			if len(e.DefaultFakeintakeLBs()) != 0 {
				useLoadBalancer = true
			} else {
				e.Ctx().Log.Warn("Load balancer is enabled but no listener is defined, will not use LB", nil)
			}
		}

		if useLoadBalancer {
			err = fargateSvcLB(e, namer, taskDef, fi, opts...)
		} else {
			err = fargateSvcNoLB(e, namer, taskDef, fi, opts...)
		}
		if err != nil {
			return err
		}

		return nil
	})
}

// fargateSvcNoLB deploys one fakeintake container to a dedicated Fargate cluster
// Hardcoded on sandbox
func fargateSvcNoLB(e aws.Environment, namer namer.Namer, taskDef *awsxEcs.FargateTaskDefinition, fi *fakeintake.Fakeintake, opts ...pulumi.ResourceOption) error {
	fargateService, err := ecs.FargateService(e, namer.ResourceName("srv"), e.ECSFargateFakeintakeClusterArn(), taskDef.TaskDefinition.Arn(), nil, opts...)
	if err != nil {
		return err
	}

	// Hack passing taskDef.TaskDefinition.Arn() to execute apply function
	// when taskDef has an ARN, thus it is defined on AWS side
	output := pulumi.All(taskDef.TaskDefinition.Arn(), fargateService.Service.Name(), e.ECSFargateFakeintakeClusterArn()).ApplyT(func(args []any) ([]string, error) {
		serviceName := args[1].(string)
		fakeintakeECSArn := args[2].(string)
		ctx := context.Background()
		ipAddress, err := backoff.Retry(ctx, func() (string, error) {
			e.Ctx().Log.Debug("waiting for fakeintake task private ip", nil)
			ecsClient, err := ecs.NewECSClient(e.Ctx().Context(), e)
			if err != nil {
				return "", err
			}
			ip, err := ecsClient.GetTaskPrivateIP(fakeintakeECSArn, serviceName)
			if err != nil {
				return "", err
			}
			e.Ctx().Log.Info(fmt.Sprintf("fakeintake task private ip found: %s\n", ip), nil)
			return ip, nil
		}, backoff.WithBackOff(backoff.NewConstantBackOff(sleepInterval)), backoff.WithMaxTries(maxRetries))
		if err != nil {
			e.Ctx().Log.Warn(fmt.Sprintf("error while waiting for fakeintake task private ip: %v", err), nil)
			return nil, err
		}

		// fail the deployment if the fakeintake is not healthy
		e.Ctx().Log.Info(fmt.Sprintf("waiting for fakeintake at %s to be healthy", ipAddress), nil)
		healthURL := buildFakeIntakeURL("http", ipAddress, "/fakeintake/health", httpPort)
		_, err = backoff.Retry(ctx, func() (any, error) {
			e.Ctx().Log.Debug(fmt.Sprintf("getting fakeintake health at %s", healthURL), nil)
			resp, err := http.Get(healthURL)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("error getting fakeintake health: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
			}
			return nil, nil
		}, backoff.WithBackOff(backoff.NewConstantBackOff(sleepInterval)), backoff.WithMaxTries(maxRetries))
		if err != nil {
			e.Ctx().Log.Warn(fmt.Sprintf("error while waiting for fakeintake at %s: %v", ipAddress, err), nil)
			return nil, err
		}
		e.Ctx().Log.Info(fmt.Sprintf("fakeintake healthy at %s", ipAddress), nil)

		return []string{ipAddress, buildFakeIntakeURL("http", ipAddress, "", httpPort)}, nil
	}).(pulumi.StringArrayOutput)

	fi.Scheme = pulumi.Sprintf("%s", "http")
	fi.Port = pulumi.Int(httpPort).ToIntOutput()
	fi.Host = output.Index(pulumi.Int(0))
	fi.URL = output.Index(pulumi.Int(1))

	return err
}

func fargateSvcLB(e aws.Environment, namer namer.Namer, taskDef *awsxEcs.FargateTaskDefinition, fi *fakeintake.Fakeintake, opts ...pulumi.ResourceOption) error {
	targetGroup, err := clb.NewTargetGroup(e.Ctx(), namer.ResourceName("target-group"), &clb.TargetGroupArgs{
		Port:          pulumi.Int(80),
		Protocol:      pulumi.String("HTTP"),
		TargetType:    pulumi.String("ip"),
		IpAddressType: pulumi.String("ipv4"),
		VpcId:         pulumi.StringPtr(e.DefaultVPCID()),
		Name:          e.CommonNamer().DisplayName(32, pulumi.String("fakeintake")),
	}, utils.MergeOptions(opts, e.WithProviders(config.ProviderAWS))...)
	if err != nil {
		return err
	}

	// Hashing fakeintake resource name as prefix for Host header
	hostPrefix := utils.StrHash(namer.ResourceName(e.Ctx().Stack()))
	host := pulumi.Sprintf("%s%s", hostPrefix, e.ECSFakeintakeLBBaseHost())

	_, err = clb.NewListenerRule(e.Ctx(), namer.ResourceName(hostPrefix), &clb.ListenerRuleArgs{
		ListenerArn: e.ECSFakeintakeLBListenerArn(),
		Conditions: clb.ListenerRuleConditionArray{
			clb.ListenerRuleConditionArgs{
				HostHeader: clb.ListenerRuleConditionHostHeaderArgs{
					Values: host.ApplyT(func(host string) []string {
						return []string{host}
					}).(pulumi.StringArrayOutput),
				},
			},
		},
		Actions: clb.ListenerRuleActionArray{
			clb.ListenerRuleActionArgs{
				Type:           pulumi.String("forward"),
				TargetGroupArn: targetGroup.Arn,
			},
		},
	}, utils.MergeOptions(opts, e.WithProviders(config.ProviderAWS))...)
	if err != nil {
		return err
	}

	balancerArray := classicECS.ServiceLoadBalancerArray{
		&classicECS.ServiceLoadBalancerArgs{
			ContainerName:  pulumi.String(containerName),
			ContainerPort:  pulumi.Int(httpPort),
			TargetGroupArn: targetGroup.Arn,
		},
	}

	_, err = ecs.FargateService(e, namer.ResourceName("srv"), e.ECSFargateFakeintakeClusterArn(), taskDef.TaskDefinition.Arn(), balancerArray, opts...)
	if err != nil {
		return err
	}

	fi.Scheme = pulumi.Sprintf("%s", "https")
	fi.Port = pulumi.Int(httpsPort).ToIntOutput()
	fi.Host = host
	fi.URL = pulumi.Sprintf("%s://%s", fi.Scheme, host)
	return nil
}

func fargateLinuxContainerDefinition(apiKeySSMParamName pulumi.StringInput, params *Params) *awsxEcs.TaskDefinitionContainerDefinitionArgs {
	command := []string{}
	if params.DDDevForwarding {
		command = append(command, "--dddev-forward")
	}

	if params.RetentionPeriod != "" {
		command = append(command, "-retention-period="+params.RetentionPeriod)
	}

	return &awsxEcs.TaskDefinitionContainerDefinitionArgs{
		Name:        pulumi.String(containerName),
		Image:       pulumi.String(params.ImageURL),
		Essential:   pulumi.BoolPtr(true),
		Command:     pulumi.ToStringArray(command),
		MountPoints: awsxEcs.TaskDefinitionMountPointArray{},
		Environment: awsxEcs.TaskDefinitionKeyValuePairArray{
			awsxEcs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("GOMEMLIMIT"),
				Value: pulumi.StringPtr(fmt.Sprintf("%dMiB", params.Memory)),
			},
			awsxEcs.TaskDefinitionKeyValuePairArgs{
				Name:  pulumi.StringPtr("STORAGE_DRIVER"),
				Value: pulumi.StringPtr("memory"),
			},
		},
		Secrets: awsxEcs.TaskDefinitionSecretArray{
			awsxEcs.TaskDefinitionSecretArgs{
				Name:      pulumi.String("DD_API_KEY"),
				ValueFrom: apiKeySSMParamName,
			},
		},
		PortMappings: awsxEcs.TaskDefinitionPortMappingArray{
			awsxEcs.TaskDefinitionPortMappingArgs{
				ContainerPort: pulumi.Int(httpPort),
				HostPort:      pulumi.Int(httpPort),
				Protocol:      pulumi.StringPtr("tcp"),
			},
		},
		HealthCheck: &awsxEcs.TaskDefinitionHealthCheckArgs{
			Command: pulumi.ToStringArray([]string{"CMD-SHELL", "curl --fail http://localhost/fakeintake/health"}),
			// Explicitly set the following 3 parameters to their default values.
			// Because otherwise, `pulumi up` wants to recreate the task definition even when it is not needed.
			Interval: pulumi.IntPtr(30),
			Retries:  pulumi.IntPtr(3),
			Timeout:  pulumi.IntPtr(5),
		},
		DockerLabels: pulumi.StringMap{
			"com.datadoghq.ad.checks": pulumi.String(utils.JSONMustMarshal(
				map[string]interface{}{
					"openmetrics": map[string]interface{}{
						"init_config": map[string]interface{}{},
						"instances": []map[string]interface{}{
							{
								"openmetrics_endpoint": "http://%%host%%/metrics",
								"namespace":            "fakeintake",
								"metrics": []string{
									".*",
								},
							},
						},
					},
					"http_check": map[string]interface{}{
						"init_config": map[string]interface{}{},
						"instances": []map[string]interface{}{
							{
								"name": "health",
								"url":  "http://%%host%%/fakeintake/health",
							},
							{
								"name": "metrics query",
								"url":  "http://%%host%%/fakeintake/payloads?endpoint=/api/v2/series",
							},
							{
								"name": "logs query",
								"url":  "http://%%host%%/fakeintake/payloads?endpoint=/api/v2/logs",
							},
						},
					},
				}),
			),
		},
		LogConfiguration: ecs.GetFirelensLogConfiguration(pulumi.String("fakeintake"), pulumi.String("fakeintake"), apiKeySSMParamName),
	}
}

func buildFakeIntakeURL(scheme, host, path string, port int) string {
	url := &url.URL{
		Scheme: scheme,
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		Path:   path,
	}
	return url.String()
}

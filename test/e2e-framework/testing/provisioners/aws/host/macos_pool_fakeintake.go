// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package awshost

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsECS "github.com/aws/aws-sdk-go-v2/service/ecs"
	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	awsresources "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ecs"
	fakeintakescenario "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
)

// fakeIntakeContainerName is the ECS container name for the single fakeintake
// container. Unlike the Pulumi ECS path, this Pulumi-free path does not bundle a
// self-monitoring Agent sidecar or Firelens log router: E2E tests only exercise
// fakeintake's own HTTP API, so that machinery would be provisioned for no benefit.
const fakeIntakeContainerName = "fakeintake"

const (
	fakeIntakeHTTPPort           = 80
	fakeIntakeTaskCPU            = "512"
	fakeIntakeTaskMemory         = "1024"
	fakeIntakeServiceWaitTimeout = 5 * time.Minute
	fakeIntakeHealthWaitTimeout  = 5 * time.Minute
	fakeIntakePollInterval       = 5 * time.Second
)

// macOSPoolFakeIntake tracks the raw ECS resources created for a single macOS pool
// run's FakeIntake, so Destroy can tear them down without a live *pulumi.Context.
type macOSPoolFakeIntake struct {
	region      string
	profile     string
	clusterArn  string
	serviceName string
	taskDefArn  string
}

// provisionMacOSPoolFakeIntake registers an ECS Fargate task definition and service
// running a single fakeintake container, waits for it to become healthy, and returns
// its output alongside the tracking state Destroy needs.
func provisionMacOSPoolFakeIntake(ctx context.Context, logger io.Writer, region, profile, envName string, fiOpts []fakeintakescenario.Option) (*macOSPoolFakeIntake, *fakeintake.FakeintakeOutput, error) {
	params, err := fakeintakescenario.NewParams(fiOpts...)
	if err != nil {
		return nil, nil, err
	}

	clusterArn, taskExecutionRole, _, vpcID, subnets, securityGroups, allocatePublicIP := awsresources.ECSFakeintakeNetworkDefaults(envName)
	if clusterArn == "" || vpcID == "" || len(subnets) == 0 {
		return nil, nil, fmt.Errorf("no ECS/network defaults found for environment %q", envName)
	}

	client, err := ecs.NewRawClient(ctx, region, profile)
	if err != nil {
		return nil, nil, err
	}

	family := "fakeintake-ecs-pool-" + macOSPoolOwnerID()
	registerOut, err := client.RegisterTaskDefinition(ctx, &awsECS.RegisterTaskDefinitionInput{
		Family:                  &family,
		NetworkMode:             ecsTypes.NetworkModeAwsvpc,
		RequiresCompatibilities: []ecsTypes.Compatibility{ecsTypes.CompatibilityFargate},
		Cpu:                     awssdk.String(fakeIntakeTaskCPU),
		Memory:                  awssdk.String(fakeIntakeTaskMemory),
		ExecutionRoleArn:        awssdk.String(taskExecutionRole),
		ContainerDefinitions:    []ecsTypes.ContainerDefinition{fakeIntakeContainerDefinition(params)},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to register fakeintake task definition: %w", err)
	}
	taskDefArn := *registerOut.TaskDefinition.TaskDefinitionArn

	fi := &macOSPoolFakeIntake{
		region:     region,
		profile:    profile,
		clusterArn: clusterArn,
		taskDefArn: taskDefArn,
	}

	assignPublicIP := ecsTypes.AssignPublicIpDisabled
	if allocatePublicIP {
		assignPublicIP = ecsTypes.AssignPublicIpEnabled
	}

	serviceName := family
	_, err = client.CreateService(ctx, &awsECS.CreateServiceInput{
		Cluster:        &clusterArn,
		ServiceName:    &serviceName,
		TaskDefinition: &taskDefArn,
		DesiredCount:   awssdk.Int32(1),
		LaunchType:     ecsTypes.LaunchTypeFargate,
		NetworkConfiguration: &ecsTypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecsTypes.AwsVpcConfiguration{
				Subnets:        subnets,
				SecurityGroups: securityGroups,
				AssignPublicIp: assignPublicIP,
			},
		},
	})
	if err != nil {
		return fi, nil, fmt.Errorf("failed to create fakeintake ECS service: %w", err)
	}
	fi.serviceName = serviceName

	fmt.Fprintf(logger, "waiting for fakeintake ECS task private ip on cluster %s, service %s\n", clusterArn, serviceName)
	ip, err := waitForFakeIntakeTaskIP(client, clusterArn, serviceName, fakeIntakeServiceWaitTimeout)
	if err != nil {
		return fi, nil, fmt.Errorf("failed to obtain fakeintake task private ip: %w", err)
	}

	url := fmt.Sprintf("http://%s:%d", ip, fakeIntakeHTTPPort)
	fmt.Fprintf(logger, "waiting for fakeintake at %s to be healthy\n", url)
	if err := waitForFakeIntakeHealth(url, fakeIntakeHealthWaitTimeout); err != nil {
		return fi, nil, fmt.Errorf("fakeintake at %s never became healthy: %w", url, err)
	}
	fmt.Fprintf(logger, "fakeintake healthy at %s\n", url)

	return fi, &fakeintake.FakeintakeOutput{
		Host:   ip,
		Scheme: "http",
		Port:   fakeIntakeHTTPPort,
		URL:    url,
	}, nil
}

// destroy deletes the FakeIntake ECS service and deregisters its task definition. It
// is a no-op if provisionMacOSPoolFakeIntake never successfully created a service.
func (fi *macOSPoolFakeIntake) destroy(ctx context.Context) error {
	if fi == nil || fi.serviceName == "" {
		return nil
	}

	client, err := ecs.NewRawClient(ctx, fi.region, fi.profile)
	if err != nil {
		return err
	}

	force := true
	if _, err := client.DeleteService(ctx, &awsECS.DeleteServiceInput{
		Cluster: &fi.clusterArn,
		Service: &fi.serviceName,
		Force:   &force,
	}); err != nil {
		return fmt.Errorf("failed to delete fakeintake ECS service %s: %w", fi.serviceName, err)
	}

	if fi.taskDefArn != "" {
		if _, err := client.DeregisterTaskDefinition(ctx, &awsECS.DeregisterTaskDefinitionInput{
			TaskDefinition: &fi.taskDefArn,
		}); err != nil {
			return fmt.Errorf("failed to deregister fakeintake task definition %s: %w", fi.taskDefArn, err)
		}
	}

	return nil
}

func fakeIntakeContainerDefinition(params *fakeintakescenario.Params) ecsTypes.ContainerDefinition {
	command := []string{}
	if params.DDDevForwarding {
		command = append(command, "--dddev-forward")
	}
	if params.RetentionPeriod != "" {
		command = append(command, "-retention-period="+params.RetentionPeriod)
	}
	command = append(command, "--rc-key-data="+fakeintake.DefaultRCSigningKeySeed)

	return ecsTypes.ContainerDefinition{
		Name:      awssdk.String(fakeIntakeContainerName),
		Image:     awssdk.String(params.ImageURL),
		Essential: awssdk.Bool(true),
		Command:   command,
		Environment: []ecsTypes.KeyValuePair{
			{Name: awssdk.String("GOMEMLIMIT"), Value: awssdk.String(fmt.Sprintf("%dMiB", params.Memory))},
			{Name: awssdk.String("STORAGE_DRIVER"), Value: awssdk.String("memory")},
		},
		PortMappings: []ecsTypes.PortMapping{
			{ContainerPort: awssdk.Int32(fakeIntakeHTTPPort), HostPort: awssdk.Int32(fakeIntakeHTTPPort), Protocol: ecsTypes.TransportProtocolTcp},
		},
		HealthCheck: &ecsTypes.HealthCheck{
			Command:  []string{"CMD-SHELL", "curl --fail http://localhost/fakeintake/health"},
			Interval: awssdk.Int32(30),
			Retries:  awssdk.Int32(3),
			Timeout:  awssdk.Int32(5),
		},
	}
}

// waitForFakeIntakeTaskIP polls the ECS service until a running task with a private
// IP appears, or timeout elapses.
func waitForFakeIntakeTaskIP(client *ecs.Client, clusterArn, serviceName string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ip, err := client.GetTaskPrivateIP(clusterArn, serviceName)
		if err == nil {
			return ip, nil
		}
		lastErr = err
		time.Sleep(fakeIntakePollInterval)
	}
	return "", fmt.Errorf("timed out waiting for fakeintake task private ip: %w", lastErr)
}

// waitForFakeIntakeHealth polls fakeintake's health endpoint until it returns 200, or
// timeout elapses.
func waitForFakeIntakeHealth(url string, timeout time.Duration) error {
	healthURL := url + "/fakeintake/health"
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := http.Get(healthURL) //nolint:gosec // healthURL is built from our own ECS task's private IP, not user input.
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("unexpected status %d from %s", resp.StatusCode, healthURL)
		} else {
			lastErr = err
		}
		time.Sleep(fakeIntakePollInterval)
	}
	return lastErr
}

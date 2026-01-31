// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecs

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// WaitForContainerInstances waits for at least minInstances container instances to be registered
// in the ECS cluster before returning. This ensures services can place tasks.
func WaitForContainerInstances(e aws.Environment, clusterArn pulumi.StringOutput, minInstances int) pulumi.StringOutput {
	// Use pulumi.All to wait for the cluster ARN to be resolved
	return pulumi.All(clusterArn).ApplyT(func(args []interface{}) (string, error) {
		clusterArnStr := args[0].(string)

		// Load AWS SDK config
		ctx := context.Background()
		cfg, err := awsconfig.LoadDefaultConfig(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to load AWS config: %w", err)
		}

		ecsClient := ecs.NewFromConfig(cfg)

		// Wait for container instances with exponential backoff
		maxWaitTime := 5 * time.Minute
		pollInterval := 10 * time.Second
		startTime := time.Now()

		e.Ctx().Log.Info(fmt.Sprintf("Waiting for at least %d container instance(s) to register in cluster %s", minInstances, clusterArnStr), nil)

		for {
			// Check if we've exceeded max wait time
			if time.Since(startTime) > maxWaitTime {
				return "", fmt.Errorf("timeout waiting for container instances after %v", maxWaitTime)
			}

			// List container instances
			listOutput, err := ecsClient.ListContainerInstances(ctx, &ecs.ListContainerInstancesInput{
				Cluster: awssdk.String(clusterArnStr),
				Status:  "ACTIVE",
			})
			if err != nil {
				e.Ctx().Log.Warn(fmt.Sprintf("Failed to list container instances: %v, retrying...", err), nil)
				time.Sleep(pollInterval)
				continue
			}

			registeredCount := len(listOutput.ContainerInstanceArns)
			e.Ctx().Log.Info(fmt.Sprintf("Found %d registered container instance(s) (need %d)", registeredCount, minInstances), nil)

			// Check if we have enough instances
			if registeredCount >= minInstances {
				e.Ctx().Log.Info(fmt.Sprintf("Container instances ready! Found %d instance(s)", registeredCount), nil)
				return "ready", nil
			}

			// Wait before next poll
			e.Ctx().Log.Info(fmt.Sprintf("Waiting %v before checking again...", pollInterval), nil)
			time.Sleep(pollInterval)
		}
	}).(pulumi.StringOutput)
}

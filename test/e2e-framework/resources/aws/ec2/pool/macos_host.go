// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pool

import (
	"context"
	"fmt"
	"os"
	"time"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	awsec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// LaunchConfig holds the AWS region/profile for the macOS pool provisioner, sourced
// from env vars since there is no Pulumi-free way to read stack config outside a live
// *pulumi.Context.
type LaunchConfig struct {
	Region  string
	Profile string
}

// LoadLaunchConfigFromEnv reads LaunchConfig from E2E_MACOS_POOL_* env vars, defaulting
// the region to us-east-1 if unset.
func LoadLaunchConfigFromEnv() (*LaunchConfig, error) {
	return &LaunchConfig{
		Region:  getEnvDefault("E2E_MACOS_POOL_REGION", "us-east-1"),
		Profile: os.Getenv("E2E_MACOS_POOL_PROFILE"),
	}, nil
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// rootVolumeReplaceWaitTimeout bounds how long ResetRootVolume waits for a
// create-replace-root-volume-task to reach a terminal state.
const rootVolumeReplaceWaitTimeout = 10 * time.Minute

// ResetRootVolume resets instanceID's root volume to its launch-time state and reboots
// the instance.
func ResetRootVolume(ctx context.Context, client *awsec2.Client, instanceID string) error {
	createOut, err := client.CreateReplaceRootVolumeTask(ctx, &awsec2.CreateReplaceRootVolumeTaskInput{
		InstanceId: &instanceID,
	})
	if err != nil {
		return fmt.Errorf("failed to create replace-root-volume task for instance %s: %w", instanceID, err)
	}
	taskID := *createOut.ReplaceRootVolumeTask.ReplaceRootVolumeTaskId

	deadline := time.Now().Add(rootVolumeReplaceWaitTimeout)
	for {
		describeOut, err := client.DescribeReplaceRootVolumeTasks(ctx, &awsec2.DescribeReplaceRootVolumeTasksInput{
			ReplaceRootVolumeTaskIds: []string{taskID},
		})
		if err != nil {
			return fmt.Errorf("failed to describe replace-root-volume task %s: %w", taskID, err)
		}
		if len(describeOut.ReplaceRootVolumeTasks) > 0 {
			switch describeOut.ReplaceRootVolumeTasks[0].TaskState {
			case awsec2types.ReplaceRootVolumeTaskStateSucceeded:
				return nil
			case awsec2types.ReplaceRootVolumeTaskStateFailed, awsec2types.ReplaceRootVolumeTaskStateFailedDetached:
				return fmt.Errorf("replace-root-volume task %s for instance %s ended in state %s", taskID, instanceID, describeOut.ReplaceRootVolumeTasks[0].TaskState)
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("replace-root-volume task %s for instance %s did not complete within %s", taskID, instanceID, rootVolumeReplaceWaitTimeout)
		}
		select {
		case <-time.After(10 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// DescribeInstance returns instanceID's current private IP, used to import an
// existing pool member acquired via Acquire.
func DescribeInstance(ctx context.Context, client *awsec2.Client, instanceID string) (privateIP string, err error) {
	out, err := client.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{InstanceIds: []string{instanceID}})
	if err != nil {
		return "", fmt.Errorf("failed to describe instance %s: %w", instanceID, err)
	}
	for _, reservation := range out.Reservations {
		for _, instance := range reservation.Instances {
			if instance.PrivateIpAddress == nil {
				return "", fmt.Errorf("instance %s has no private IP yet", instanceID)
			}
			return *instance.PrivateIpAddress, nil
		}
	}
	return "", fmt.Errorf("instance %s not found", instanceID)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pool

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	awsec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// hostAvailableWaitTimeout bounds how long AllocateDedicatedHost waits for a freshly
// allocated Dedicated Host to reach the "available" state.
const hostAvailableWaitTimeout = 10 * time.Minute

// LaunchConfig holds every value NewInstance's Pulumi path would otherwise read from
// Pulumi stack config (Pulumi.<stack>.yaml), which has no reader outside a live
// *pulumi.Context. There is no Pulumi-free equivalent of that config system in this
// codebase, so the macOS pool provisioner sources these from env vars instead, falling
// back to the values resources/aws/environmentDefaults.go's sandboxDefault() hardcodes
// for the sandbox account (the account macOS E2E tests run against today).
type LaunchConfig struct {
	Region             string
	Profile            string
	SubnetID           string
	SecurityGroupIDs   []string
	KeyPairName        string
	InstanceProfile    string
	ShutdownBehavior   string
	StorageSize        int32
	HTTPTokensRequired bool
}

// LoadLaunchConfigFromEnv reads LaunchConfig from E2E_MACOS_POOL_* env vars, falling
// back to sandboxDefault()'s values for anything not credential-like. KeyPairName has
// no safe fallback (same as aws.Environment.DefaultKeyPairName()'s Require semantics)
// and errors if unset.
func LoadLaunchConfigFromEnv() (*LaunchConfig, error) {
	keyPairName := os.Getenv("E2E_MACOS_POOL_KEY_PAIR")
	if keyPairName == "" {
		return nil, fmt.Errorf("E2E_MACOS_POOL_KEY_PAIR is required (no default key pair for the macOS pool)")
	}

	storageSize := int32(200)
	if v := os.Getenv("E2E_MACOS_POOL_STORAGE_SIZE"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid E2E_MACOS_POOL_STORAGE_SIZE %q: %w", v, err)
		}
		storageSize = int32(parsed)
	}

	securityGroups := []string{"sg-46506837", "sg-7fedd80a", "sg-0e952e295ab41e748"}
	if v := os.Getenv("E2E_MACOS_POOL_SECURITY_GROUPS"); v != "" {
		securityGroups = strings.Split(v, ",")
	}

	return &LaunchConfig{
		Region:             getEnvDefault("E2E_MACOS_POOL_REGION", "us-east-1"),
		Profile:            os.Getenv("E2E_MACOS_POOL_PROFILE"),
		SubnetID:           getEnvDefault("E2E_MACOS_POOL_SUBNET_ID", "subnet-b89e00e2"),
		SecurityGroupIDs:   securityGroups,
		KeyPairName:        keyPairName,
		InstanceProfile:    getEnvDefault("E2E_MACOS_POOL_INSTANCE_PROFILE", "ec2InstanceRole"),
		ShutdownBehavior:   getEnvDefault("E2E_MACOS_POOL_SHUTDOWN_BEHAVIOR", "stop"),
		StorageSize:        storageSize,
		HTTPTokensRequired: false,
	}, nil
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// subnetAvailabilityZone returns subnetID's availability zone, needed to allocate a
// Dedicated Host in the same AZ as the instance that will run on it.
func subnetAvailabilityZone(ctx context.Context, client *awsec2.Client, subnetID string) (string, error) {
	out, err := client.DescribeSubnets(ctx, &awsec2.DescribeSubnetsInput{SubnetIds: []string{subnetID}})
	if err != nil {
		return "", fmt.Errorf("failed to describe subnet %s: %w", subnetID, err)
	}
	if len(out.Subnets) == 0 {
		return "", fmt.Errorf("subnet %s not found", subnetID)
	}
	return *out.Subnets[0].AvailabilityZone, nil
}

// AllocateDedicatedHost allocates a new EC2 Dedicated Host for instanceType in the
// same availability zone as cfg.SubnetID, waiting for it to become available. This is
// the raw-SDK equivalent of resources/aws/ec2/dedicated_host.go's NewDedicatedHost.
func AllocateDedicatedHost(ctx context.Context, client *awsec2.Client, cfg *LaunchConfig, instanceType string) (hostID string, err error) {
	az, err := subnetAvailabilityZone(ctx, client, cfg.SubnetID)
	if err != nil {
		return "", err
	}

	out, err := client.AllocateHosts(ctx, &awsec2.AllocateHostsInput{
		AvailabilityZone: &az,
		InstanceType:     &instanceType,
		Quantity:         pointer.Ptr(int32(1)),
		HostRecovery:     awsec2types.HostRecoveryOff,
		AutoPlacement:    awsec2types.AutoPlacementOff,
	})
	if err != nil {
		return "", fmt.Errorf("failed to allocate dedicated host for instance type %s: %w", instanceType, err)
	}
	if len(out.HostIds) == 0 {
		return "", fmt.Errorf("AllocateHosts returned no host id for instance type %s", instanceType)
	}
	hostID = out.HostIds[0]

	deadline := time.Now().Add(hostAvailableWaitTimeout)
	for {
		describeOut, err := client.DescribeHosts(ctx, &awsec2.DescribeHostsInput{HostIds: []string{hostID}})
		if err != nil {
			return "", fmt.Errorf("failed to describe dedicated host %s: %w", hostID, err)
		}
		if len(describeOut.Hosts) > 0 && describeOut.Hosts[0].State == awsec2types.AllocationStateAvailable {
			return hostID, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("dedicated host %s did not become available within %s", hostID, hostAvailableWaitTimeout)
		}
		select {
		case <-time.After(10 * time.Second):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

// LaunchInstance creates a new EC2 instance pinned to hostID (tenancy "host") and
// waits for it to reach the running state, returning its instance ID. This is the
// raw-SDK equivalent of resources/aws/ec2/vm.go's NewInstance, for the macOS
// dedicated-host case specifically (Tenancy is always "host").
func LaunchInstance(ctx context.Context, client *awsec2.Client, cfg *LaunchConfig, ami, instanceType, hostID string) (instanceID string, err error) {
	rootBlockDevice := awsec2types.BlockDeviceMapping{
		DeviceName: pointer.Ptr("/dev/sda1"),
		Ebs: &awsec2types.EbsBlockDevice{
			VolumeSize: &cfg.StorageSize,
		},
	}

	input := &awsec2.RunInstancesInput{
		ImageId:                           &ami,
		InstanceType:                      awsec2types.InstanceType(instanceType),
		MinCount:                          pointer.Ptr(int32(1)),
		MaxCount:                          pointer.Ptr(int32(1)),
		SubnetId:                          &cfg.SubnetID,
		SecurityGroupIds:                  cfg.SecurityGroupIDs,
		KeyName:                           &cfg.KeyPairName,
		IamInstanceProfile:                &awsec2types.IamInstanceProfileSpecification{Name: &cfg.InstanceProfile},
		Placement:                         &awsec2types.Placement{Tenancy: awsec2types.TenancyHost, HostId: &hostID},
		BlockDeviceMappings:               []awsec2types.BlockDeviceMapping{rootBlockDevice},
		InstanceInitiatedShutdownBehavior: awsec2types.ShutdownBehavior(cfg.ShutdownBehavior),
	}
	if cfg.HTTPTokensRequired {
		input.MetadataOptions = &awsec2types.InstanceMetadataOptionsRequest{HttpTokens: awsec2types.HttpTokensStateRequired}
	}

	out, err := client.RunInstances(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to run macOS pool instance (ami %s, host %s): %w", ami, hostID, err)
	}
	if len(out.Instances) == 0 {
		return "", fmt.Errorf("RunInstances returned no instance for ami %s", ami)
	}
	instanceID = *out.Instances[0].InstanceId

	waiter := awsec2.NewInstanceRunningWaiter(client)
	if err := waiter.Wait(ctx, &awsec2.DescribeInstancesInput{InstanceIds: []string{instanceID}}, hostAvailableWaitTimeout); err != nil {
		return "", fmt.Errorf("instance %s did not reach running state: %w", instanceID, err)
	}
	return instanceID, nil
}

// rootVolumeReplaceWaitTimeout bounds how long ResetRootVolume waits for a
// create-replace-root-volume-task to reach a terminal state.
const rootVolumeReplaceWaitTimeout = 10 * time.Minute

// ResetRootVolume resets instanceID's root volume to its exact state at launch time
// (no AMI/snapshot involved), rebooting the instance as part of the task. This is the
// raw-SDK equivalent of resources/aws/ec2/root_volume.go's ReplaceRootVolumeToLaunchState,
// used on release instead of a Pulumi local.Command since there is no Pulumi resource to
// attach a Delete handler to on this path.
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

// DescribeInstance returns instanceID's current private IP and state name, used both
// to import an existing pool member (Found=true) and to read back a freshly launched
// one after LaunchInstance's waiter confirms it's running.
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

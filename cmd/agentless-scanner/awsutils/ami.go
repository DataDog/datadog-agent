// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package awsutils

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/devices"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ebs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// SetupAMI sets up the AMI for scanning.
func SetupAMI(ctx context.Context, scan *types.ScanTask, waiter *SnapshotWaiter) ([]string, error) {
	ec2client := ec2.NewFromConfig(GetConfigFromCloudID(ctx, scan.Roles, scan.CloudID))

	snapshots, err := ec2client.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{
		SnapshotIds: []string{scan.CloudID.ResourceName()},
	})
	if err != nil {
		return nil, err
	}
	if len(snapshots.Snapshots) == 0 {
		return nil, fmt.Errorf("snapshot not found: %s", scan.CloudID)
	}

	snapshot := snapshots.Snapshots[0]
	log.Debugf("%s: copying snapshot %s", scan, scan.CloudID)
	copyCreatedAt := time.Now()
	copyOutput, err := ec2client.CopySnapshot(ctx, &ec2.CopySnapshotInput{
		SourceRegion:      aws.String(scan.CloudID.Region()),
		SourceSnapshotId:  aws.String(scan.CloudID.ResourceName()),
		TagSpecifications: cloudResourceTagSpec(scan.CloudID.ResourceType(), scan.ScannerHostname),
		Encrypted:         snapshot.Encrypted,
		KmsKeyId:          snapshot.KmsKeyId,
	})
	if err != nil {
		return nil, fmt.Errorf("copying snapshot %q: %w", scan.CloudID, err)
	}

	copiedSnapshotID, err := types.AWSCloudID("ec2", scan.CloudID.Region(), scan.CloudID.AccountID(), types.ResourceTypeSnapshot, *copyOutput.SnapshotId)
	if err != nil {
		return nil, err
	}
	scan.PushCreatedResource(copiedSnapshotID, copyCreatedAt)

	if err = <-waiter.Wait(ctx, copiedSnapshotID, ec2client); err != nil {
		return nil, fmt.Errorf("waiting for snapshot %q: %w", copiedSnapshotID, err)
	}

	switch scan.DiskMode {
	case types.DiskModeVolumeAttach:
		if err := AttachSnapshotWithVolume(ctx, scan, waiter, copiedSnapshotID); err != nil {
			return nil, err
		}
	case types.DiskModeNBDAttach:
		ebsclient := ebs.NewFromConfig(GetConfigFromCloudID(ctx, scan.Roles, copiedSnapshotID))
		if err := AttachSnapshotWithNBD(ctx, scan, copiedSnapshotID, ebsclient); err != nil {
			return nil, err
		}
	case types.DiskModeNoAttach:
		return nil, nil // nothing to do. early exit.
	default:
		panic("unreachable")
	}

	partitions, err := devices.ListPartitions(ctx, scan, *scan.AttachedDeviceName)
	if err != nil {
		return nil, err
	}

	return devices.Mount(ctx, scan, partitions)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package awsutils

import (
	"context"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/devices"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"
	"github.com/aws/aws-sdk-go-v2/service/ebs"
)

// SetupAMI sets up the AMI for scanning.
func SetupAMI(ctx context.Context, scan *types.ScanTask, waiter *SnapshotWaiter) ([]string, error) {
	snapshotID := scan.CloudID
	switch scan.DiskMode {
	case types.DiskModeVolumeAttach:
		if err := AttachSnapshotWithVolume(ctx, scan, waiter, snapshotID); err != nil {
			return nil, err
		}
	case types.DiskModeNBDAttach:
		ebsclient := ebs.NewFromConfig(GetConfigFromCloudID(ctx, scan, snapshotID))
		if err := AttachSnapshotWithNBD(ctx, scan, snapshotID, ebsclient); err != nil {
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

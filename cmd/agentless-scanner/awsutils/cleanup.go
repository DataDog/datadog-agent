// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package awsutils

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/devices"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// CleanSlate removes all volumes that are currently attached to the instance
// and have no mountpoints.
func CleanSlate(ctx context.Context, bds []devices.BlockDevice, roles types.RolesMapping) {
	var attachedVolumes []string
	for _, bd := range bds {
		if strings.HasPrefix(bd.Serial, "vol") && len(bd.Children) > 0 {
			isScan := false
			noMount := 0
			// TODO: we could maybe rely on the output of lsblk to do our cleanup instead
			for _, child := range bd.Children {
				if len(child.Mountpoints) == 0 {
					noMount++
				} else {
					for _, mountpoint := range child.Mountpoints {
						if strings.HasPrefix(mountpoint, types.ScansRootDir+"/") {
							isScan = true
						}
					}
				}
			}
			if isScan || len(bd.Children) == noMount {
				volumeID := "vol-" + strings.TrimPrefix(bd.Serial, "vol")
				attachedVolumes = append(attachedVolumes, volumeID)
			}
		}
	}

	if self, err := getSelfEC2InstanceIndentity(ctx); err == nil {
		for _, volumeID := range attachedVolumes {
			volumeID, err := types.AWSCloudID(self.Region, self.AccountID, types.ResourceTypeVolume, volumeID)
			if err != nil {
				log.Warnf("clean slate: %v", err)
				continue
			}
			if errd := CleanupScanVolume(ctx, nil, volumeID, roles); err != nil {
				log.Warnf("clean slate: %v", errd)
			}
		}
	}
}

// ListResourcesForCleanup lists all AWS resources that are created by our
// scanner.
func ListResourcesForCleanup(ctx context.Context, maxTTL time.Duration, region string, assumedRole types.CloudID) ([]types.CloudID, error) {
	cfg := GetConfig(ctx, region, assumedRole)
	ec2client := ec2.NewFromConfig(cfg)
	var toBeDeleted []types.CloudID
	var nextToken *string

	stsclient := sts.NewFromConfig(cfg)
	identity, err := stsclient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("could not get caller identity: %w", err)
	}

	for {
		volumes, err := ec2client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
			NextToken: nextToken,
			Filters:   cloudResourceTagFilters(),
		})
		if err != nil {
			log.Warnf("could not list volumes created by agentless-scanner: %v", err)
			break
		}
		for _, volume := range volumes.Volumes {
			if volume.State == ec2types.VolumeStateAvailable {
				volumeID, err := types.AWSCloudID(region, *identity.Account, types.ResourceTypeVolume, *volume.VolumeId)
				if err != nil {
					return nil, err
				}
				toBeDeleted = append(toBeDeleted, volumeID)
			}
		}
		nextToken = volumes.NextToken
		if nextToken == nil {
			break
		}
	}

	for {
		snapshots, err := ec2client.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{
			NextToken: nextToken,
			Filters:   cloudResourceTagFilters(),
		})
		if err != nil {
			log.Warnf("could not list snapshots created by agentless-scanner: %v", err)
			break
		}
		for _, snapshot := range snapshots.Snapshots {
			if snapshot.State != ec2types.SnapshotStateCompleted {
				continue
			}
			since := time.Now().Add(-maxTTL)
			if snapshot.StartTime != nil && snapshot.StartTime.After(since) {
				continue
			}
			snapshotID, err := types.AWSCloudID(region, *identity.Account, types.ResourceTypeSnapshot, *snapshot.SnapshotId)
			if err != nil {
				return nil, err
			}
			toBeDeleted = append(toBeDeleted, snapshotID)
		}
		nextToken = snapshots.NextToken
		if nextToken == nil {
			break
		}
	}
	return toBeDeleted, nil
}

// ResourcesCleanup removes all resources provided in the map.
func ResourcesCleanup(ctx context.Context, toBeDeleted []types.CloudID, region string, assumedRole types.CloudID) {
	ec2client := ec2.NewFromConfig(GetConfig(ctx, region, assumedRole))
	for _, resourceID := range toBeDeleted {
		if err := ctx.Err(); err != nil {
			return
		}
		log.Infof("cleaning up resource %q", resourceID)
		var err error
		switch resourceID.ResourceType() {
		case types.ResourceTypeSnapshot:
			_, err = ec2client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
				SnapshotId: aws.String(resourceID.ResourceName()),
			})
		case types.ResourceTypeVolume:
			_, err = ec2client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{
				VolumeId: aws.String(resourceID.ResourceName()),
			})
		}
		if err != nil {
			log.Errorf("could not delete resource %q: %s", resourceID, err)
		}
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package awsutils

import (
	"context"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/devices"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func cloudResourceTagSpec(resourceType types.ResourceType, scannerHostname string) []ec2types.TagSpecification {
	return []ec2types.TagSpecification{
		{
			ResourceType: ec2types.ResourceType(resourceType),
			Tags: []ec2types.Tag{
				{Key: aws.String("DatadogAgentlessScanner"), Value: aws.String("true")},
				{Key: aws.String("DatadogAgentlessScannerHostOrigin"), Value: aws.String(scannerHostname)},
				// TODO: add origin account and instance ID
			},
		},
	}
}

func cloudResourceTagFilters() []ec2types.Filter {
	return []ec2types.Filter{
		{
			Name: aws.String("tag:DatadogAgentlessScanner"),
			Values: []string{
				"true",
			},
		},
	}
}

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

	if self, err := GetSelfEC2InstanceIndentity(ctx); err == nil {
		for _, volumeID := range attachedVolumes {
			volumeID, err := types.AWSCloudID("ec2", self.Region, self.AccountID, types.ResourceTypeVolume, volumeID)
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
func ListResourcesForCleanup(ctx context.Context, maxTTL time.Duration, region string, assumedRole *types.CloudID) map[types.ResourceType][]string {
	ec2client := ec2.NewFromConfig(GetConfig(ctx, region, assumedRole))
	toBeDeleted := make(map[types.ResourceType][]string)
	var nextToken *string

	for {
		volumes, err := ec2client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
			NextToken: nextToken,
			Filters:   cloudResourceTagFilters(),
		})
		if err != nil {
			log.Warnf("could not list volumes created by agentless-scanner: %v", err)
			break
		}
		for i := range volumes.Volumes {
			if volumes.Volumes[i].State == ec2types.VolumeStateAvailable {
				volumeID := *volumes.Volumes[i].VolumeId
				toBeDeleted[types.ResourceTypeVolume] = append(toBeDeleted[types.ResourceTypeVolume], volumeID)
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
		for i := range snapshots.Snapshots {
			if snapshots.Snapshots[i].State != ec2types.SnapshotStateCompleted {
				continue
			}
			since := time.Now().Add(-maxTTL)
			if snapshots.Snapshots[i].StartTime != nil && snapshots.Snapshots[i].StartTime.After(since) {
				continue
			}
			snapshotID := *snapshots.Snapshots[i].SnapshotId
			toBeDeleted[types.ResourceTypeSnapshot] = append(toBeDeleted[types.ResourceTypeSnapshot], snapshotID)
		}
		nextToken = snapshots.NextToken
		if nextToken == nil {
			break
		}
	}
	return toBeDeleted
}

// ResourcesCleanup removes all resources provided in the map.
func ResourcesCleanup(ctx context.Context, toBeDeleted map[types.ResourceType][]string, region string, assumedRole *types.CloudID) {
	ec2client := ec2.NewFromConfig(GetConfig(ctx, region, assumedRole))
	for resourceType, resources := range toBeDeleted {
		for _, resourceName := range resources {
			if err := ctx.Err(); err != nil {
				return
			}
			log.Infof("cleaning up resource %s/%s", resourceType, resourceName)
			var err error
			switch resourceType {
			case types.ResourceTypeSnapshot:
				_, err = ec2client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
					SnapshotId: aws.String(resourceName),
				})
			case types.ResourceTypeVolume:
				_, err = ec2client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{
					VolumeId: aws.String(resourceName),
				})
			}
			if err != nil {
				log.Errorf("could not delete resource %s/%s: %s", resourceType, resourceName, err)
			}
		}
	}
}

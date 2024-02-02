// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package awsutils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/devices"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/nbd"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

const (
	cleanupMaxDuration = 2 * time.Minute
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
			volumeARN := EC2ARN(self.Region, self.AccountID, types.ResourceTypeVolume, volumeID)
			if errd := CleanupScanVolumes(ctx, nil, volumeARN, roles); err != nil {
				log.Warnf("clean slate: %v", errd)
			}
		}
	}
}

// CleanupScan removes all resources associated with a scan.
func CleanupScan(scan *types.ScanTask) {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupMaxDuration)
	defer cancel()

	scanRoot := scan.Path()

	log.Debugf("%s: cleaning up scan data on filesystem", scan)

	for snapshotARNString, snapshotCreatedAt := range scan.CreatedSnapshots {
		snapshotARN, err := types.ParseARN(snapshotARNString, types.ResourceTypeSnapshot)
		if err != nil {
			continue
		}
		_, snapshotID, _ := types.GetARNResource(snapshotARN)
		cfg, err := GetConfig(ctx, snapshotARN.Region, scan.Roles[snapshotARN.AccountID])
		if err != nil {
			log.Errorf("%s: %v", scan, err)
		} else {
			ec2client := ec2.NewFromConfig(cfg)
			log.Debugf("%s: deleting snapshot %q", scan, snapshotID)
			if _, err := ec2client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
				SnapshotId: aws.String(snapshotID),
			}); err != nil {
				log.Warnf("%s: could not delete snapshot %s: %v", scan, snapshotID, err)
			} else {
				log.Debugf("%s: snapshot deleted %s", scan, snapshotID)
				statsResourceTTL(types.ResourceTypeSnapshot, scan, *snapshotCreatedAt)
			}
		}
	}

	entries, err := os.ReadDir(scanRoot)
	if err == nil {
		var wg sync.WaitGroup

		umount := func(mountPoint string) {
			defer wg.Done()
			devices.Umount(ctx, scan, mountPoint)
		}

		var ebsMountPoints []fs.DirEntry
		var ctrMountPoints []fs.DirEntry
		var pidFiles []fs.DirEntry

		for _, entry := range entries {
			if entry.IsDir() {
				if strings.HasPrefix(entry.Name(), types.EBSMountPrefix) {
					ebsMountPoints = append(ebsMountPoints, entry)
				}
				if strings.HasPrefix(entry.Name(), types.ContainerdMountPrefix) || strings.HasPrefix(entry.Name(), types.DockerMountPrefix) {
					ctrMountPoints = append(ctrMountPoints, entry)
				}
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".pid") {
					pidFiles = append(pidFiles, entry)
				}
			}
		}
		for _, entry := range ctrMountPoints {
			wg.Add(1)
			go umount(filepath.Join(scanRoot, entry.Name()))
		}
		wg.Wait()
		// unmount "ebs-*" entrypoint last as the other mountpoint may depend on it
		for _, entry := range ebsMountPoints {
			wg.Add(1)
			go umount(filepath.Join(scanRoot, entry.Name()))
		}
		wg.Wait()

		for _, entry := range pidFiles {
			pidFile, err := os.Open(filepath.Join(scanRoot, entry.Name()))
			if err != nil {
				continue
			}
			pidRaw, err := io.ReadAll(io.LimitReader(pidFile, 32))
			if err != nil {
				pidFile.Close()
				continue
			}
			pidFile.Close()
			if pid, err := strconv.Atoi(strings.TrimSpace(string(pidRaw))); err == nil {
				if proc, err := os.FindProcess(pid); err == nil {
					log.Debugf("%s: killing remaining scanner process with pid %d", scan, pid)
					_ = proc.Kill()
				}
			}
		}
	}

	log.Debugf("%s: removing folder %q", scan, scanRoot)
	if err := os.RemoveAll(scanRoot); err != nil {
		log.Errorf("%s: could not cleanup mount root %q: %v", scan, scanRoot, err)
	}

	if scan.AttachedDeviceName != nil {
		blockDevices, err := devices.List(ctx, *scan.AttachedDeviceName)
		if err == nil && len(blockDevices) == 1 {
			for _, child := range blockDevices[0].GetChildrenType("lvm") {
				if err := exec.Command("dmsetup", "remove", child.Path).Run(); err != nil {
					log.Errorf("%s: could not remove logical device %q from block device %q: %v", scan, child.Path, child.Name, err)
				}
			}
		}
	}

	switch scan.DiskMode {
	case types.VolumeAttach:
		if volumeARN := scan.AttachedVolumeARN; volumeARN != nil {
			if errd := CleanupScanVolumes(ctx, scan, *volumeARN, scan.Roles); errd != nil {
				log.Warnf("%s: could not delete volume %q: %v", scan, volumeARN, errd)
			} else {
				statsResourceTTL(types.ResourceTypeVolume, scan, *scan.AttachedVolumeCreatedAt)
			}
		}
	case types.NBDAttach:
		if diskDeviceName := scan.AttachedDeviceName; diskDeviceName != nil {
			nbd.StopNBDBlockDevice(ctx, *diskDeviceName)
		}
	default:
		panic("unreachable")
	}
}

// CleanupScanVolumes removes all resources associated with a volume.
func CleanupScanVolumes(ctx context.Context, maybeScan *types.ScanTask, volumeARN arn.ARN, roles types.RolesMapping) error {
	_, volumeID, _ := types.GetARNResource(volumeARN)
	cfg, err := GetConfig(ctx, volumeARN.Region, roles[volumeARN.AccountID])
	if err != nil {
		return err
	}

	ec2client := ec2.NewFromConfig(cfg)

	volumeNotFound := false
	volumeDetached := false
	log.Debugf("%s: detaching volume %q", maybeScan, volumeID)
	for i := 0; i < 5; i++ {
		if _, err := ec2client.DetachVolume(ctx, &ec2.DetachVolumeInput{
			Force:    aws.Bool(true),
			VolumeId: aws.String(volumeID),
		}); err != nil {
			var aerr smithy.APIError
			// NOTE(jinroh): we're trying to detach a volume in an 'available'
			// state for instance. Just bail.
			if errors.As(err, &aerr) {
				if aerr.ErrorCode() == "IncorrectState" {
					volumeDetached = true
					break
				}
				if aerr.ErrorCode() == "InvalidVolume.NotFound" {
					volumeNotFound = true
					break
				}
			}
			log.Warnf("%s: could not detach volume %s: %v", maybeScan, volumeID, err)
		} else {
			volumeDetached = true
			break
		}
		if !sleepCtx(ctx, 10*time.Second) {
			return fmt.Errorf("could not detach volume: %w", ctx.Err())
		}
	}

	if volumeDetached && maybeScan != nil && maybeScan.AttachedDeviceName != nil {
		for i := 0; i < 30; i++ {
			if !sleepCtx(ctx, 1*time.Second) {
				return ctx.Err()
			}
			devices, err := devices.List(ctx, *maybeScan.AttachedDeviceName)
			if err != nil || len(devices) == 0 {
				break
			}
		}
	}

	var errd error
	for i := 0; i < 10; i++ {
		if volumeNotFound {
			break
		}
		_, errd = ec2client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{
			VolumeId: aws.String(volumeID),
		})
		if errd != nil {
			var aerr smithy.APIError
			if errors.As(err, &aerr) && aerr.ErrorCode() == "InvalidVolume.NotFound" {
				errd = nil
				break
			}
		} else {
			log.Debugf("%s: volume deleted %q", maybeScan, volumeID)
			break
		}
		if !sleepCtx(ctx, 10*time.Second) {
			errd = ctx.Err()
			break
		}
	}
	if errd != nil {
		return fmt.Errorf("could not delete volume %q: %w", volumeID, errd)
	}
	return nil
}

func statsResourceTTL(resourceType types.ResourceType, scan *types.ScanTask, createTime time.Time) {
	ttl := time.Since(createTime)
	tags := scan.Tags(fmt.Sprintf("aws_resource_type:%s", string(resourceType)))
	if err := statsd.Histogram("datadog.agentless_scanner.aws.resources_ttl", float64(ttl.Milliseconds()), tags, 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
}

// ListResourcesForCleanup lists all AWS resources that are created by our
// scanner.
func ListResourcesForCleanup(ctx context.Context, maxTTL time.Duration, region string, assumedRole *arn.ARN) map[types.ResourceType][]string {
	cfg, err := GetConfig(ctx, region, assumedRole)
	if err != nil {
		return nil
	}

	ec2client := ec2.NewFromConfig(cfg)
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
func ResourcesCleanup(ctx context.Context, toBeDeleted map[types.ResourceType][]string, region string, assumedRole *arn.ARN) {
	cfg, err := GetConfig(ctx, region, assumedRole)
	if err != nil {
		return
	}

	ec2client := ec2.NewFromConfig(cfg)
	for resourceType, resources := range toBeDeleted {
		for _, resourceID := range resources {
			if err := ctx.Err(); err != nil {
				return
			}
			log.Infof("cleaning up resource %s/%s", resourceType, resourceID)
			var err error
			switch resourceType {
			case types.ResourceTypeSnapshot:
				_, err = ec2client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
					SnapshotId: aws.String(resourceID),
				})
			case types.ResourceTypeVolume:
				_, err = ec2client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{
					VolumeId: aws.String(resourceID),
				})
			}
			if err != nil {
				log.Errorf("could not delete resource %s/%s: %s", resourceType, resourceID, err)
			}
		}
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package awsutils

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/devices"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/nbd"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ebs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

const (
	maxSnapshotRetries = 3
	maxAttachRetries   = 15
)

// SetupEBS prepares the EBS volume for scanning. It creates a snapshot of the
// volume, attaches it to the instance and returns the list of partitions that
// were mounted.
func SetupEBS(ctx context.Context, scan *types.ScanTask, waiter *SnapshotWaiter) ([]string, error) {
	if scan.TargetHostname == "" {
		return nil, fmt.Errorf("ebs-volume: missing hostname")
	}

	assumedRole := scan.Roles[scan.CloudID.AccountID]
	cfg, err := GetConfig(ctx, scan.CloudID.Region, assumedRole)
	if err != nil {
		return nil, err
	}

	ec2client := ec2.NewFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	var snapshotID types.CloudID
	switch scan.CloudID.ResourceType {
	case types.ResourceTypeVolume:
		snapshotID, err = CreateSnapshot(ctx, scan, waiter, ec2client, scan.CloudID)
		if err != nil {
			return nil, err
		}
	case types.ResourceTypeSnapshot:
		snapshotID = scan.CloudID
	default:
		return nil, fmt.Errorf("ebs-volume: bad arn %q", scan.CloudID)
	}

	log.Infof("%s: start EBS scanning", scan)

	switch scan.DiskMode {
	case types.VolumeAttach:
		if err := AttachSnapshotWithVolume(ctx, scan, waiter, snapshotID); err != nil {
			return nil, err
		}
	case types.NBDAttach:
		ebsclient := ebs.NewFromConfig(cfg)
		if err := AttachSnapshotWithNBD(ctx, scan, snapshotID, ebsclient); err != nil {
			return nil, err
		}
	default:
		panic("unreachable")
	}

	partitions, err := devices.ListPartitions(ctx, scan, *scan.AttachedDeviceName)
	if err != nil {
		return nil, err
	}

	return devices.Mount(ctx, scan, partitions)
}

// CreateSnapshot creates a snapshot of the given EBS volume and returns its Cloud Identifier.
func CreateSnapshot(ctx context.Context, scan *types.ScanTask, waiter *SnapshotWaiter, ec2client *ec2.Client, volumeID types.CloudID) (types.CloudID, error) {
	snapshotCreatedAt := time.Now()
	if err := statsd.Count("datadog.agentless_scanner.snapshots.started", 1.0, scan.Tags(), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	log.Debugf("%s: starting volume snapshotting %q", scan, volumeID)

	retries := 0
retry:
	if volumeID.ResourceType != types.ResourceTypeVolume {
		return types.CloudID{}, fmt.Errorf("bad resource ID %q: expecting a volume", volumeID)
	}
	createSnapshotOutput, err := ec2client.CreateSnapshot(ctx, &ec2.CreateSnapshotInput{
		VolumeId:          aws.String(volumeID.ResourceName),
		TagSpecifications: cloudResourceTagSpec(types.ResourceTypeSnapshot, scan.ScannerHostname),
	})
	if err != nil {
		var aerr smithy.APIError
		var isRateExceededError bool
		// TODO: if we reach this error, we maybe could reuse a pending or
		// very recent snapshot that was created by the scanner.
		if errors.As(err, &aerr) && aerr.ErrorCode() == "SnapshotCreationPerVolumeRateExceeded" {
			isRateExceededError = true
		}
		if retries <= maxSnapshotRetries {
			retries++
			if isRateExceededError {
				// https://docs.aws.amazon.com/AWSEC2/latest/APIReference/errors-overview.html
				// Wait at least 15 seconds between concurrent volume snapshots.
				d := 15 * time.Second
				log.Debugf("%s: snapshot creation rate exceeded for volume %q; retrying after %v (%d/%d)", scan, volumeID, d, retries, maxSnapshotRetries)
				if !sleepCtx(ctx, d) {
					return types.CloudID{}, ctx.Err()
				}
				goto retry
			}
		}
		if isRateExceededError {
			log.Debugf("%s: snapshot creation rate exceeded for volume %q; skipping)", scan, volumeID)
		}
	}
	if err != nil {
		var isVolumeNotFoundError bool
		var aerr smithy.APIError
		if errors.As(err, &aerr) && aerr.ErrorCode() == "InvalidVolume.NotFound" {
			isVolumeNotFoundError = true
		}
		var tags []string
		if isVolumeNotFoundError {
			tags = scan.TagsNotFound()
		} else {
			tags = scan.TagsFailure(err)
		}
		if err := statsd.Count("datadog.agentless_scanner.snapshots.finished", 1.0, tags, 1.0); err != nil {
			log.Warnf("failed to send metric: %v", err)
		}
		return types.CloudID{}, err
	}

	snapshotID, err := EC2CloudID(volumeID.Region, volumeID.AccountID, types.ResourceTypeSnapshot, *createSnapshotOutput.SnapshotId)
	if err != nil {
		return snapshotID, err
	}
	scan.CreatedSnapshots[snapshotID.String()] = &snapshotCreatedAt

	err = <-waiter.Wait(ctx, snapshotID, ec2client)
	if err == nil {
		snapshotDuration := time.Since(snapshotCreatedAt)
		log.Debugf("%s: volume snapshotting of %q finished successfully %q (took %s)", scan, volumeID, snapshotID, snapshotDuration)
		if err := statsd.Histogram("datadog.agentless_scanner.snapshots.duration", float64(snapshotDuration.Milliseconds()), scan.Tags(), 1.0); err != nil {
			log.Warnf("failed to send metric: %v", err)
		}
		if err := statsd.Histogram("datadog.agentless_scanner.snapshots.size", float64(*createSnapshotOutput.VolumeSize), scan.TagsFailure(err), 1.0); err != nil {
			log.Warnf("failed to send metric: %v", err)
		}
		if err := statsd.Count("datadog.agentless_scanner.snapshots.finished", 1.0, scan.TagsSuccess(), 1.0); err != nil {
			log.Warnf("failed to send metric: %v", err)
		}
	} else {
		if err := statsd.Count("datadog.agentless_scanner.snapshots.finished", 1.0, scan.TagsFailure(err), 1.0); err != nil {
			log.Warnf("failed to send metric: %v", err)
		}
	}
	return snapshotID, err
}

// AttachSnapshotWithNBD attaches the given snapshot to the instance using a
// Network Block Device (NBD).
func AttachSnapshotWithNBD(ctx context.Context, scan *types.ScanTask, snapshotID types.CloudID, ebsclient *ebs.Client) error {
	device, ok := devices.NextNBD()
	if !ok {
		return fmt.Errorf("could not find non busy NBD block device")
	}
	backend, err := nbd.NewEBSBackend(ebsclient, snapshotID)
	if err != nil {
		return err
	}
	if err := nbd.StartNBDBlockDevice(scan.ID, device, backend); err != nil {
		return err
	}
	_, err = devices.Poll(ctx, scan, device, nil)
	if err != nil {
		return err
	}
	scan.AttachedDeviceName = &device
	return nil
}

// AttachSnapshotWithVolume attaches the given snapshot to the instance as a
// new volume.
func AttachSnapshotWithVolume(ctx context.Context, scan *types.ScanTask, waiter *SnapshotWaiter, snapshotID types.CloudID) error {
	if snapshotID.ResourceType != types.ResourceTypeSnapshot {
		return fmt.Errorf("expected snapshot resource: %s", snapshotID.String())
	}
	self, err := GetSelfEC2InstanceIndentity(ctx)
	if err != nil {
		return fmt.Errorf("could not get EC2 instance identity: using attach volumes cannot work outside an EC2 instance: %w", err)
	}

	remoteAssumedRole := scan.Roles[snapshotID.AccountID]
	remoteAWSCfg, err := GetConfig(ctx, snapshotID.Region, remoteAssumedRole)
	if err != nil {
		return err
	}
	remoteEC2Client := ec2.NewFromConfig(remoteAWSCfg)

	var localSnapshotID types.CloudID
	if snapshotID.Region != self.Region {
		log.Debugf("%s: copying snapshot %q into %q", scan, snapshotID, self.Region)
		copySnapshotCreatedAt := time.Now()
		copySnapshot, err := remoteEC2Client.CopySnapshot(ctx, &ec2.CopySnapshotInput{
			SourceRegion: aws.String(snapshotID.Region),
			// DestinationRegion: aws.String(self.Region): automatically filled by SDK
			SourceSnapshotId:  aws.String(snapshotID.ResourceName),
			TagSpecifications: cloudResourceTagSpec(types.ResourceTypeSnapshot, scan.ScannerHostname),
		})
		if err != nil {
			return fmt.Errorf("could not copy snapshot %q to %q: %w", snapshotID, self.Region, err)
		}
		localSnapshotID, err = EC2CloudID(self.Region, snapshotID.AccountID, types.ResourceTypeSnapshot, *copySnapshot.SnapshotId)
		if err != nil {
			return err
		}
		log.Debugf("%s: waiting for copy of snapshot %q into %q as %q", scan, snapshotID, self.Region, *copySnapshot.SnapshotId)
		err = <-waiter.Wait(ctx, localSnapshotID, remoteEC2Client)
		if err != nil {
			return fmt.Errorf("could not finish copying %q to %q as %q: %w", snapshotID, self.Region, *copySnapshot.SnapshotId, err)
		}
		log.Debugf("%s: successfully copied snapshot %q into %q: %q", scan, snapshotID, self.Region, *copySnapshot.SnapshotId)
		scan.CreatedSnapshots[localSnapshotID.String()] = &copySnapshotCreatedAt
	} else {
		localSnapshotID = snapshotID
	}

	if localSnapshotID.AccountID != "" && localSnapshotID.AccountID != self.AccountID {
		_, err = remoteEC2Client.ModifySnapshotAttribute(ctx, &ec2.ModifySnapshotAttributeInput{
			SnapshotId:    aws.String(snapshotID.ResourceName),
			Attribute:     ec2types.SnapshotAttributeNameCreateVolumePermission,
			OperationType: ec2types.OperationTypeAdd,
			UserIds:       []string{self.AccountID},
		})
		if err != nil {
			return fmt.Errorf("could not modify snapshot attributes %q for sharing with account ID %q: %w", localSnapshotID, self.AccountID, err)
		}
	}

	localAssumedRole := scan.Roles[self.AccountID]
	localAWSCfg, err := GetConfig(ctx, self.Region, localAssumedRole)
	if err != nil {
		return err
	}
	locaEC2Client := ec2.NewFromConfig(localAWSCfg)

	log.Debugf("%s: creating new volume for snapshot %q in az %q", scan, localSnapshotID, self.AvailabilityZone)
	volume, err := locaEC2Client.CreateVolume(ctx, &ec2.CreateVolumeInput{
		VolumeType:        ec2types.VolumeTypeGp3,
		AvailabilityZone:  aws.String(self.AvailabilityZone),
		SnapshotId:        aws.String(localSnapshotID.ResourceName),
		TagSpecifications: cloudResourceTagSpec(types.ResourceTypeVolume, scan.ScannerHostname),
	})
	if err != nil {
		return fmt.Errorf("could not create volume from snapshot: %w", err)
	}

	volumeID, err := EC2CloudID(localSnapshotID.Region, localSnapshotID.AccountID, types.ResourceTypeVolume, *volume.VolumeId)
	if err != nil {
		return err
	}
	scan.AttachedVolumeID = &volumeID
	scan.AttachedVolumeCreatedAt = volume.CreateTime

	device, ok := devices.NextXen()
	if !ok {
		return fmt.Errorf("could not find non busy XEN block device")
	}
	scan.AttachedDeviceName = &device

	log.Debugf("%s: attaching volume %q into device %q", scan, *volume.VolumeId, device)
	var errAttach error
	for i := 0; i < maxAttachRetries; i++ {
		sleep := 2 * time.Second
		if !sleepCtx(ctx, sleep) {
			return ctx.Err()
		}
		_, errAttach = locaEC2Client.AttachVolume(ctx, &ec2.AttachVolumeInput{
			InstanceId: aws.String(self.InstanceID),
			VolumeId:   volume.VolumeId,
			Device:     aws.String(device),
		})
		if errAttach == nil {
			log.Debugf("%s: volume attached successfully %q device=%s", scan, *volume.VolumeId, device)
			break
		}
		var aerr smithy.APIError
		// NOTE(jinroh): we're trying to attach a volume in not yet in an
		// 'available' state. Continue.
		if errors.As(errAttach, &aerr) && aerr.ErrorCode() == "IncorrectState" {
			log.Tracef("%s: couldn't attach volume %q into device %q; retrying after %v (%d/%d)", scan, *volume.VolumeId, device, sleep, i+1, maxAttachRetries)
		} else {
			break
		}
	}
	if errAttach != nil {
		return fmt.Errorf("could not attach volume %q into device %q: %w", *volume.VolumeId, device, errAttach)
	}

	// NOTE(jinroh): we identified that on some Linux kernel the device path
	// may not be the expected one (passed to AttachVolume). The kernel may
	// map the block device to another path. However, the serial number
	// associated with the volume is always of the form volXXX (not vol-XXX).
	// So we use both the expected device path AND the serial number to find
	// the actual block device path.
	serialNumber := "vol" + strings.TrimPrefix(*volume.VolumeId, "vol-") // vol-XXX => volXXX
	foundBlockDevice, err := devices.Poll(ctx, scan, device, &serialNumber)
	if err != nil {
		return err
	}
	if foundBlockDevice.Path != *scan.AttachedDeviceName {
		scan.AttachedDeviceName = &foundBlockDevice.Path
	}
	return nil
}

// CleanupScanEBS removes all resources associated with a scan.
func CleanupScanEBS(ctx context.Context, scan *types.ScanTask) {
	for snapshotIDString, snapshotCreatedAt := range scan.CreatedSnapshots {
		snapshotID, err := types.ParseCloudID(snapshotIDString, types.ResourceTypeSnapshot)
		if err != nil {
			continue
		}
		cfg, err := GetConfig(ctx, snapshotID.Region, scan.Roles[snapshotID.AccountID])
		if err != nil {
			log.Errorf("%s: %v", scan, err)
		} else {
			ec2client := ec2.NewFromConfig(cfg)
			log.Debugf("%s: deleting snapshot %q", scan, snapshotID)
			if _, err := ec2client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
				SnapshotId: aws.String(snapshotID.ResourceName),
			}); err != nil {
				log.Warnf("%s: could not delete snapshot %s: %v", scan, snapshotID, err)
			} else {
				log.Debugf("%s: snapshot deleted %s", scan, snapshotID)
				statsResourceTTL(types.ResourceTypeSnapshot, scan, *snapshotCreatedAt)
			}
		}
	}

	switch scan.DiskMode {
	case types.VolumeAttach:
		if volumeID := scan.AttachedVolumeID; volumeID != nil {
			if errd := CleanupScanVolumes(ctx, scan, *volumeID, scan.Roles); errd != nil {
				log.Warnf("%s: could not delete volume %q: %v", scan, volumeID, errd)
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
func CleanupScanVolumes(ctx context.Context, maybeScan *types.ScanTask, volumeID types.CloudID, roles types.RolesMapping) error {
	cfg, err := GetConfig(ctx, volumeID.Region, roles[volumeID.AccountID])
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
			VolumeId: aws.String(volumeID.ResourceName),
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
			VolumeId: aws.String(volumeID.ResourceName),
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

func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

// EC2CloudID returns an ARN for the given EC2 resource.
func EC2CloudID(region, accountID string, resourceType types.ResourceType, resourceName string) (types.CloudID, error) {
	return types.ParseCloudID(fmt.Sprintf("arn:aws:ec2:%s:%s:%s/%s", region, accountID, resourceType, resourceName))
}

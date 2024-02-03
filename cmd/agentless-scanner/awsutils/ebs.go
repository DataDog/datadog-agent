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

	assumedRole := scan.Roles[scan.ARN.AccountID]
	cfg, err := GetConfig(ctx, scan.ARN.Region, assumedRole)
	if err != nil {
		return nil, err
	}

	ec2client := ec2.NewFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	var snapshotARN types.ARN
	switch scan.ARN.ResourceType {
	case types.ResourceTypeVolume:
		snapshotARN, err = CreateSnapshot(ctx, scan, waiter, ec2client, scan.ARN)
		if err != nil {
			return nil, err
		}
	case types.ResourceTypeSnapshot:
		snapshotARN = scan.ARN
	default:
		return nil, fmt.Errorf("ebs-volume: bad arn %q", scan.ARN)
	}

	log.Infof("%s: start EBS scanning", scan)

	switch scan.DiskMode {
	case types.VolumeAttach:
		if err := AttachSnapshotWithVolume(ctx, scan, waiter, snapshotARN); err != nil {
			return nil, err
		}
	case types.NBDAttach:
		ebsclient := ebs.NewFromConfig(cfg)
		if err := AttachSnapshotWithNBD(ctx, scan, snapshotARN, ebsclient); err != nil {
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

// CreateSnapshot creates a snapshot of the given EBS volume and returns its ARN.
func CreateSnapshot(ctx context.Context, scan *types.ScanTask, waiter *SnapshotWaiter, ec2client *ec2.Client, volumeARN types.ARN) (types.ARN, error) {
	snapshotCreatedAt := time.Now()
	if err := statsd.Count("datadog.agentless_scanner.snapshots.started", 1.0, scan.Tags(), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	log.Debugf("%s: starting volume snapshotting %q", scan, volumeARN)

	retries := 0
retry:
	if volumeARN.ResourceType != types.ResourceTypeVolume {
		return types.ARN{}, fmt.Errorf("bad volume ARN %q: expecting a volume ARN", volumeARN)
	}
	volumeID := volumeARN.ResourceName
	createSnapshotOutput, err := ec2client.CreateSnapshot(ctx, &ec2.CreateSnapshotInput{
		VolumeId:          aws.String(volumeID),
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
				log.Debugf("%s: snapshot creation rate exceeded for volume %q; retrying after %v (%d/%d)", scan, volumeARN, d, retries, maxSnapshotRetries)
				if !sleepCtx(ctx, d) {
					return types.ARN{}, ctx.Err()
				}
				goto retry
			}
		}
		if isRateExceededError {
			log.Debugf("%s: snapshot creation rate exceeded for volume %q; skipping)", scan, volumeARN)
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
		return types.ARN{}, err
	}

	snapshotID := *createSnapshotOutput.SnapshotId
	snapshotARN := EC2ARN(volumeARN.Region, volumeARN.AccountID, types.ResourceTypeSnapshot, snapshotID)
	scan.CreatedSnapshots[snapshotARN.String()] = &snapshotCreatedAt

	err = <-waiter.Wait(ctx, snapshotARN, ec2client)
	if err == nil {
		snapshotDuration := time.Since(snapshotCreatedAt)
		log.Debugf("%s: volume snapshotting of %q finished successfully %q (took %s)", scan, volumeARN, snapshotID, snapshotDuration)
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
	return snapshotARN, err
}

// AttachSnapshotWithNBD attaches the given snapshot to the instance using a
// Network Block Device (NBD).
func AttachSnapshotWithNBD(ctx context.Context, scan *types.ScanTask, snapshotARN types.ARN, ebsclient *ebs.Client) error {
	device, ok := devices.NextNBD()
	if !ok {
		return fmt.Errorf("could not find non busy NBD block device")
	}
	backend, err := nbd.NewEBSBackend(ebsclient, snapshotARN)
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
func AttachSnapshotWithVolume(ctx context.Context, scan *types.ScanTask, waiter *SnapshotWaiter, snapshotARN types.ARN) error {
	if snapshotARN.ResourceType != types.ResourceTypeSnapshot {
		return fmt.Errorf("expected ARN for a snapshot: %s", snapshotARN.String())
	}
	snapshotID := snapshotARN.ResourceName
	self, err := GetSelfEC2InstanceIndentity(ctx)
	if err != nil {
		return fmt.Errorf("could not get EC2 instance identity: using attach volumes cannot work outside an EC2 instance: %w", err)
	}

	remoteAssumedRole := scan.Roles[snapshotARN.AccountID]
	remoteAWSCfg, err := GetConfig(ctx, snapshotARN.Region, remoteAssumedRole)
	if err != nil {
		return err
	}
	remoteEC2Client := ec2.NewFromConfig(remoteAWSCfg)

	var localSnapshotARN types.ARN
	if snapshotARN.Region != self.Region {
		log.Debugf("%s: copying snapshot %q into %q", scan, snapshotARN, self.Region)
		copySnapshotCreatedAt := time.Now()
		copySnapshot, err := remoteEC2Client.CopySnapshot(ctx, &ec2.CopySnapshotInput{
			SourceRegion: aws.String(snapshotARN.Region),
			// DestinationRegion: aws.String(self.Region): automatically filled by SDK
			SourceSnapshotId:  aws.String(snapshotID),
			TagSpecifications: cloudResourceTagSpec(types.ResourceTypeSnapshot, scan.ScannerHostname),
		})
		if err != nil {
			return fmt.Errorf("could not copy snapshot %q to %q: %w", snapshotARN, self.Region, err)
		}
		localSnapshotARN = EC2ARN(self.Region, snapshotARN.AccountID, types.ResourceTypeSnapshot, *copySnapshot.SnapshotId)
		log.Debugf("%s: waiting for copy of snapshot %q into %q as %q", scan, snapshotARN, self.Region, *copySnapshot.SnapshotId)
		err = <-waiter.Wait(ctx, localSnapshotARN, remoteEC2Client)
		if err != nil {
			return fmt.Errorf("could not finish copying %q to %q as %q: %w", snapshotARN, self.Region, *copySnapshot.SnapshotId, err)
		}
		log.Debugf("%s: successfully copied snapshot %q into %q: %q", scan, snapshotARN, self.Region, *copySnapshot.SnapshotId)
		scan.CreatedSnapshots[localSnapshotARN.String()] = &copySnapshotCreatedAt
	} else {
		localSnapshotARN = snapshotARN
	}

	if localSnapshotARN.AccountID != "" && localSnapshotARN.AccountID != self.AccountID {
		_, err = remoteEC2Client.ModifySnapshotAttribute(ctx, &ec2.ModifySnapshotAttributeInput{
			SnapshotId:    aws.String(snapshotID),
			Attribute:     ec2types.SnapshotAttributeNameCreateVolumePermission,
			OperationType: ec2types.OperationTypeAdd,
			UserIds:       []string{self.AccountID},
		})
		if err != nil {
			return fmt.Errorf("could not modify snapshot attributes %q for sharing with account ID %q: %w", localSnapshotARN, self.AccountID, err)
		}
	}

	localAssumedRole := scan.Roles[self.AccountID]
	localAWSCfg, err := GetConfig(ctx, self.Region, localAssumedRole)
	if err != nil {
		return err
	}
	locaEC2Client := ec2.NewFromConfig(localAWSCfg)

	log.Debugf("%s: creating new volume for snapshot %q in az %q", scan, localSnapshotARN, self.AvailabilityZone)
	localSnapshotID := localSnapshotARN.ResourceName
	volume, err := locaEC2Client.CreateVolume(ctx, &ec2.CreateVolumeInput{
		VolumeType:        ec2types.VolumeTypeGp3,
		AvailabilityZone:  aws.String(self.AvailabilityZone),
		SnapshotId:        aws.String(localSnapshotID),
		TagSpecifications: cloudResourceTagSpec(types.ResourceTypeVolume, scan.ScannerHostname),
	})
	if err != nil {
		return fmt.Errorf("could not create volume from snapshot: %w", err)
	}

	volumeARN := EC2ARN(localSnapshotARN.Region, localSnapshotARN.AccountID, types.ResourceTypeVolume, *volume.VolumeId)
	scan.AttachedVolumeARN = &volumeARN
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

func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

// EC2ARN returns an ARN for the given EC2 resource.
func EC2ARN(region, accountID string, resourceType types.ResourceType, resourceID string) types.ARN {
	return types.ARN{
		Partition: "aws",
		Service:   "ec2",
		Region:    region,
		AccountID: accountID,
		Resource:  fmt.Sprintf("%s/%s", resourceType, resourceID),
	}
}

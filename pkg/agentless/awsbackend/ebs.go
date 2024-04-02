// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package awsbackend

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/agentless/devices"
	"github.com/DataDog/datadog-agent/pkg/agentless/nbd"
	"github.com/DataDog/datadog-agent/pkg/agentless/types"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	ddogstatsd "github.com/DataDog/datadog-go/v5/statsd"

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

// SetupEBSSnapshot prepares the EBS snapshot for scanning.
func SetupEBSSnapshot(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, scan *types.ScanTask, waiter *ResourceWaiter) (types.CloudID, error) {
	cfg := GetConfigFromCloudID(ctx, statsd, sc, scan.Roles, scan.TargetID)
	ec2client := ec2.NewFromConfig(cfg)
	switch scan.TargetID.ResourceType() {
	case types.ResourceTypeVolume:
		return CreateSnapshot(ctx, statsd, scan, waiter, ec2client, scan.TargetID)
	case types.ResourceTypeSnapshot:
		// Check if the snapshot is already tagged as an agentless snapshot.
		poll := <-waiter.Wait(ctx, scan.TargetID, ec2client)
		if err := poll.Err; err != nil {
			return types.CloudID{}, err
		}
		snapshot := *poll.Snapshot
		if cloudResourceIsTagged(snapshot.Tags) {
			return scan.TargetID, nil
		}
		// Otherwise, copy the snapshot and tag it as an agentless snapshot.
		return copySnapshot(ctx, statsd, scan, waiter, ec2client, scan.TargetID, snapshot)
	case types.ResourceTypeHostImage:
		snapshotID, err := getAMIRootSnapshot(ctx, scan, waiter, ec2client, scan.TargetID)
		if err != nil {
			return types.CloudID{}, err
		}
		return CopySnapshot(ctx, statsd, scan, waiter, ec2client, snapshotID)
	default:
		return types.CloudID{}, fmt.Errorf("ebs-volume: unexpected resource type for task %q: %q", scan.Type, scan.TargetID)
	}
}

// SetupEBSVolume prepares the EBS volume for scanning. It attaches the given
// snapshot to the instance and returns the list of partitions that were
// mounted.
func SetupEBSVolume(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, scan *types.ScanTask, waiter *ResourceWaiter, snapshotID types.CloudID) error {
	switch scan.DiskMode {
	case types.DiskModeVolumeAttach:
		return AttachSnapshotWithVolume(ctx, statsd, sc, scan, waiter, snapshotID)
	case types.DiskModeNBDAttach:
		return AttachSnapshotWithNBD(ctx, statsd, sc, scan, snapshotID)
	case types.DiskModeNoAttach:
		return fmt.Errorf("ebs-volume: could not setup volume in no attach mode defined for task %q", scan)
	default:
		panic("unreachable")
	}
}

// CreateSnapshot creates a snapshot of the given EBS volume and returns its Cloud Identifier.
func CreateSnapshot(ctx context.Context, statsd ddogstatsd.ClientInterface, scan *types.ScanTask, waiter *ResourceWaiter, ec2client *ec2.Client, volumeID types.CloudID) (types.CloudID, error) {
	if err := statsd.Count("datadog.agentless_scanner.snapshots.started", 1.0, scan.Tags(), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	if volumeID.ResourceType() != types.ResourceTypeVolume {
		return types.CloudID{}, fmt.Errorf("bad resource ID %q: expecting a volume", volumeID)
	}
	log.Debugf("%s: starting volume snapshotting %q", scan, volumeID)
	for tryCount := 0; ; tryCount++ {
		snapshotCreatedAt := time.Now()
		createSnapshotOutput, err := ec2client.CreateSnapshot(ctx, &ec2.CreateSnapshotInput{
			VolumeId:          aws.String(volumeID.ResourceName()),
			TagSpecifications: cloudResourceTagSpec(scan, volumeID, types.ResourceTypeSnapshot),
		})
		if err != nil {
			var aerr smithy.APIError
			var isRateExceededError bool
			var isVolumeNotFoundError bool
			if errors.As(err, &aerr) {
				if aerr.ErrorCode() == "SnapshotCreationPerVolumeRateExceeded" {
					isRateExceededError = true
				}
				if aerr.ErrorCode() == "InvalidVolume.NotFound" {
					isVolumeNotFoundError = true
				}
			}
			if isRateExceededError && tryCount < maxSnapshotRetries {
				// https://docs.aws.amazon.com/AWSEC2/latest/APIReference/errors-overview.html
				// Wait at least 15 seconds between concurrent volume snapshots.
				d := 15 * time.Second
				log.Debugf("%s: snapshot creation rate exceeded for volume %q; retrying after %v (%d/%d)", scan, volumeID, d, tryCount+1, maxSnapshotRetries)
				if !sleepCtx(ctx, d) {
					return types.CloudID{}, ctx.Err()
				}
				continue
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

		snapshotID, err := types.AWSCloudID(volumeID.Region(), volumeID.AccountID(), types.ResourceTypeSnapshot, *createSnapshotOutput.SnapshotId)
		if err != nil {
			return snapshotID, err
		}
		scan.PushCreatedResource(snapshotID, snapshotCreatedAt)

		poll := <-waiter.Wait(ctx, snapshotID, ec2client)
		if err := poll.Err; err != nil {
			if err := statsd.Count("datadog.agentless_scanner.snapshots.finished", 1.0, scan.TagsFailure(err), 1.0); err != nil {
				log.Warnf("failed to send metric: %v", err)
			}
			return types.CloudID{}, err
		}

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
		return snapshotID, err
	}
}

// CopySnapshot copies an EBS snapshot.
func CopySnapshot(ctx context.Context, statsd ddogstatsd.ClientInterface, scan *types.ScanTask, waiter *ResourceWaiter, ec2client *ec2.Client, snapshotID types.CloudID) (types.CloudID, error) {
	poll := <-waiter.Wait(ctx, snapshotID, ec2client)
	if err := poll.Err; err != nil {
		return types.CloudID{}, err
	}
	snapshot := *poll.Snapshot
	return copySnapshot(ctx, statsd, scan, waiter, ec2client, snapshotID, snapshot)
}

func copySnapshot(ctx context.Context, statsd ddogstatsd.ClientInterface, scan *types.ScanTask, waiter *ResourceWaiter, ec2client *ec2.Client, snapshotID types.CloudID, snapshot ec2types.Snapshot) (types.CloudID, error) {
	self, err := getSelfEC2InstanceIndentity(ctx)
	if err != nil {
		return types.CloudID{}, fmt.Errorf("could not get EC2 instance identity: using attach volumes cannot work outside an EC2 instance: %w", err)
	}

	log.Debugf("%s: copying snapshot %s", scan, snapshotID)
	copyCreatedAt := time.Now()
	copyOutput, err := ec2client.CopySnapshot(ctx, &ec2.CopySnapshotInput{
		SourceRegion:      aws.String(snapshotID.Region()),
		SourceSnapshotId:  aws.String(snapshotID.ResourceName()),
		TagSpecifications: cloudResourceTagSpec(scan, snapshotID, types.ResourceTypeSnapshot),
		Encrypted:         snapshot.Encrypted,
		KmsKeyId:          snapshot.KmsKeyId,
	})
	if err != nil {
		return types.CloudID{}, fmt.Errorf("copying snapshot %q: %w", snapshotID, err)
	}

	copiedSnapshotID, err := types.AWSCloudID(self.Region, snapshotID.AccountID(), types.ResourceTypeSnapshot, *copyOutput.SnapshotId)
	if err != nil {
		return types.CloudID{}, err
	}
	scan.PushCreatedResource(copiedSnapshotID, copyCreatedAt)

	poll := <-waiter.Wait(ctx, copiedSnapshotID, ec2client)
	if err := poll.Err; err != nil {
		if err := statsd.Count("datadog.agentless_scanner.snapshots.copies.finished", 1.0, scan.TagsFailure(err), 1.0); err != nil {
			log.Warnf("failed to send metric: %v", err)
		}
		return types.CloudID{}, fmt.Errorf("waiting for copied snapshot %q: %w", copiedSnapshotID, err)
	}

	copyDuration := time.Since(copyCreatedAt)
	log.Debugf("%s: snapshot copy of %q finished successfully %q (took %s)", scan, snapshotID, copiedSnapshotID, copyDuration)
	if err := statsd.Histogram("datadog.agentless_scanner.snapshots.copies.duration", float64(copyDuration.Milliseconds()), scan.Tags(), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	if err := statsd.Count("datadog.agentless_scanner.snapshots.copies.finished", 1.0, scan.TagsSuccess(), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	return copiedSnapshotID, nil
}

// AttachSnapshotWithNBD attaches the given snapshot to the instance using a
// Network Block Device (NBD).
func AttachSnapshotWithNBD(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, scan *types.ScanTask, snapshotID types.CloudID) error {
	ebsclient := ebs.NewFromConfig(GetConfigFromCloudID(ctx, statsd, sc, scan.Roles, scan.TargetID))
	device, ok := devices.NextNBD()
	if !ok {
		return fmt.Errorf("could not find non busy NBD block device")
	}
	backend, err := nbd.NewEBSBackend(ebsclient, snapshotID)
	if err != nil {
		return err
	}
	if err := nbd.StartNBDBlockDevice(scan, device, backend); err != nil {
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
func AttachSnapshotWithVolume(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, scan *types.ScanTask, waiter *ResourceWaiter, snapshotID types.CloudID) error {
	self, err := getSelfEC2InstanceIndentity(ctx)
	if err != nil {
		return fmt.Errorf("could not get EC2 instance identity: using attach volumes cannot work outside an EC2 instance: %w", err)
	}

	remoteEC2Client := ec2.NewFromConfig(GetConfigFromCloudID(ctx, statsd, sc, scan.Roles, snapshotID))
	var localSnapshotID types.CloudID
	if snapshotID.Region() != self.Region {
		localSnapshotID, err = CopySnapshot(ctx, statsd, scan, waiter, remoteEC2Client, snapshotID)
	} else {
		localSnapshotID = snapshotID
	}
	if err != nil {
		return err
	}

	locaEC2Client := ec2.NewFromConfig(GetConfig(ctx, statsd, sc, self.Region, scan.Roles.GetRole(types.CloudProviderAWS, self.AccountID)))
	if localSnapshotID.AccountID() != "" && localSnapshotID.AccountID() != self.AccountID {
		_, err = remoteEC2Client.ModifySnapshotAttribute(ctx, &ec2.ModifySnapshotAttributeInput{
			SnapshotId:    aws.String(snapshotID.ResourceName()),
			Attribute:     ec2types.SnapshotAttributeNameCreateVolumePermission,
			OperationType: ec2types.OperationTypeAdd,
			UserIds:       []string{self.AccountID},
		})
		if err != nil {
			return fmt.Errorf("could not modify snapshot attributes %q for sharing with account ID %q: %w", localSnapshotID, self.AccountID, err)
		}
	}

	log.Debugf("%s: creating new volume for snapshot %q in az %q", scan, localSnapshotID, self.AvailabilityZone)
	volume, err := locaEC2Client.CreateVolume(ctx, &ec2.CreateVolumeInput{
		VolumeType:        ec2types.VolumeTypeGp3,
		AvailabilityZone:  aws.String(self.AvailabilityZone),
		SnapshotId:        aws.String(localSnapshotID.ResourceName()),
		TagSpecifications: cloudResourceTagSpec(scan, localSnapshotID, types.ResourceTypeVolume),
	})
	if err != nil {
		return fmt.Errorf("could not create volume from snapshot: %w", err)
	}

	volumeID, err := types.AWSCloudID(localSnapshotID.Region(), localSnapshotID.AccountID(), types.ResourceTypeVolume, *volume.VolumeId)
	if err != nil {
		return err
	}
	scan.PushCreatedResource(volumeID, *volume.CreateTime)

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
func CleanupScanEBS(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, scan *types.ScanTask, resourceID types.CloudID) error {
	switch resourceID.ResourceType() {
	case types.ResourceTypeVolume:
		if err := CleanupScanVolume(ctx, statsd, sc, scan, resourceID, scan.Roles); err != nil {
			return fmt.Errorf("could not delete volume %q: %v", resourceID, err)
		}
	case types.ResourceTypeSnapshot:
		if err := CleanupScanSnapshot(ctx, statsd, sc, scan, resourceID, scan.Roles); err != nil {
			return fmt.Errorf("could not delete snapshot %s: %v", resourceID, err)
		}
	}
	return nil
}

// CleanupScanSnapshot cleans up a snapshot resource.
func CleanupScanSnapshot(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, maybeScan *types.ScanTask, snapshotID types.CloudID, roles types.RolesMapping) error {
	log.Debugf("%s: deleting snapshot %q", maybeScan, snapshotID)
	ec2client := ec2.NewFromConfig(GetConfigFromCloudID(ctx, statsd, sc, roles, snapshotID))
	if _, err := ec2client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
		SnapshotId: aws.String(snapshotID.ResourceName()),
	}); err != nil {
		return err
	}
	log.Debugf("%s: snapshot deleted %s", maybeScan, snapshotID)
	return nil
}

// CleanupScanVolume cleans up a volume resource.
func CleanupScanVolume(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, maybeScan *types.ScanTask, volumeID types.CloudID, roles types.RolesMapping) error {
	ec2client := ec2.NewFromConfig(GetConfigFromCloudID(ctx, statsd, sc, roles, volumeID))
	volumeNotFound := false
	volumeDetached := false
	log.Debugf("%s: detaching volume %q", maybeScan, volumeID)
	for i := 0; i < 5; i++ {
		if _, err := ec2client.DetachVolume(ctx, &ec2.DetachVolumeInput{
			Force:    aws.Bool(true),
			VolumeId: aws.String(volumeID.ResourceName()),
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
			if err != nil {
				log.Warnf("%s: could not list devices: %v", maybeScan, err)
				break
			}
			if len(devices) == 0 {
				log.Debugf("%s: volume detached %s", maybeScan, volumeID)
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
			VolumeId: aws.String(volumeID.ResourceName()),
		})
		if errd != nil {
			var aerr smithy.APIError
			if errors.As(errd, &aerr) && aerr.ErrorCode() == "InvalidVolume.NotFound" {
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

	log.Debugf("%s: volume deleted %s", maybeScan, volumeID)
	return nil
}

// getAMIRootSnapshot returns the root snapshot of an AMI.
func getAMIRootSnapshot(ctx context.Context, _ *types.ScanTask, waiter *ResourceWaiter, ec2client *ec2.Client, imageID types.CloudID) (types.CloudID, error) {
	poll := <-waiter.Wait(ctx, imageID, ec2client)
	if err := poll.Err; err != nil {
		return types.CloudID{}, fmt.Errorf("could not find image %q: %w", imageID, err)
	}
	image := poll.Image
	for _, blockDeviceMapping := range image.BlockDeviceMappings {
		if image.RootDeviceName == nil {
			continue
		}
		if blockDeviceMapping.DeviceName == nil {
			continue
		}
		if blockDeviceMapping.Ebs == nil {
			continue
		}
		if *blockDeviceMapping.DeviceName == *image.RootDeviceName {
			return types.AWSCloudID(imageID.Region(), imageID.AccountID(), types.ResourceTypeSnapshot, *blockDeviceMapping.Ebs.SnapshotId)
		}
	}
	return types.CloudID{}, fmt.Errorf("could not find root snapshot for AMI %q", imageID)
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

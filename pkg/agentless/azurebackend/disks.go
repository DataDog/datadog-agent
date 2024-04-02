// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package azurebackend

import (
	"context"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
	"github.com/DataDog/datadog-agent/pkg/agentless/devices"
	"github.com/DataDog/datadog-agent/pkg/agentless/nbd"
	"github.com/DataDog/datadog-agent/pkg/agentless/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/uuid"
	"time"
)

// SetupDisk prepares the disk for scanning.
// It creates a snapshot of the disk and attaches it to the VM.
func SetupDisk(ctx context.Context, cfg Config, scan *types.ScanTask) (err error) {
	var snapshot *armcompute.Snapshot

	switch scan.TargetID.ResourceType() {
	case types.ResourceTypeVolume:
		snapshot, err = createSnapshot(ctx, cfg, scan, scan.TargetID)
	//case types.ResourceTypeSnapshot:
	//	snapshotID, err = CopySnapshot(ctx, cfg, scan, waiter, scan.TargetID)
	default:
		err = fmt.Errorf("SetupDisk: unexpected resource type for task %q: %q", scan.Type, scan.TargetID)
	}
	if err != nil {
		return err
	}

	switch scan.DiskMode {
	case types.DiskModeVolumeAttach:
		return fmt.Errorf("SetupDisk: unimplemented attach mode '%q' for task %q", scan.Type, scan.TargetID)
	case types.DiskModeNBDAttach:
		return attachSnapshotWithNBD(ctx, cfg, scan, snapshot)
	case types.DiskModeNoAttach:
		return nil // nothing to do. early exit.
	default:
		panic("unreachable")
	}
}

func createSnapshot(ctx context.Context, cfg Config, scan *types.ScanTask, diskCloudID types.CloudID) (*armcompute.Snapshot, error) {
	diskID, err := diskCloudID.AsAzureID()
	if err != nil {
		return nil, log.Error(err)
	}

	snapshotID, err := arm.ParseResourceID(fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/%s/%s",
		cfg.ScannerSubscription,
		cfg.ScannerResourceGroup,
		arm.NewResourceType("Microsoft.Compute", "snapshots"),
		"dd-agentless-scanner-"+uuid.NewString()))
	if err != nil {
		return nil, err
	}
	snapshotCloudID := types.AzureCloudID(snapshotID)

	disksClient := cfg.ComputeClientFactory.NewDisksClient()
	disk, err := disksClient.Get(ctx, diskID.ResourceGroupName, diskID.Name, nil)
	if err != nil {
		return nil, log.Error(err)
	}

	snapshotsClient := cfg.ComputeClientFactory.NewSnapshotsClient()
	snapshotCreatedAt := time.Now()
	poller, err := snapshotsClient.BeginCreateOrUpdate(
		ctx,
		snapshotID.ResourceGroupName,
		snapshotID.Name,
		armcompute.Snapshot{
			Location: disk.Location, // TODO change location
			Properties: &armcompute.SnapshotProperties{
				CreationData: &armcompute.CreationData{
					CreateOption:     to.Ptr(armcompute.DiskCreateOptionCopy),
					SourceResourceID: disk.ID,
				},
			},
		},
		nil,
	)
	if err != nil {
		return nil, err
	}

	scan.PushCreatedResource(snapshotCloudID, snapshotCreatedAt)
	res, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		if err := statsd.Count("datadog.agentless_scanner.snapshots.finished", 1.0, scan.TagsFailure(err), 1.0); err != nil {
			log.Warnf("failed to send metric: %v", err)
		}
		return nil, err
	}

	snapshot := &res.Snapshot
	snapshotDuration := time.Since(snapshotCreatedAt)
	log.Debugf("%s: volume snapshotting of %q finished successfully %q (took %s)", scan, diskID, snapshotCloudID, snapshotDuration)
	if err := statsd.Histogram("datadog.agentless_scanner.snapshots.duration", float64(snapshotDuration.Milliseconds()), scan.Tags(), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	if err := statsd.Histogram("datadog.agentless_scanner.snapshots.size", float64(*snapshot.Properties.DiskSizeGB), scan.TagsFailure(err), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	if err := statsd.Count("datadog.agentless_scanner.snapshots.finished", 1.0, scan.TagsSuccess(), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}

	return snapshot, nil
}

// attachSnapshotWithNBD attaches the given snapshot to the VM using a Network Block Device (NBD).
func attachSnapshotWithNBD(ctx context.Context, cfg Config, scan *types.ScanTask, snapshot *armcompute.Snapshot) error {
	device, ok := devices.NextNBD()
	if !ok {
		return fmt.Errorf("could not find non busy NBD block device")
	}
	backend, err := nbd.NewAzureBackend(cfg.ComputeClientFactory.NewSnapshotsClient(), snapshot)
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

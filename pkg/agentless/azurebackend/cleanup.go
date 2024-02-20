// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package azurebackend

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/agentless/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CleanupScan removes all resources associated with a scan.
func CleanupScan(ctx context.Context, cfg Config, scan *types.ScanTask, resourceID types.CloudID) error {
	switch resourceID.ResourceType() {
	case types.ResourceTypeVolume:
		//if err := CleanupScanVolume(ctx, scan, resourceID, scan.Roles); err != nil {
		//	return fmt.Errorf("could not delete volume %q: %v", resourceID, err)
		//}
		return fmt.Errorf("volume cleanup not implemented")
	case types.ResourceTypeSnapshot:
		if err := cleanupScanSnapshot(ctx, cfg, scan, resourceID); err != nil {
			return fmt.Errorf("could not delete snapshot %s: %v", resourceID, err)
		}
	}
	return nil
}

// cleanupScanSnapshot cleans up a snapshot resource.
func cleanupScanSnapshot(ctx context.Context, cfg Config, maybeScan *types.ScanTask, snapshotCloudID types.CloudID) error {
	log.Debugf("%s: deleting snapshot %q", maybeScan, snapshotCloudID)

	snapshotID, err := snapshotCloudID.AsAzureID()
	if err != nil {
		return err
	}

	snapshotsClient := cfg.ComputeClientFactory.NewSnapshotsClient()
	pollerRevoke, err := snapshotsClient.BeginRevokeAccess(ctx, snapshotID.ResourceGroupName, snapshotID.Name, nil)
	if err != nil {
		return err
	}
	if _, err = pollerRevoke.PollUntilDone(ctx, nil); err != nil {
		return err
	}

	pollerDelete, err := snapshotsClient.BeginDelete(ctx, snapshotID.ResourceGroupName, snapshotID.Name, nil)
	if err != nil {
		return err
	}
	if _, err = pollerDelete.PollUntilDone(ctx, nil); err != nil {
		return err
	}

	log.Debugf("%s: snapshot deleted %s", maybeScan, snapshotID)

	return nil
}

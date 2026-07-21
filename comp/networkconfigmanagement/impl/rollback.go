// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"context"
	"fmt"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	ncmremote "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/remote"
	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// RollbackConfig rolls back a device to a previous configuration that's saved
// locally on this agent. If any commands are sent to the device, the returned
// PushResult will document them and their results. NOTE: the returned error
// will be non-nil if ANYTHING went wrong, including the later stages of the
// config push or checks performed after the push completed. This means that
// even if this func returns an error, the config may still have been
// successfully rolled back, or partially rolled back - check the returned
// PushResult to see what commands were actually run on the device.
func (n *networkDeviceConfigImpl) RollbackConfig(ctx context.Context, deviceID string, configVersion string, hash string) (result *ncmremote.PushResult, rberr types.RollbackError) {
	if n.store == nil {
		return nil, types.RollbackDisabled
	}
	var log log.Component = NewLogWrapper(n.log, fmt.Sprintf("ncm[%s]: ", deviceID))
	log.Infof("Rollback requested: Device %q to version %q", deviceID, configVersion)
	ctx = WithLogger(ctx, log)
	dc, err := n.devices.GetAndLock(ctx, deviceID)
	if err != nil {
		return nil, types.AsRollbackError(err)
	}
	defer dc.UnlockOrLog(log)

	rawConfig, metadata, err := n.store.GetConfig(configVersion)
	if err != nil {
		return nil, types.AsRollbackError(err)
	}
	if metadata.DeviceID != deviceID {
		return nil, types.WrapErrorf(types.ErrWrongDeviceID, "input mismatch: config %q is not for device %q", configVersion, deviceID)
	}

	expectedHash := ncmstore.HashConfig(rawConfig)
	if expectedHash != hash {
		return nil, types.WrapErrorf(types.ErrWrongHash, "hash mismatch for config %q", configVersion)
	}

	conn, rberr := n.connectAndEnsureProfile(ctx, dc)
	if rberr != nil {
		return nil, rberr
	}
	defer conn.Close()

	result, err = conn.PushConfig(ctx, rawConfig)
	if err != nil {
		return result, types.AsRollbackError(err)
	}

	if err := n.reportConfig(ctx, dc, n.sender); err != nil {
		return result, types.AsRollbackError(err)
	}
	return result, nil
}

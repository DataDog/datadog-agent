// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"context"
	"errors"
	"fmt"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	ncmremote "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/remote"
	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
)

type RollbackDisabled struct{}

func (r *RollbackDisabled) Error() string {
	return "rollback is disabled"
}

type ArgumentError struct {
	wrapped error
}

func (a *ArgumentError) Error() string {
	return a.wrapped.Error()
}

func (a *ArgumentError) Unwrap() error {
	return a.wrapped
}

// RollbackConfig rolls back a device to a previous configuration that's saved
// locally on this agent. If any commands are sent to the device, the returned
// PushResult will document them and their results. NOTE: the returned error
// will be non-nil if ANYTHING went wrong, including the later stages of the
// config push or checks performed after the push completed. This means that
// even if this func returns an error, the config may still have been
// successfully rolled back, or partially rolled back - check the returned
// PushResult to see what commands were actually run on the device.
func (n *networkDeviceConfigImpl) RollbackConfig(ctx context.Context, deviceID string, configVersion string, hash string) (*ncmremote.PushResult, error) {
	if n.store == nil {
		return nil, &RollbackDisabled{}
	}
	var log log.Component = NewLogWrapper(n.log, fmt.Sprintf("ncm[%s]: ", deviceID))
	log.Infof("Rollback requested: Device %q to version %q", deviceID, configVersion)
	ctx = WithLogger(ctx, log)
	dc, err := n.devices.GetAndLock(ctx, deviceID)
	if err != nil {
		// UnknownDeviceError -> the deviceID is bad.
		if errors.Is(err, &UnknownDeviceError{}) {
			return nil, &ArgumentError{err}
		}
		return nil, err
	}
	defer dc.UnlockOrLog(log)

	rawConfig, metadata, err := n.store.GetConfig(configVersion)
	if err != nil {
		// Can be [UnknownUUIDError] or various internal errors
		// UnknownUUIDError -> the deviceID is bad.
		if errors.Is(err, &ncmstore.UnknownUUIDError{}) {
			return nil, &ArgumentError{err}
		}
		return nil, err
	}
	if metadata.DeviceID != deviceID {
		return nil, &ArgumentError{fmt.Errorf("input mismatch: config %q is not for device %q", configVersion, deviceID)}
	}

	expectedHash := ncmstore.HashConfig(rawConfig)
	if expectedHash != hash {
		return nil, &ArgumentError{fmt.Errorf("hash mismatch for config %q", configVersion)}
	}

	conn, err := n.connectAndEnsureProfile(ctx, dc)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", deviceID, err)
	}
	defer conn.Close()

	result, err := conn.PushConfig(ctx, rawConfig)
	if err != nil {
		return result, err
	}

	if err := n.reportConfig(ctx, dc, n.sender); err != nil {
		return result, err
	}
	return result, nil
}

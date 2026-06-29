// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"context"
	"errors"
	"fmt"
)

// PinConfig marks a config as pinned in the local boltdb store so it is
// excluded from eviction.
func (n *networkDeviceConfigImpl) PinConfig(_ context.Context, deviceID string, configID string, hash string) error {
	if n.store == nil {
		return errors.New("rollback is disabled")
	}

	_, metadata, err := n.store.GetConfig(configID)
	if err != nil {
		return fmt.Errorf("config not found in local store (may have been evicted)")
	}
	if metadata.DeviceID != deviceID {
		return fmt.Errorf("input mismatch: config %q is not for device %q", configID, deviceID)
	}
	if metadata.RawHash != hash {
		return fmt.Errorf("hash mismatch for config %q", configID)
	}

	return n.store.SetPinned(configID, true)
}

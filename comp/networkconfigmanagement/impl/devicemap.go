// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"context"
	"fmt"
	"time"

	ncmconfig "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	ncmprofile "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
)

// DeviceMap wraps a sync.Map of DeviceContexts to streamline the process of
// fetching and locking them.
type DeviceMap struct {
	devices *Map[*DeviceContext]
	timeout time.Duration
}

// NewDeviceMap creates an empty DeviceMap. timeout is the maximum time to wait
// for a device lock when calling RegisterDevice or GetAndLock.
func NewDeviceMap(timeout time.Duration) *DeviceMap {
	return &DeviceMap{
		devices: NewMap[*DeviceContext](),
		timeout: timeout,
	}
}

// RegisterDevice adds this device to the map; if an entry with this deviceID
// already exists, it will be updated instead.
func (d *DeviceMap) RegisterDevice(ctx context.Context, device *ncmconfig.DeviceInstance, profile *ncmprofile.NCMProfile) error {
	dc, loaded := d.devices.LoadOrStore(device.DeviceID(), NewDeviceContext(device, profile))
	if !loaded {
		// The item we got is the one we just created with NewDeviceContext, so
		// it doesn't need to be reset.
		return nil
	}
	err := dc.Lock(ctx, d.timeout)
	if err != nil {
		return err
	}
	dc.SetDevice(device, profile)
	return dc.Unlock()
}

func (d *DeviceMap) Get(deviceID string) (*DeviceContext, error) {
	dc, ok := d.devices.Load(deviceID)
	if !ok {
		return nil, fmt.Errorf("unknown device: %q", deviceID)
	}
	return dc, nil
}

func (d *DeviceMap) GetAndLock(ctx context.Context, deviceID string) (*DeviceContext, error) {
	dc, err := d.Get(deviceID)
	if err != nil {
		return nil, err
	}
	err = dc.Lock(ctx, d.timeout)
	if err != nil {
		return nil, err
	}
	return dc, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ncmprofile "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
)

func TestDeviceMap(t *testing.T) {
	dm := NewDeviceMap(time.Millisecond)
	device := createTestDevice()
	p1 := &ncmprofile.NCMProfile{Name: "p1"}
	p2 := &ncmprofile.NCMProfile{Name: "p2"}

	err := dm.RegisterDevice(t.Context(), device, p1)
	require.NoError(t, err)

	dc, err := dm.Get(device.DeviceID())
	require.NoError(t, err)
	assert.Equal(t, "p1", dc.profile.Name)

	err = dm.RegisterDevice(t.Context(), device, p2)
	require.NoError(t, err)

	dc, err = dm.Get(device.DeviceID())
	require.NoError(t, err)
	assert.Equal(t, "p2", dc.profile.Name)
}

func TestDeviceMap_Get_Unknown(t *testing.T) {
	dm := NewDeviceMap(time.Millisecond)

	_, err := dm.Get("nonexistent")
	assert.ErrorContains(t, err, "unknown device")
}

func TestDeviceMap_GetAndLock_Unknown(t *testing.T) {
	dm := NewDeviceMap(time.Millisecond)

	_, err := dm.GetAndLock(t.Context(), "nonexistent")
	assert.ErrorContains(t, err, "unknown device")
}

func TestDeviceMap_GetAndLock_LocksDevice(t *testing.T) {
	dm := NewDeviceMap(time.Millisecond)
	device := createTestDevice()

	err := dm.RegisterDevice(t.Context(), device, nil)
	require.NoError(t, err)

	dc, err := dm.GetAndLock(t.Context(), device.DeviceID())
	require.NoError(t, err)
	assert.NotNil(t, dc)

	// GetAndLock should time out.
	_, err = dm.GetAndLock(t.Context(), device.DeviceID())
	require.ErrorIs(t, err, context.DeadlineExceeded)

	// RegisterDevice should also time out.
	err = dm.RegisterDevice(t.Context(), device, nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	// After unlocking, GetAndLock should succeed again.
	require.NoError(t, dc.Unlock())
	dc2, err := dm.GetAndLock(t.Context(), device.DeviceID())
	require.NoError(t, err)
	assert.NotNil(t, dc2)
	require.NoError(t, dc2.Unlock())
}

func TestDeviceMap_ExtraUnlock(t *testing.T) {
	dm := NewDeviceMap(time.Millisecond)
	device := createTestDevice()

	err := dm.RegisterDevice(t.Context(), device, nil)
	require.NoError(t, err)

	dc, err := dm.Get(device.DeviceID())
	require.NoError(t, err)
	require.ErrorContains(t, dc.Unlock(), "unlocked")
}

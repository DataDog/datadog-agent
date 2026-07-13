// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"context"
	"errors"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	ncmconfig "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	ncmprofile "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
)

// DeviceContext is a wrapper around some information about a device. It also
// functions as a lock, so that if multiple threads try to access the same
// device at the same time (e.g. because multiple rollbacks are triggered
// simultaneously, or a rollback runs at the exact moment that the config check
// is running) these will happen serially.
type DeviceContext struct {
	device *ncmconfig.DeviceInstance
	// lastReportTime is the last time this device's config was reported to the
	// backend. It is only used for metrics tracking how frequently devices are
	// being checked.
	lastReportTime time.Time
	profile        *ncmprofile.NCMProfile
	lock           chan bool
	// noMatchingProfile is set when .device.Profile is empty and we have tried
	// every known profile and found no matches. This way we don't try again on
	// every check - we just report the error again.
	noMatchingProfile bool
}

func NewDeviceContext(device *ncmconfig.DeviceInstance, profile *ncmprofile.NCMProfile) *DeviceContext {
	return &DeviceContext{
		device:  device,
		profile: profile,
		lock:    make(chan bool, 1),
	}
}

// SetDevice updates the device config (useful e.g. if the check configuration
// is reloaded).
func (dc *DeviceContext) SetDevice(device *ncmconfig.DeviceInstance, profile *ncmprofile.NCMProfile) {
	dc.device = device
	dc.profile = profile
	// clear noMatchingProfile - if profile is nil, then the next time this
	// device is checked we'll test every available profile.
	dc.noMatchingProfile = false
}

// Lock blocks until this device is available and then locks it.
func (dc *DeviceContext) Lock(ctx context.Context, timeout time.Duration) error {
	if timeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	select {
	case dc.lock <- true:
		// writing a bool locks it, because the channel only holds one element.
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}

}

// Unlock unlocks this device. It returns an error if the device is not locked.
func (dc *DeviceContext) Unlock() error {
	select {
	case <-dc.lock:
		// Reading the single element from the channel unlocks the lock.
		return nil
	default:
		return errors.New("called Unlock() on unlocked DeviceContext")
	}
}

// UnlockOrLog unlocks this device, logging any errors to the given logger.
func (dc *DeviceContext) UnlockOrLog(log log.Component) {
	if err := dc.Unlock(); err != nil {
		log.Warnf("Error releasing device lock %s: %s", dc.device.DeviceID(), err)
	}
}

// GetTags returns standard tags for this device.
func (dc *DeviceContext) GetTags() []string {
	return []string{
		"device_namespace:" + dc.device.Namespace,
		"device_ip:" + dc.device.IPAddress,
		"device_id:" + dc.device.DeviceID(),
		"config_source:cli",
		"profile:" + dc.profile.Name,
	}
}

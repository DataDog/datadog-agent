// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"time"

	"github.com/benbjohnson/clock"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	ncmconfig "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	ncmprofile "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	ncmremote "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/remote"
	ncmreport "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/report"
	ncmsender "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/sender"
	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
)

func newNetworkDeviceConfigImpl(log log.Component, store ncmstore.ConfigStore, sender sender.Sender, hostname string, profiles ncmprofile.Map, connectFn func(*ncmconfig.DeviceInstance) (ncmremote.Connection, error), clock clock.Clock) *networkDeviceConfigImpl {
	return &networkDeviceConfigImpl{
		log:      log,
		store:    store,
		sender:   sender,
		devices:  NewDeviceMap(deviceTimeout),
		hostname: hostname,
		profiles: profiles,
		connect:  connectFn,
		clock:    clock,
	}
}

// deviceTimeout is the maximum time to wait when attempting to lock a device.
// Lock contention should be extremely rare - it only happens if two processes
// try to access the same device at the same time, e.g. if a rollback triggers
// at the same time that the NCM check tries to fetch the config.
const deviceTimeout = time.Second * 30

type networkDeviceConfigImpl struct {
	log    log.Component
	store  ncmstore.ConfigStore
	sender sender.Sender

	devices *DeviceMap

	inventoryMaxInterval  time.Duration
	lastInventoryReportAt time.Time
	inventoryLock         sync.Mutex
	clock                 clock.Clock
	hostname              string
	profiles              ncmprofile.Map

	connect func(*ncmconfig.DeviceInstance) (ncmremote.Connection, error)
}

// RegisterDevice tells the component how to connect to a device.
func (n *networkDeviceConfigImpl) RegisterDevice(device *ncmconfig.DeviceInstance) error {
	var profile *ncmprofile.NCMProfile
	if device.Profile != "" {
		var ok bool
		profile, ok = n.profiles[ncmprofile.ProfileName(device.Profile)]
		if !ok {
			return fmt.Errorf("nonexistent NCM profile %q specified for device %s", device.Profile, device.DeviceID())
		}
	}
	return n.devices.RegisterDevice(context.Background(), device, profile)
}

// SetMaxReportInterval sets a maximum time to wait between sending inventory
// reports - if a check runs and doesn't find any new configs to report, but
// it's been at least this long since the last time inventory was reported, we
// will send an inventory report anyway.
func (n *networkDeviceConfigImpl) SetMaxReportInterval(interval time.Duration) {
	if n.inventoryMaxInterval != 0 && n.inventoryMaxInterval != interval {
		n.log.Warnf("Changing inventory max interval from %v to %v - all check runners are supposed to agree on this", n.inventoryMaxInterval, interval)
	}
	n.inventoryMaxInterval = interval
}

// ReportConfig runs the NCM check - it fetches the running and startup config
// and communicates them to the DD backend, along with an inventory report if
// necessary. The inventory report will be included if the device had new
// configuration, or if more than n.inventoryMaxInterval has elapsed since the
// last time inventory was reported.
func (n *networkDeviceConfigImpl) ReportConfig(ctx context.Context, deviceID string, baseSender sender.Sender) error {
	var log log.Component = NewLogWrapper(n.log, fmt.Sprintf("ncm[%s]: ", deviceID))
	log.Debug("Running config check.")
	ctx = WithLogger(ctx, log)
	dc, err := n.devices.GetAndLock(ctx, deviceID)
	if err != nil {
		return err
	}
	defer dc.UnlockOrLog(log)
	return n.reportConfig(ctx, dc, baseSender)
}

// reportConfig implements the NCM check, applied to a device context that is
// already locked.
func (n *networkDeviceConfigImpl) reportConfig(ctx context.Context, dc *DeviceContext, baseSender sender.Sender) error {
	startTime := n.clock.Now()
	log := LoggerFromContext(ctx)
	deviceID := dc.device.DeviceID()
	if dc.noMatchingProfile {
		log.Debugf("All profiles tested on past runs with no matches.")
		return fmt.Errorf("no matching NCM profile for device %s", deviceID)
	}
	device := dc.device
	sender := ncmsender.NewNCMSender(baseSender, device.Namespace, n.clock, n.hostname)

	conn, err := n.connectAndEnsureProfile(ctx, dc)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Update the remote client's device profile to access the correct commands
	conn.SetProfile(dc.profile)

	sender.SetDeviceTags(dc.GetTags())
	var nonBlockingErrors []error

	if err := sender.SendDeviceMetadata(deviceID, device.IPAddress); err != nil {
		log.Warnf("failed to send device metadata: %s", err)
		nonBlockingErrors = append(nonBlockingErrors, fmt.Errorf("failed to send device metadata: %w", err))
	}
	defer sender.Commit()

	configs, localStoreChanged, confErrs := retrieveAndStoreBothConfigs(ctx, dc, conn, n.store, sender)
	nonBlockingErrors = append(nonBlockingErrors, confErrs...)

	var inventoryEntries []ncmreport.InventoryEntry
	timeSinceInventory := startTime.Sub(n.getLastInventoryTime())
	hasStore := n.store != nil
	if !hasStore {
		log.Debugf("rollback is disabled, so no inventory will be reported.")
	} else if localStoreChanged {
		log.Debugf("local configstore has updated, so inventory will be reported.")
	} else if timeSinceInventory > n.inventoryMaxInterval {
		log.Debugf("inventory hasn't been reported in %v > %v and so will be reported.", timeSinceInventory, n.inventoryMaxInterval)
	} else {
		log.Debugf("local config store unchanged since last report %v ago (< %v).", timeSinceInventory, n.inventoryMaxInterval)
	}
	if hasStore && (localStoreChanged || timeSinceInventory > n.inventoryMaxInterval) {
		inventoryEntries, err = n.buildInventoryReport()
		if err != nil {
			log.Errorf("skipping inventory report due to error: %v", err)
		}
	}
	if len(configs)+len(inventoryEntries) > 0 {
		log.Debugf("Sending NCM payload with %d configs and %d inventory entries", len(configs), len(inventoryEntries))
		err := sender.SendNCMPayload(ncmreport.ToNCMPayload(device.Namespace, n.hostname, configs, inventoryEntries, n.clock.Now().Unix()))
		if err != nil {
			log.Warnf("Failed to send payload to backend: %v", err)
			nonBlockingErrors = append(nonBlockingErrors, fmt.Errorf("failed to send payload to backend: %w", err))
		} else if len(inventoryEntries) > 0 {
			n.setLastInventoryTime(n.clock.Now())
		}
	} else {
		log.Debugf("no new config and no need to send inventory data")
	}
	if len(nonBlockingErrors) == 0 {
		sender.SendNCMCheckMetrics(startTime, dc.lastReportTime, true)
		dc.lastReportTime = startTime
		return nil
	}
	sender.SendNCMCheckMetrics(startTime, dc.lastReportTime, false)
	return fmt.Errorf("check completed but with errors: %v", errors.Join(nonBlockingErrors...))
}

func (n *networkDeviceConfigImpl) buildInventoryReport() ([]ncmreport.InventoryEntry, error) {
	if n.store == nil {
		return nil, nil
	}
	configMeta, err := n.store.GetAllConfigMetadata()
	if err != nil {
		return nil, err
	}
	entries := make([]ncmreport.InventoryEntry, 0, len(configMeta))
	for _, m := range configMeta {
		entries = append(entries, ncmreport.InventoryEntry{
			Namespace:  m.GetNamespace(),
			ConfigID:   m.ConfigUUID,
			DeviceID:   m.DeviceID,
			ReportedAt: m.CapturedAt,
		})
	}
	return entries, nil
}

// RollbackConfig rolls back a device to a previous configuration that's
// saved locally on this agent.
func (n *networkDeviceConfigImpl) RollbackConfig(ctx context.Context, deviceID string, configVersion string, hash string) error {
	if n.store == nil {
		return errors.New("rollback is disabled")
	}
	var log log.Component = NewLogWrapper(n.log, fmt.Sprintf("ncm[%s]: ", deviceID))
	log.Infof("Rollback requested: Device %q to version %q", deviceID, configVersion)
	ctx = WithLogger(ctx, log)
	dc, err := n.devices.GetAndLock(ctx, deviceID)
	if err != nil {
		return err
	}
	defer dc.UnlockOrLog(log)

	rawConfig, metadata, err := n.store.GetConfig(configVersion)
	if err != nil {
		return err
	}
	if metadata.DeviceID != deviceID {
		return fmt.Errorf("input mismatch: config %q is not for device %q", configVersion, deviceID)
	}

	expectedHash := ncmstore.HashConfig(rawConfig)
	if expectedHash != hash {
		return fmt.Errorf("hash mismatch for config %q", configVersion)
	}

	conn, err := n.connectAndEnsureProfile(ctx, dc)
	if err != nil {
		return fmt.Errorf("%v: %w", deviceID, err)
	}
	defer conn.Close()

	err = conn.PushConfig(ctx, rawConfig)
	if err != nil {
		return fmt.Errorf("cannot push config to device %q: %w", deviceID, err)
	}

	// TODO if this fails we should still return success so that the user knows
	// the rollback itself happened.
	reportErr := n.reportConfig(ctx, dc, n.sender)
	if reportErr != nil {
		log.Errorf("Rollback succeeded, but reportConfig failed: %v", reportErr)
	}
	return nil
}

// connectAndEnsureProfile connects to dc.device and sets the profile on the connection, calling findMatchingProfile if dc.profile is not yet set.
func (n *networkDeviceConfigImpl) connectAndEnsureProfile(ctx context.Context, dc *DeviceContext) (ncmremote.Connection, error) {
	log := LoggerFromContext(ctx)
	conn, err := n.connect(dc.device)
	if err != nil {
		log.Errorf("unable to connect to device: %s", err)
		return nil, err
	}
	if dc.profile == nil {
		log.Debug("No profile specified, testing known profiles")
		prof, ok := n.findMatchingProfile(ctx, conn)
		if !ok {
			dc.noMatchingProfile = true
			_ = conn.Close()
			return nil, fmt.Errorf("no matching NCM profile for device %s", dc.device.DeviceID())
		}
		dc.profile = prof
	}
	conn.SetProfile(dc.profile)
	log.Debugf("Using profile %q", dc.profile.Name)
	return conn, nil
}

// findMatchingProfile tests each profile until one is successful.
func (n *networkDeviceConfigImpl) findMatchingProfile(ctx context.Context, conn ncmremote.Connection) (*ncmprofile.NCMProfile, bool) {
	logger := LoggerFromContext(ctx)
	logger.Debugf("Testing %d profiles", len(n.profiles))
	for profName, prof := range n.profiles {
		if prof.Commands.Verify == nil {
			continue
		}
		logger.Debugf("testing profile %s", profName)
		conn.SetProfile(prof)
		if err := conn.Verify(ctx); err != nil {
			logger.Debugf("Profile %s does not match: %s", profName, err)
			continue
		}
		logger.Infof("Profile match: %s", profName)
		return prof, true
	}
	return nil, false
}

func (n *networkDeviceConfigImpl) getLastInventoryTime() time.Time {
	n.inventoryLock.Lock()
	defer n.inventoryLock.Unlock()
	return n.lastInventoryReportAt
}

func (n *networkDeviceConfigImpl) setLastInventoryTime(now time.Time) {
	n.inventoryLock.Lock()
	defer n.inventoryLock.Unlock()
	n.lastInventoryReportAt = now
}

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
		devices:  NewMap[*DeviceContext](),
		hostname: hostname,
		profiles: profiles,
		connect:  connectFn,
		clock:    clock,
	}
}

type networkDeviceConfigImpl struct {
	log    log.Component
	store  ncmstore.ConfigStore
	sender sender.Sender

	devices *Map[*DeviceContext]

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
		profile, ok = n.profiles[device.Profile]
		if !ok {
			return fmt.Errorf("nonexistent NCM profile %q specified for device %s", device.Profile, device.DeviceID())
		}
	}
	// LoadOrStore so that if for some reason two threads try to do this at the
	// same time they'll get the same device context.
	dc, _ := n.devices.LoadOrStore(device.DeviceID(), &DeviceContext{})
	dc.Lock()
	defer dc.Unlock()
	dc.SetDevice(device, profile)
	return nil
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
func (n *networkDeviceConfigImpl) ReportConfig(deviceID string) error {
	return n.ReportConfigWithSender(deviceID, n.sender)
}

// ReportConfigWithSender runs the NCM check using the specified sender.
func (n *networkDeviceConfigImpl) ReportConfigWithSender(deviceID string, baseSender sender.Sender) error {
	var log log.Component = NewLogWrapper(n.log, fmt.Sprintf("ncm[%s]: ", deviceID))

	ctx := WithLogger(context.Background(), log)
	startTime := n.clock.Now()
	dc, ok := n.devices.Load(deviceID)
	if !ok {
		return fmt.Errorf("unknown device: %q", deviceID)
	}
	// lock the device so that if two threads try to use the same device at the
	// same time they won't collide.
	dc.Lock()
	defer dc.Unlock()
	if dc.noMatchingProfile {
		log.Debugf("All profiles tested on past runs with no matches.")
		return fmt.Errorf("no matching NCM profile for device %s", deviceID)
	}
	device := dc.device
	sender := ncmsender.NewNCMSender(baseSender, device.Namespace, n.clock, n.hostname)

	conn, err := n.connect(device)
	if err != nil {
		log.Errorf("unable to connect to device: %s", err)
		return err
	}
	defer conn.Close()

	if dc.profile == nil {
		log.Debug("No profile specified, testing known profiles")
		prof, ok := n.findMatchingProfile(ctx, conn)
		if !ok {
			dc.noMatchingProfile = true
			return fmt.Errorf("no matching NCM profile for device %s", deviceID)
		}
		dc.profile = prof
	}
	log.Debugf("Using profile %q", dc.profile.Name)

	// Update the remote client's device profile to access the correct commands
	conn.SetProfile(dc.profile)

	sender.SetDeviceTags(dc.GetTags())
	var nonBlockingErrors []error

	if err := sender.SendDeviceMetadata(deviceID, device.IPAddress); err != nil {
		log.Warnf("failed to send device metadata: %s", err)
		nonBlockingErrors = append(nonBlockingErrors, fmt.Errorf("failed to send device metadata: %w", err))
	}
	defer sender.Commit()

	configs, localStoreChanged, confErrs := retrieveAndStoreBothConfigs(ctx, dc, conn, n.store)
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

	ctx = WithLogger(ctx, log)
	dc, ok := n.devices.Load(deviceID)
	if !ok {
		return fmt.Errorf("unknown device: %q", deviceID)
	}
	// lock the device so that if two threads try to use the same device at the
	// same time they won't collide.
	dc.Lock()
	defer dc.Unlock()
	profile := dc.GetExplicitProfile()
	if profile == nil {
		return fmt.Errorf("no NCM profile configured for device %s", deviceID)
	}

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

	conn, err := n.connect(dc.device)
	if err != nil {
		return fmt.Errorf("%v: %w", deviceID, err)
	}
	defer conn.Close()
	conn.SetProfile(profile)

	err = conn.PushConfig(ctx, rawConfig)
	if err != nil {
		return fmt.Errorf("cannot push config to device %q: %w", deviceID, err)
	}

	return n.ReportConfig(deviceID)
}

// findMatchingProfile tests each profile until one is successful.
func (n *networkDeviceConfigImpl) findMatchingProfile(ctx context.Context, conn ncmremote.Connection) (*ncmprofile.NCMProfile, bool) {
	logger := LoggerFromContext(ctx)
	logger.Infof("Testing %d profiles", len(n.profiles))
	for profName, prof := range n.profiles {
		if prof.Commands.Verify == nil {
			continue
		}
		logger.Debugf("testing profile %s", profName)
		conn.SetProfile(prof)
		if err := conn.Verify(ctx); err != nil {
			logger.Infof("Profile %s does not match: %s", profName, err)
			continue
		}
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

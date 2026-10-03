// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"context"
	"errors"
	"fmt"

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
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
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

	clock    clock.Clock
	hostname string
	profiles ncmprofile.Map

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

// ReportConfig runs the NCM check - it fetches the running and startup config
// and sends them to the DD backend, along with an inventory report of the configs 
// currently held in the local store for this device.
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
	device := dc.device
	sender := ncmsender.NewNCMSender(baseSender, device.Namespace, n.clock, n.hostname)
	sender.SetDeviceTags(dc.GetTags())
	defer sender.Commit()

	if dc.noMatchingProfile {
		log.Debugf("All profiles tested on past runs with no matches.")
		sender.SendNCMCheckFailure(types.ErrNoProfile)
		return fmt.Errorf("no matching NCM profile for device %s", deviceID)
	}

	conn, connErr := n.connectAndEnsureProfile(ctx, dc)
	if connErr != nil {
		sender.SendNCMCheckFailure(connErr.Type())
		return connErr
	}
	defer conn.Close()

	// Update the remote client's device profile to access the correct commands
	conn.SetProfile(dc.profile)

	// dc.profile is now resolved, so refresh the device tags to include it.
	sender.SetDeviceTags(dc.GetTags())
	var nonBlockingErrors []error

	if err := sender.SendDeviceMetadata(deviceID, device.IPAddress); err != nil {
		log.Warnf("failed to send device metadata: %s", err)
		nonBlockingErrors = append(nonBlockingErrors, types.WrapErrorf(types.ErrMetadataSendFailed, "failed to send device metadata: %w", err))
	}

	configs, confErrs := retrieveAndStoreBothConfigs(ctx, dc, conn, n.store, sender)
	nonBlockingErrors = append(nonBlockingErrors, confErrs...)

	var inventoryEntries []ncmreport.InventoryEntry
	var inventoryReportBuilt bool
	if n.store == nil {
		log.Debugf("rollback is disabled, so no inventory will be reported.")
	} else {
		entry, err := n.buildInventoryReport(deviceID)
		if err != nil {
			log.Errorf("skipping inventory report due to error: %v", err)
		} else {
			inventoryEntries = []ncmreport.InventoryEntry{*entry}
			inventoryReportBuilt = true
		}
	}

	inventoryEmpty := inventoryReportBuilt && len(inventoryEntries[0].ConfigID) == 0
	if len(configs) > 0 || inventoryReportBuilt {
		log.Debugf("Sending NCM payload with %d configs and %d inventory entries", len(configs), len(inventoryEntries))
		err := sender.SendNCMPayload(ncmreport.ToNCMPayload(device.Namespace, n.hostname, configs, inventoryEntries, inventoryEmpty, n.clock.Now().Unix()))
		if err != nil {
			log.Warnf("Failed to send payload to backend: %v", err)
			nonBlockingErrors = append(nonBlockingErrors, types.WrapErrorf(types.ErrPayloadSendFailed, "failed to send payload to backend: %w", err))
		}
	} else {
		log.Debugf("no new config and no need to send inventory data")
	}
	if len(nonBlockingErrors) == 0 {
		sender.SendNCMCheckMetrics(startTime, dc.lastReportTime, true)
		dc.lastReportTime = startTime
		return nil
	}
	errTypes := make([]types.ErrorType, 0, len(nonBlockingErrors))
	for _, nbErr := range nonBlockingErrors {
		errTypes = append(errTypes, types.AsRollbackError(nbErr).Type())
	}
	sender.SendNCMCheckFailure(errTypes...)
	sender.SendNCMCheckMetrics(startTime, dc.lastReportTime, false)
	return fmt.Errorf("check completed but with errors: %v", errors.Join(nonBlockingErrors...))
}

// buildInventoryReport returns the inventory entry describing which configs
// are currently held in the local store for deviceID.
//
// TODO: this still does a full scan over the whole store's metadata bucket,
// since it isn't indexed by device - see the TODOs on ConfigStore about
// adding a composite/prefix key. Scoping this to a single device doesn't
// reduce that read cost by itself.
func (n *networkDeviceConfigImpl) buildInventoryReport(deviceID string) (*ncmreport.InventoryEntry, error) {
	configMeta, err := n.store.GetAllConfigMetadata()
	if err != nil {
		return nil, err
	}

	var configIDs []string
	for _, m := range configMeta {
		if m.DeviceID == deviceID {
			configIDs = append(configIDs, m.ConfigUUID)
		}
	}

	return &ncmreport.InventoryEntry{
		DeviceID: deviceID,
		ConfigID: configIDs,
	}, nil
}

// connectAndEnsureProfile connects to dc.device and sets the profile on the connection, calling findMatchingProfile if dc.profile is not yet set.
func (n *networkDeviceConfigImpl) connectAndEnsureProfile(ctx context.Context, dc *DeviceContext) (ncmremote.Connection, types.RollbackError) {
	log := LoggerFromContext(ctx)
	conn, err := n.connect(dc.device)
	if err != nil {
		log.Errorf("unable to connect to device: %s", err)
		return nil, types.WrapErrorf(types.ErrCannotConnect, "unable to connect to %s: %w", dc.device.DeviceID(), err)
	}
	if dc.profile == nil {
		log.Debug("No profile specified, testing known profiles")
		prof, ok := n.findMatchingProfile(ctx, conn)
		if !ok {
			dc.noMatchingProfile = true
			_ = conn.Close()
			return nil, types.WrapErrorf(types.ErrNoProfile, "no matching NCM profile for device %s", dc.device.DeviceID())
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

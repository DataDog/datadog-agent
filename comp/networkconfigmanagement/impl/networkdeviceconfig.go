// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package networkconfigmanagementimpl implements the networkconfigmanagement component interface
package networkconfigmanagementimpl

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"

	"time"

	"github.com/benbjohnson/clock"

	demultiplexer "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/def"
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	ncmconfig "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	ncmprofile "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	ncmremote "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/remote"
	ncmreport "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/report"
	ncmsender "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/sender"
	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
	ncmtypes "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckName is the name of the check
const CheckName = "network_config_management"

// Provides defines the output of the networkconfigmanagement component
type Provides struct {
	compdef.Out

	Comp              option.Option[networkconfigmanagement.Component]
	GetConfigEndpoint api.EndpointProvider `group:"agent_endpoint"`
}

// Requires defines the dependencies for the networkconfigmanagement component
type Requires struct {
	compdef.In
	Lifecycle       compdef.Lifecycle
	Config          config.Component
	Logger          log.Component
	Demux           demultiplexer.Component
	HostnameService hostname.Component
}

// NewComponent creates a new networkconfigmanagement component
func NewComponent(reqs Requires) (Provides, error) {
	var compOpt option.Option[networkconfigmanagement.Component]
	var store ncmstore.ConfigStore
	comp, err := newComponent(reqs)
	if err != nil {
		reqs.Logger.Errorf("NCM service could not be initialized: %s", err)
		compOpt = option.None[networkconfigmanagement.Component]()
	} else {
		compOpt = option.New(networkconfigmanagement.Component(comp))
		store = comp.store
	}
	var getConfigHandler http.HandlerFunc
	if store == nil {
		getConfigHandler = func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"error": "ncm not enabled for agent"}`, http.StatusBadRequest)
		}
	} else {
		getConfigHandler = newConfigEndpointHandler(store)
	}
	return Provides{
		Comp:              compOpt,
		GetConfigEndpoint: api.NewAgentEndpointProvider(getConfigHandler, "/ncm/config", "GET").Provider,
	}, nil

}

func newComponent(reqs Requires) (*networkDeviceConfigImpl, error) {
	rollbackEnabled := reqs.Config.GetBool("network_devices.config_management.rollback.enabled")
	hostname, err := reqs.HostnameService.Get(context.Background())
	if err != nil {
		return nil, err
	}
	sender, err := reqs.Demux.GetSender(CheckName)
	if err != nil {
		return nil, err
	}
	profiles, err := ncmprofile.GetProfileMap()
	if err != nil {
		return nil, err
	}
	var store ncmstore.ConfigStore
	if rollbackEnabled {
		runPath := reqs.Config.GetString("run_path")
		dbPath := filepath.Join(runPath, "ncm_config.db")
		reqs.Logger.Debugf("config rollback enabled; local db is %v", dbPath)
		store, err = ncmstore.Open(dbPath)
		if err != nil {
			return nil, err
		}
		reqs.Lifecycle.Append(compdef.Hook{OnStop: store.Close})
	}

	impl := newNetworkDeviceConfigImpl(
		reqs.Logger,
		store,
		sender,
		hostname,
		profiles,
		ncmremote.ConnectOverSSH,
		clock.New(),
	)
	return impl, nil
}

func newNetworkDeviceConfigImpl(log log.Component, store ncmstore.ConfigStore, sender sender.Sender, hostname string, profiles ncmprofile.Map, connectFn func(*ncmconfig.DeviceInstance) (ncmremote.Connection, error), clock clock.Clock) *networkDeviceConfigImpl {
	return &networkDeviceConfigImpl{
		log:              log,
		store:            store,
		sender:           sender,
		deviceConfigs:    NewMap[*ncmconfig.DeviceInstance](),
		lastReportTimes:  NewMap[time.Time](),
		detectedProfiles: NewMap[*ncmprofile.NCMProfile](),
		hostname:         hostname,
		profiles:         profiles,
		connect:          connectFn,
		clock:            clock,
	}
}

type networkDeviceConfigImpl struct {
	log    log.Component
	store  ncmstore.ConfigStore
	sender sender.Sender

	// deviceConfigs maps deviceIDs to their data. It is populated by check
	// instances calling RegisterDevice
	deviceConfigs    *Map[*ncmconfig.DeviceInstance]
	detectedProfiles *Map[*ncmprofile.NCMProfile]
	// lastReportTimes maps a deviceID to the timestamp of the last time a
	// config report was requested for that deviceID.
	lastReportTimes       *Map[time.Time]
	inventoryMaxInterval  time.Duration
	lastInventoryReportAt time.Time
	inventoryLock         sync.Mutex
	clock                 clock.Clock
	hostname              string
	profiles              ncmprofile.Map

	connect func(*ncmconfig.DeviceInstance) (ncmremote.Connection, error)
}

// RegisterDevice tells the component how to connect to a device.
func (n *networkDeviceConfigImpl) RegisterDevice(config *ncmconfig.DeviceInstance) error {
	n.deviceConfigs.Store(config.DeviceID(), config)
	return nil
}

// ReportConfig runs the NCM check - it fetches the running and startup config
// and communicates them to the DD backend, along with an inventory report if
// necessary. The inventory report will be included if the device had new
// configuration, or if more than n.inventoryMaxInterval has elapsed since the
// last time inventory was reported.
func (n *networkDeviceConfigImpl) ReportConfig(deviceID string) error {
	return n.ReportConfigWithSender(deviceID, n.sender)
}

func (n *networkDeviceConfigImpl) retrieveAndStoreConfig(
	ctx context.Context,
	device *ncmconfig.DeviceInstance,
	conn ncmremote.Connection,
	profile *ncmprofile.NCMProfile,
	confType ncmtypes.ConfigType,
	deviceTags []string) (*ncmreport.NetworkDeviceConfig, bool, error) {
	getConfig := conn.RetrieveRunningConfig
	mode := "running"
	if confType == ncmtypes.STARTUP {
		getConfig = conn.RetrieveStartupConfig
		mode = "startup"
	}
	rawConfig, checkErr := getConfig(ctx)
	if checkErr != nil {
		return nil, false, checkErr
	}

	configStore := n.store
	deviceID := device.DeviceID()
	redactedConfig, metadata, checkErr := profile.ProcessConfig(rawConfig)
	if checkErr != nil {
		return nil, false, fmt.Errorf("unable to process rules for %s config for device %s: %s", mode, deviceID, checkErr)
	}
	configID, configHash, stored := "", "", false
	if configStore != nil {
		var err error
		configID, configHash, stored, err = configStore.StoreConfig(deviceID, confType, string(rawConfig))
		if err != nil {
			n.log.Warnf("ncm[%s]: unable to store %s config: %v", deviceID, mode, err)
		}
	}
	conf := ncmreport.ToNetworkDeviceConfig(deviceID, device.IPAddress, confType, metadata, deviceTags, redactedConfig, configID, configHash)
	return &conf, stored, nil
}

// getSavedProfileForDevice returns the profile for this device, if known. If
// explicitOnly is true, then it will only return a profile if the device is
// explicitly configured with one; otherwise, it will return a previously
// detected profile if we have one. A return value of (nil, nil), i.e. no
// profile but no error, means that we don't yet have a profile for this device
// but we haven't tested to see if any of our known profiles work.
func (n *networkDeviceConfigImpl) getSavedProfileForDevice(device *ncmconfig.DeviceInstance, explicitOnly bool) (prof *ncmprofile.NCMProfile, err error) {
	deviceID := device.DeviceID()
	if device.Profile != "" {
		knownProfile, ok := n.profiles[device.Profile]
		if !ok {
			// device is explicitly configured with a profile we don't know
			return nil, fmt.Errorf("nonexistent NCM profile %q specified for device %s", device.Profile, deviceID)
		}
		return knownProfile, nil
	}
	if explicitOnly {
		return nil, fmt.Errorf("no profile configured for device %s", deviceID)
	}
	profile, ok := n.detectedProfiles.Load(deviceID)
	if ok {
		if profile == nil {
			// explicit nil indicates that we've already tried to detect a profile and failed.
			return nil, fmt.Errorf("no matching NCM profile for device %s", deviceID)
		}
		return profile, nil
	}
	// nil, nil means we don't have a profile for this device but we might find one if we try to detect it.
	return nil, nil
}

// saveDetectedProfileForDevice records the profile we've detected for a device.
func (n *networkDeviceConfigImpl) saveDetectedProfileForDevice(device *ncmconfig.DeviceInstance, prof *ncmprofile.NCMProfile) {
	n.detectedProfiles.Store(device.DeviceID(), prof)
}

// saveNoProfileForDevice records that we've tried all our profiles for this
// device and none of them worked.
func (n *networkDeviceConfigImpl) saveNoProfileForDevice(device *ncmconfig.DeviceInstance) {
	n.detectedProfiles.Store(device.DeviceID(), nil)
}

func (n *networkDeviceConfigImpl) ReportConfigWithSender(deviceID string, baseSender sender.Sender) error {
	var configs []ncmreport.NetworkDeviceConfig
	ctx := context.Background()
	startTime := n.clock.Now()
	debugf := func(format string, params ...interface{}) {
		format = fmt.Sprintf("ncm[%s]: %s", deviceID, format)
		n.log.Debugf(format, params...)
	}
	device, ok := n.deviceConfigs.Load(deviceID)
	if !ok {
		return fmt.Errorf("unknown device: %q", deviceID)
	}
	profile, err := n.getSavedProfileForDevice(device, false)
	if err != nil {
		return err
	}

	sender := ncmsender.NewNCMSender(baseSender, device.Namespace, n.clock, n.hostname)

	conn, err := n.connect(device)
	if err != nil {
		n.log.Errorf("ncm[%s]: unable to connect to remote device: %s", deviceID, err)
		return err
	}
	defer conn.Close()

	if profile == nil {
		debugf("No profile specified, testing known profiles")
		prof, ok := n.findMatchingProfile(device, conn)
		if !ok {
			// store explicit nil to indicate that we shouldn't try to search for this again.
			n.saveNoProfileForDevice(device)
			return fmt.Errorf("no matching NCM profile for device %s", deviceID)
		}
		n.saveDetectedProfileForDevice(device, prof)
		profile = prof
	}
	debugf("Using profile %q", profile.Name)
	// Update the remote client's device profile to access the correct commands
	conn.SetProfile(profile)

	deviceTags := n.getDeviceTags(device)
	sender.SetDeviceTags(deviceTags)
	var nonBlockingErrors []error

	if err := sender.SendDeviceMetadata(deviceID, device.IPAddress); err != nil {
		n.log.Warnf("ncm[%s]: failed to send device metadata: %s", deviceID, err)
		nonBlockingErrors = append(nonBlockingErrors, fmt.Errorf("failed to send device metadata: %w", err))
	}
	defer sender.Commit()
	configStore := n.store

	var localStoreChanged bool

	if runningConfig, stored, err := n.retrieveAndStoreConfig(ctx, device, conn, profile, ncmtypes.RUNNING, deviceTags); err != nil {
		n.log.Warnf("ncm[%s]: unable to retrieve running config, will not send: %v", deviceID, err)
		nonBlockingErrors = append(nonBlockingErrors, fmt.Errorf("failed to retrieve running config: %w", err))
	} else {
		localStoreChanged = localStoreChanged || stored
		configs = append(configs, *runningConfig)
	}

	if startupConfig, stored, err := n.retrieveAndStoreConfig(ctx, device, conn, profile, ncmtypes.STARTUP, deviceTags); err != nil {
		n.log.Warnf("ncm[%s]: unable to retrieve startup config, will not send: %v", deviceID, err)
		nonBlockingErrors = append(nonBlockingErrors, fmt.Errorf("failed to retrieve startup config: %w", err))
	} else {
		localStoreChanged = localStoreChanged || stored
		configs = append(configs, *startupConfig)
	}

	var inventoryEntries []ncmreport.InventoryEntry
	timeSinceInventory := startTime.Sub(n.getLastInventoryTime())
	if configStore == nil {
		debugf("rollback is disabled, so no inventory will be reported.")
	} else if localStoreChanged {
		debugf("local configstore has updated, so inventory will be reported.")
	} else if timeSinceInventory > n.inventoryMaxInterval {
		debugf("inventory hasn't been reported in %v > %v and so will be reported.", timeSinceInventory, n.inventoryMaxInterval)
	} else {
		debugf("local config store unchanged, so no inventory will be reported.", timeSinceInventory)
	}
	if configStore != nil && (localStoreChanged || timeSinceInventory > n.inventoryMaxInterval) {
		inventoryEntries = n.buildInventoryReport()
	}
	if len(configs)+len(inventoryEntries) > 0 {
		debugf("Sending NCM payload with %d configs and %d inventory entries", len(configs), len(inventoryEntries))
		err := sender.SendNCMPayload(ncmreport.ToNCMPayload(device.Namespace, n.hostname, configs, inventoryEntries, n.clock.Now().Unix()))
		if err != nil {
			n.log.Warnf("ncm[%v]: Failed to send payload to backend: %v", deviceID, err)
			nonBlockingErrors = append(nonBlockingErrors, fmt.Errorf("failed to send payload to backend: %w", err))
		} else if len(inventoryEntries) > 0 {
			n.setLastInventoryTime(n.clock.Now())
		}
	} else {
		debugf("no new config and no need to send inventory data")
	}
	if len(nonBlockingErrors) == 0 {
		lastTime, _ := n.lastReportTimes.Swap(deviceID, startTime)
		sender.SendNCMCheckMetrics(startTime, lastTime, true)
		return nil
	}
	lastTime, _ := n.lastReportTimes.Load(deviceID)
	sender.SendNCMCheckMetrics(startTime, lastTime, false)
	return fmt.Errorf("check completed but with errors: %v", errors.Join(nonBlockingErrors...))
}

func (n *networkDeviceConfigImpl) getDeviceTags(device *ncmconfig.DeviceInstance) []string {
	return []string{
		"device_namespace:" + device.Namespace,
		"device_ip:" + device.IPAddress,
		"device_id:" + device.DeviceID(),
		// TODO: device_hostname - may need to be extracted from config / output to be retrieved in NCM core check
		"config_source:cli",
		"profile:" + device.Profile,
	}
}

func (n *networkDeviceConfigImpl) buildInventoryReport() []ncmreport.InventoryEntry {
	if n.store == nil {
		return nil
	}
	configMeta, err := n.store.GetAllConfigMetadata()
	if err != nil {
		n.log.Errorf("error retrieving config metadata for inventory report: %v, skipping", err)
		return nil
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
	return entries
}

// RollbackConfig rolls back a device to a previous configuration that's
// saved locally on this agent.
func (n *networkDeviceConfigImpl) RollbackConfig(_ string, _ string, _ string) error {
	return errors.New("not implemented")
}

// SetMaxReportInterval sets the minimum time
func (n *networkDeviceConfigImpl) SetMaxReportInterval(interval time.Duration) {
	if n.inventoryMaxInterval != 0 && n.inventoryMaxInterval != interval {
		n.log.Warnf("Changing inventory max interval from %v to %v - all check runners are supposed to agree on this", n.inventoryMaxInterval, interval)
	}
	n.inventoryMaxInterval = interval
}

// findMatchingProfile tests each profile until one is successful.
// TODO use GetVersion instead of fetching the entire config.
func (n *networkDeviceConfigImpl) findMatchingProfile(device *ncmconfig.DeviceInstance, conn ncmremote.Connection) (*ncmprofile.NCMProfile, bool) {
	for profName, prof := range n.profiles {
		n.log.Debugf("ncm[%s] testing profile %s", device.DeviceID(), profName)
		conn.SetProfile(prof)
		_, err := conn.RetrieveRunningConfig(context.Background())
		if err != nil {
			n.log.Infof("profile %s does not match remote device %s: %s", profName, device.IPAddress, err)
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

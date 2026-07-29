// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"context"
	"fmt"

	ncmremote "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/remote"
	ncmreport "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/report"
	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
	ncmtypes "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// retrieveAndStoreConfig requests a config from the given connection, runs the
// metadata & redaction processing on it, and stores it in the configStore if
// applicable. It returns the redacted config + metadata as an
// [ncmreport.NetworkDeviceConfig], as well as a bool indicating whether the
// local store was changed as a result of this (this will be false if the local
// store is nil, or if the store detects that this config is identical to one
// already present).
func retrieveAndStoreConfig(ctx context.Context, dc *DeviceContext, conn ncmremote.Connection, configStore ncmstore.ConfigStore, confType ncmtypes.ConfigType) (*ncmreport.NetworkDeviceConfig, bool, error) {
	logger := LoggerFromContext(ctx)
	getConfig := conn.RetrieveRunningConfig
	mode := "running"
	if confType == ncmtypes.STARTUP {
		getConfig = conn.RetrieveStartupConfig
		mode = "startup"
	}
	rawConfig, err := getConfig(ctx)
	if err != nil {
		return nil, false, err
	}

	deviceID := dc.device.DeviceID()
	result, err := dc.profile.ProcessConfig(rawConfig)
	if err != nil {
		return nil, false, fmt.Errorf("unable to process rules for %s config for device %s: %s", mode, deviceID, err)
	}
	configID, configHash, stored := "", "", false
	if configStore != nil {
		var err error
		configID, configHash, stored, err = configStore.StoreConfig(deviceID, confType, string(result.Raw))
		if err != nil {
			logger.Warnf("unable to store %s config: %v", mode, err)
		}
	}
	conf := ncmreport.ToNetworkDeviceConfig(deviceID, dc.device.IPAddress, confType, string(dc.profile.Name), result.Metadata, dc.GetTags(), result.Redacted, configID, configHash)
	return &conf, stored, nil
}

// retrieveAndStoreBothConfigs runs retrieveAndStoreConfig for both running and
// startup config. It returns the configs that were successfully fetched, a
// boolean indicating if anything new was written to the configstore, and a list
// of errors that happened during processing. Note that if either the startup or
// the running config fails, the other will still be attempted; thus, configs
// may be nonempty and storeChanged may be true even if errors is also nonempty.
func retrieveAndStoreBothConfigs(ctx context.Context, dc *DeviceContext, conn ncmremote.Connection, store ncmstore.ConfigStore) (configs []ncmreport.NetworkDeviceConfig, storeChanged bool, errors []error) {
	logger := LoggerFromContext(ctx)
	if runningConfig, stored, err := retrieveAndStoreConfig(ctx, dc, conn, store, ncmtypes.RUNNING); err != nil {
		logger.Warnf("unable to retrieve running config, will not send: %v", err)
		errors = append(errors, fmt.Errorf("failed to retrieve running config: %w", err))
	} else {
		storeChanged = storeChanged || stored
		configs = append(configs, *runningConfig)
	}

	if startupConfig, stored, err := retrieveAndStoreConfig(ctx, dc, conn, store, ncmtypes.STARTUP); err != nil {
		logger.Warnf("unable to retrieve startup config, will not send: %v", err)
		errors = append(errors, fmt.Errorf("failed to retrieve startup config: %w", err))
	} else {
		storeChanged = storeChanged || stored
		configs = append(configs, *startupConfig)
	}
	return configs, storeChanged, errors
}

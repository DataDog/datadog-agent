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
	ncmsender "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/sender"
	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
	ncmtypes "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// retrieveAndStoreConfig requests a config from the given connection, runs the
// metadata & redaction processing on it, and stores it in the configStore if
// applicable. It returns the redacted config + metadata as an
// [ncmreport.NetworkDeviceConfig].
func retrieveAndStoreConfig(ctx context.Context, dc *DeviceContext, conn ncmremote.Connection, configStore ncmstore.ConfigStore, confType ncmtypes.ConfigType, ncmSender *ncmsender.NCMSender) (*ncmreport.NetworkDeviceConfig, error) {
	logger := LoggerFromContext(ctx)
	getConfig := conn.RetrieveRunningConfig
	mode := "running"
	if confType == ncmtypes.STARTUP {
		getConfig = conn.RetrieveStartupConfig
		mode = "startup"
	}
	rawConfig, err := getConfig(ctx)
	if err != nil {
		return nil, err
	}

	deviceID := dc.device.DeviceID()
	result, err := dc.profile.ProcessConfig([]byte(rawConfig.Output))
	if err != nil {
		return nil, fmt.Errorf("unable to process rules for %s config for device %s: %s", mode, deviceID, err)
	}
	configID, configHash := "", ""
	if configStore != nil {
		var err error
		var stored bool
		configID, configHash, stored, err = configStore.StoreConfig(deviceID, confType, string(result.Raw))
		if err != nil {
			logger.Warnf("unable to store %s config: %v", mode, err)
		}
		if stored {
			evicted, err := configStore.EvictConfigs()
			if err != nil {
				logger.Warnf("unable to evict configs: %v", err)
			}
			if ncmSender != nil {
				ncmSender.SendStoreEvictionMetrics(len(evicted), err)
			}
		}
	}
	conf := ncmreport.ToNetworkDeviceConfig(deviceID, dc.device.IPAddress, confType, string(dc.profile.Name), result.Metadata, dc.GetTags(), result.Redacted, configID, configHash)
	return &conf, nil
}

// retrieveAndStoreBothConfigs runs retrieveAndStoreConfig for both running and
// startup config. It returns the configs that were successfully fetched and a
// list of errors that happened during processing. Note that if either the
// startup or the running config fails, the other will still be attempted;
// thus, configs may be nonempty even if errors is also nonempty.
func retrieveAndStoreBothConfigs(ctx context.Context, dc *DeviceContext, conn ncmremote.Connection, store ncmstore.ConfigStore, ncmSender *ncmsender.NCMSender) (configs []ncmreport.NetworkDeviceConfig, errors []error) {
	logger := LoggerFromContext(ctx)
	if runningConfig, err := retrieveAndStoreConfig(ctx, dc, conn, store, ncmtypes.RUNNING, ncmSender); err != nil {
		logger.Warnf("unable to retrieve running config, will not send: %v", err)
		errors = append(errors, ncmtypes.WrapErrorf(ncmtypes.ErrConfigRetrievalFailed, "failed to retrieve running config: %w", err))
	} else {
		configs = append(configs, *runningConfig)
	}

	if startupConfig, err := retrieveAndStoreConfig(ctx, dc, conn, store, ncmtypes.STARTUP, ncmSender); err != nil {
		logger.Warnf("unable to retrieve startup config, will not send: %v", err)
		errors = append(errors, ncmtypes.WrapErrorf(ncmtypes.ErrConfigRetrievalFailed, "failed to retrieve startup config: %w", err))
	} else {
		configs = append(configs, *startupConfig)
	}
	return configs, errors
}

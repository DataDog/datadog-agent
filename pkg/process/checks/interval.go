// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	//nolint:revive // TODO(PROC) Fix revive linter
	ProcessCheckDefaultInterval = 10 * time.Second
	//nolint:revive // TODO(PROC) Fix revive linter
	RTProcessCheckDefaultInterval = 2 * time.Second
	//nolint:revive // TODO(PROC) Fix revive linter
	ContainerCheckDefaultInterval = 10 * time.Second
	//nolint:revive // TODO(PROC) Fix revive linter
	RTContainerCheckDefaultInterval = 2 * time.Second
	//nolint:revive // TODO(PROC) Fix revive linter
	ConnectionsCheckDefaultInterval = 30 * time.Second
	//nolint:revive // TODO(PROC) Fix revive linter
	ProcessDiscoveryCheckDefaultInterval = 4 * time.Hour

	discoveryMinInterval = 10 * time.Minute
)

var (
	defaultIntervals = map[string]time.Duration{
		ProcessCheckName:     ProcessCheckDefaultInterval,
		RTProcessCheckName:   RTProcessCheckDefaultInterval,
		ContainerCheckName:   ContainerCheckDefaultInterval,
		RTContainerCheckName: RTContainerCheckDefaultInterval,
		ConnectionsCheckName: ConnectionsCheckDefaultInterval,
		DiscoveryCheckName:   ProcessDiscoveryCheckDefaultInterval,
	}

	configKeys = map[string]string{
		ProcessCheckName:     "process_config.intervals.process",
		RTProcessCheckName:   "process_config.intervals.process_realtime",
		ContainerCheckName:   "process_config.intervals.container",
		RTContainerCheckName: "process_config.intervals.container_realtime",
		ConnectionsCheckName: "process_config.intervals.connections",
	}
)

// GetDefaultInterval returns the default check interval value
func GetDefaultInterval(checkName string) time.Duration {
	return defaultIntervals[checkName]
}

// GetInterval returns the configured check interval value
func GetInterval(cfg pkgconfigmodel.Reader, checkName string) time.Duration {
	switch checkName {
	case DiscoveryCheckName:
		// We don't need to check if the key exists since we already bound it to a default in InitConfig.
		// We use a minimum of 10 minutes for this value.
		discoveryInterval := cfg.GetDuration("process_config.process_discovery.interval")
		if discoveryInterval < discoveryMinInterval {
			discoveryInterval = discoveryMinInterval
			_ = log.Warnf("Invalid interval for process discovery (< %s) using minimum value of %[1]s", discoveryMinInterval.String())
		}
		return discoveryInterval

	default:
		defaultInterval := defaultIntervals[checkName]
		configKey, ok := configKeys[checkName]
		if !ok {
			return defaultInterval
		}

		if seconds := cfg.GetInt(configKey); seconds != 0 {
			log.Infof("Overriding %s check interval to %ds", configKey, seconds)
			return time.Duration(seconds) * time.Second
		}
		return defaultInterval
	}
}

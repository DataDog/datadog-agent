// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	ProcessCheckDefaultInterval          = 10 * time.Second
	RTProcessCheckDefaultInterval        = 2 * time.Second
	ContainerCheckDefaultInterval        = 10 * time.Second
	RTContainerCheckDefaultInterval      = 2 * time.Second
	ConnectionsCheckDefaultInterval      = 30 * time.Second
	PodCheckDefaultInterval              = 10 * time.Second
	ProcessDiscoveryCheckDefaultInterval = 4 * time.Hour

	discoveryMinInterval = 10 * time.Minute

	configIntervals = configPrefix + "intervals."

	// The interval, in seconds, at which we will run each check. If you want consistent
	// behavior between real-time you may set the Container/ProcessRT intervals to 10.
	// Defaults to 10s for normal checks and 2s for others.
	configProcessInterval     = configIntervals + "process"
	configRTProcessInterval   = configIntervals + "process_realtime"
	configContainerInterval   = configIntervals + "container"
	configRTContainerInterval = configIntervals + "container_realtime"
	configConnectionsInterval = configIntervals + "connections"
)

var (
	defaultIntervals = map[string]time.Duration{
		ProcessCheckName:       ProcessCheckDefaultInterval,
		RTProcessCheckName:     RTProcessCheckDefaultInterval,
		ContainerCheckName:     ContainerCheckDefaultInterval,
		RTContainerCheckName:   RTContainerCheckDefaultInterval,
		ConnectionsCheckName:   ConnectionsCheckDefaultInterval,
		PodCheckName:           PodCheckDefaultInterval,
		DiscoveryCheckName:     ProcessDiscoveryCheckDefaultInterval,
		ProcessEventsCheckName: config.DefaultProcessEventsCheckInterval,
	}

	configKeys = map[string]string{
		ProcessCheckName:     configProcessInterval,
		RTProcessCheckName:   configRTProcessInterval,
		ContainerCheckName:   configContainerInterval,
		RTContainerCheckName: configRTContainerInterval,
		ConnectionsCheckName: configConnectionsInterval,
	}
)

// GetDefaultInterval returns the default check interval value
func GetDefaultInterval(checkName string) time.Duration {
	return defaultIntervals[checkName]
}

// GetInterval returns the configured check interval value
func GetInterval(cfg config.ConfigReader, checkName string) time.Duration {
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

	case ProcessEventsCheckName:
		eventsInterval := cfg.GetDuration("process_config.event_collection.interval")
		if eventsInterval < config.DefaultProcessEventsMinCheckInterval {
			eventsInterval = config.DefaultProcessEventsCheckInterval
			_ = log.Warnf("Invalid interval for process_events check (< %s) using default value of %s",
				config.DefaultProcessEventsMinCheckInterval.String(), config.DefaultProcessEventsCheckInterval.String())
		}
		return eventsInterval

	default:
		defaultInterval := defaultIntervals[checkName]
		configKey, ok := configKeys[checkName]
		if !ok || !cfg.IsSet(configKey) {
			return defaultInterval
		}

		if seconds := cfg.GetInt(configKey); seconds != 0 {
			log.Infof("Overriding %s check interval to %ds", configKey, seconds)
			return time.Duration(seconds) * time.Second
		}
		return defaultInterval
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config holds config related files
package config

import (
	"strings"

	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	rsNS = "runtime_security_config"
	evNS = "event_monitoring_config"
)

// Config defines the config
type Config struct {
	// SocketPath is the path to the socket that is used to communicate with the security agent and process agent
	SocketPath string

	// EventServerBurst defines the maximum burst of events that can be sent over the grpc server
	EventServerBurst int

	// ProcessConsumerEnabled defines if the process-agent wants to receive kernel events
	ProcessConsumerEnabled bool

	// NetworkConsumerEnabled defines if the network tracer system-probe module wants to receive kernel events
	NetworkConsumerEnabled bool
}

// NewConfig creates a config for the event monitoring module
func NewConfig(spConfig *sysconfigtypes.Config) *Config {
	return &Config{
		// event server
		SocketPath:       coreconfig.SystemProbe.GetString(join(evNS, "socket")),
		EventServerBurst: coreconfig.SystemProbe.GetInt(join(evNS, "event_server.burst")),

		// consumers
		ProcessConsumerEnabled: getBool("process.enabled"),
		NetworkConsumerEnabled: getBool("network_process.enabled") && spConfig.ModuleIsEnabled(config.NetworkTracerModule),
	}
}

func join(pieces ...string) string {
	return strings.Join(pieces, ".")
}

func getAllKeys(key string) (string, string) {
	deprecatedKey := strings.Join([]string{rsNS, key}, ".")
	newKey := strings.Join([]string{evNS, key}, ".")
	return deprecatedKey, newKey
}

func getBool(key string) bool {
	deprecatedKey, newKey := getAllKeys(key)
	if coreconfig.SystemProbe.IsSet(deprecatedKey) {
		log.Warnf("%s has been deprecated: please set %s instead", deprecatedKey, newKey)
		return coreconfig.SystemProbe.GetBool(deprecatedKey)
	}
	return coreconfig.SystemProbe.GetBool(newKey)
}

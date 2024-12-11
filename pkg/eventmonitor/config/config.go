// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config holds config related files
package config

import (
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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

	EnvVarsResolutionEnabled bool
}

// NewConfig creates a config for the event monitoring module
func NewConfig() *Config {
	return &Config{
		// event server
		SocketPath:       pkgconfigsetup.SystemProbe().GetString(sysconfig.FullKeyPath(evNS, "socket")),
		EventServerBurst: pkgconfigsetup.SystemProbe().GetInt(sysconfig.FullKeyPath(evNS, "event_server.burst")),

		// consumers
		ProcessConsumerEnabled: getBool("process.enabled"),

		// options
		EnvVarsResolutionEnabled: pkgconfigsetup.SystemProbe().GetBool(sysconfig.FullKeyPath(evNS, "env_vars_resolution.enabled")),
	}
}

func getAllKeys(key string) (string, string) {
	deprecatedKey := sysconfig.FullKeyPath(rsNS, key)
	newKey := sysconfig.FullKeyPath(evNS, key)
	return deprecatedKey, newKey
}

func getBool(key string) bool {
	deprecatedKey, newKey := getAllKeys(key)
	if pkgconfigsetup.SystemProbe().IsSet(deprecatedKey) {
		log.Warnf("%s has been deprecated: please set %s instead", deprecatedKey, newKey)
		return pkgconfigsetup.SystemProbe().GetBool(deprecatedKey)
	}
	return pkgconfigsetup.SystemProbe().GetBool(newKey)
}

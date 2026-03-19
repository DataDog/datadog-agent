// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package opamp provides a DDOT-specific OpAmp extension that extends the
// upstream opampextension with additional capabilities:
//   - ReportsHeartbeat: agent sends periodic keep-alive messages whose interval
//     can be adjusted by the server.
//   - AcceptsOpAMPConnectionSettings: agent accepts TLS certificate rotation and
//     heartbeat-interval changes pushed by the server.
//   - ReportsConnectionSettingsStatus: agent reports whether the pushed settings
//     were applied successfully.
package opamp

import (
	"fmt"
	"time"

	opampextension "github.com/open-telemetry/opentelemetry-collector-contrib/extension/opampextension"
	"go.opentelemetry.io/collector/component"
)

// Type is the component type, kept identical to the upstream extension so that
// existing "extensions: {opamp: ...}" configs work without change.
var Type = component.MustNewType("opamp")

// Config mirrors the upstream opampextension.Config but replaces the
// Capabilities struct with our extended version.
type Config struct {
	Server           *opampextension.OpAMPServer     `mapstructure:"server"`
	InstanceUID      string                          `mapstructure:"instance_uid"`
	AgentDescription opampextension.AgentDescription `mapstructure:"agent_description"`
	Capabilities     Capabilities                    `mapstructure:"capabilities"`
	PPIDPollInterval time.Duration                   `mapstructure:"ppid_poll_interval"`
	PPID             int                             `mapstructure:"ppid"`
}

// Capabilities extends the upstream capability set with features that require
// explicit support from the agent but are handled at the opamp-go client library
// level once declared.
type Capabilities struct {
	// Upstream capabilities (same defaults as opampextension).
	ReportsEffectiveConfig     bool `mapstructure:"reports_effective_config"`
	ReportsHealth              bool `mapstructure:"reports_health"`
	ReportsAvailableComponents bool `mapstructure:"reports_available_components"`
	AcceptsRestartCommand      bool `mapstructure:"accepts_restart_command"`

	// Extended DDOT capabilities.

	// AcceptsOpAMPConnectionSettings allows the server to push TLS certificates
	// and heartbeat-interval updates. The opamp-go client applies heartbeat-interval
	// changes and stores TLS certs for the next reconnection automatically.
	AcceptsOpAMPConnectionSettings bool `mapstructure:"accepts_opamp_connection_settings"`

	// ReportsHeartbeat enables periodic AgentToServer keep-alive messages whose
	// interval is set by the server via AcceptsOpAMPConnectionSettings.
	// The opamp-go WebSocket client handles the heartbeat ticker internally.
	ReportsHeartbeat bool `mapstructure:"reports_heartbeat"`

	// ReportsConnectionSettingsStatus enables the agent to report whether a
	// pushed OpAMPConnectionSettings was applied (APPLIED) or rejected (FAILED).
	// Required for T028 (invalid TLS cert rejection).
	ReportsConnectionSettingsStatus bool `mapstructure:"reports_connection_settings_status"`

	// ReportsOwnMetrics enables forwarding the agent's internal metrics to an
	// OTLP endpoint specified by the OpAMP server via OwnMetrics ConnectionSettings.
	ReportsOwnMetrics bool `mapstructure:"reports_own_metrics"`
}

func (caps Capabilities) toAgentCapabilities() uint64 {
	const (
		reportsStatus                   uint64 = 1
		reportsEffectiveConfig          uint64 = 4
		reportsHealth                   uint64 = 2048
		reportsAvailableComponents      uint64 = 16384
		acceptsOpAMPConnectionSettings  uint64 = 256
		reportsHeartbeat                uint64 = 8192
		reportsConnectionSettingsStatus uint64 = 32768
		reportsOwnMetrics               uint64 = 64
	)

	result := reportsStatus
	if caps.ReportsEffectiveConfig {
		result |= reportsEffectiveConfig
	}
	if caps.ReportsHealth {
		result |= reportsHealth
	}
	if caps.ReportsAvailableComponents {
		result |= reportsAvailableComponents
	}
	if caps.AcceptsOpAMPConnectionSettings {
		result |= acceptsOpAMPConnectionSettings
	}
	if caps.ReportsHeartbeat {
		result |= reportsHeartbeat
	}
	if caps.ReportsConnectionSettingsStatus {
		result |= reportsConnectionSettingsStatus
	}
	if caps.ReportsOwnMetrics {
		result |= reportsOwnMetrics
	}
	return result
}

// Validate checks that the config is well-formed.
func (c *Config) Validate() error {
	if c.Server == nil {
		return nil
	}
	switch {
	case c.Server.WS == nil && c.Server.HTTP == nil:
		return fmt.Errorf("opamp server must have at least ws or http set")
	case c.Server.WS != nil && c.Server.HTTP != nil:
		return fmt.Errorf("opamp server must have only ws or http set")
	}
	return nil
}

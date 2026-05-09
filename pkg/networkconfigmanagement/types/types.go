// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package types provides common types and constants used throughout NCM.
package types

// ConfigType defines the type of network device configuration
type ConfigType string

const (
	// RUNNING represents the running configuration of a network device (the current active configuration)
	RUNNING ConfigType = "running"
	// STARTUP represents the startup configuration of a network device (the configuration that is loaded on boot)
	STARTUP ConfigType = "startup"
)

// ConfigSource represents where the config was retrieved from (in the case of the integration, it's always via CLI commands)
type ConfigSource string

const (
	// CLI represents the source the config was retrieved
	CLI ConfigSource = "cli"
)

// ConfigMetadata holds the metadata for configs - used to help validate rollbacks and its underlying functions
type ConfigMetadata struct {
	ConfigUUID     string     `json:"config_uuid"`
	DeviceID       string     `json:"device_id"`        // NDM device ID (e.g. namespace:ip_address)
	ConfigType     ConfigType `json:"config_type"`      // "running", "startup"
	CapturedAt     int64      `json:"captured_at"`      // unix timestamp when the config was stored in bbolt
	LastAccessedAt int64      `json:"last_accessed_at"` // updated on read, to be used for LRU evictions later
	RawHash        string     `json:"raw_hash"`         // hex of the unredacted config
	IsPinned       bool       `json:"is_pinned"`        // determines if a config is up for eviction
	AgentVersion   string     `json:"agent_version"`    // TODO: should it be useful to include as part of the payload?
}

// RawConfig is a temporary backup method until blocks logic is stable
type RawConfig struct {
	Content string `json:"content"`
}

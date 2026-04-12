// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

package store

import "context"

// BlockType represents enums for configuration blocks (currently focused on separating text from sensitive data)
type BlockType string

const (
	// TextBlock represents a regular config text
	TextBlock BlockType = "text"
	// SecretBlock is a placeholder for sensitive data that can be referenced by a UUID
	SecretBlock BlockType = "secret"
)

// ConfigBlock represents a segment of the device configuration
// A list of blocks represents a full configuration for a device
type ConfigBlock struct {
	Type  BlockType `json:"type"`
	Value string    `json:"value,omitempty"` // plain text (for TextBlock only)
	ID    string    `json:"id,omitempty"`    // reference UUID (for SecretBlock only)
}

// ConfigMetadata holds the metadata for configs - used to help validate rollbacks and its underlying functions
type ConfigMetadata struct {
	ConfigUUID     string `json:"config_uuid"`
	DeviceID       string `json:"device_id"`        // NDM device ID (e.g. namespace:ip_address)
	ConfigType     string `json:"config_type"`      // "running", "startup" (from payload pkg's ConfigType)
	CapturedAt     int64  `json:"captured_at"`      // unix timestamp when the config was stored in bbolt
	LastAccessedAt int64  `json:"last_accessed_at"` // updated on read, to be used for LRU evictions later
	RawHash        string `json:"raw_hash"`         // hex of the unredacted config
	IsPinned       bool   `json:"is_pinned"`        // determines if a config is up for eviction
	AgentVersion   string `json:"agent_version"`    // TODO: should it be useful to include as part of the payload?
}

// RawConfig is a temporary backup method until blocks logic is stable
type RawConfig struct {
	Content string `json:"content"`
}

// ConfigStore implements persistent KV store for configurations for rollbacks
// whenever a config is retrieved, we will store agent-side along with the payload sent
// to intake to enable "rollbacks" without sending sensitive data (in configs) back and forth
type ConfigStore interface {
	Close(context.Context) error
	StoreConfig(deviceID, configType string, rawConfig string, blocks []ConfigBlock, secrets map[string]string) (string, error)
	GetConfig(configUUID string) (string, []ConfigBlock, *ConfigMetadata, map[string]string, error)
}

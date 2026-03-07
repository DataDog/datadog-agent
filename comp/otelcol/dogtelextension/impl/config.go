// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dogtelextensionimpl provides the implementation of the dogtelextension component.
package dogtelextensionimpl

import (
	"fmt"
)

// Config defines the configuration for the dogtelextension
type Config struct {
	// Metadata collection settings
	EnableMetadataCollection bool `mapstructure:"enable_metadata_collection"`
	MetadataInterval         int  `mapstructure:"metadata_interval"` // seconds

	// Tagger server settings
	EnableTaggerServer      bool   `mapstructure:"enable_tagger_server"`
	TaggerServerPort        int    `mapstructure:"tagger_server_port"`         // 0 = auto-assign
	TaggerServerAddr        string `mapstructure:"tagger_server_addr"`         // Default: localhost
	TaggerMaxMessageSize    int    `mapstructure:"tagger_max_message_size"`    // Default: 4MB
	TaggerMaxConcurrentSync int    `mapstructure:"tagger_max_concurrent_sync"` // Default: 5

	// Standalone mode (controls secrets and other features)
	// This is typically set via DD_OTEL_STANDALONE environment variable
	StandaloneMode bool `mapstructure:"standalone_mode"`
}

// Validate validates the configuration
func (cfg *Config) Validate() error {
	if cfg.TaggerServerPort < 0 || cfg.TaggerServerPort > 65535 {
		return fmt.Errorf("invalid tagger_server_port: %d (must be 0-65535)", cfg.TaggerServerPort)
	}

	if cfg.TaggerMaxMessageSize <= 0 {
		cfg.TaggerMaxMessageSize = 4 * 1024 * 1024 // 4MB default
	}

	if cfg.MetadataInterval < 0 {
		return fmt.Errorf("invalid metadata_interval: %d (must be >= 0)", cfg.MetadataInterval)
	}

	if cfg.TaggerMaxConcurrentSync <= 0 {
		cfg.TaggerMaxConcurrentSync = 5 // Default
	}

	return nil
}

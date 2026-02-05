// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogtelextension

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateDefaultConfig(t *testing.T) {
	cfg := createDefaultConfig()
	require.NotNil(t, cfg)

	config, ok := cfg.(*Config)
	require.True(t, ok, "config should be of type *Config")

	// Verify default values
	assert.Equal(t, 300, config.MetadataInterval, "default metadata interval should be 300 seconds")
	assert.True(t, config.EnableTaggerServer, "tagger server should be enabled by default")
	assert.Equal(t, 0, config.TaggerServerPort, "default tagger port should be 0 (auto-assign)")
	assert.Equal(t, "localhost", config.TaggerServerAddr, "default tagger address should be localhost")
	assert.Equal(t, 4*1024*1024, config.TaggerMaxMessageSize, "default max message size should be 4MB")
	assert.Equal(t, 5, config.TaggerMaxConcurrentSync, "default max concurrent sync should be 5")
}

func TestConfigValidate_ValidConfig(t *testing.T) {
	cfg := &Config{
		MetadataInterval:        300,
		EnableTaggerServer:      true,
		TaggerServerPort:        5000,
		TaggerServerAddr:        "localhost",
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	err := cfg.Validate()
	assert.NoError(t, err, "valid config should pass validation")
}

func TestConfigValidate_InvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"negative port", -1},
		{"port too large", 65536},
		{"port way too large", 100000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				TaggerServerPort:        tt.port,
				MetadataInterval:        300,
				TaggerMaxMessageSize:    4 * 1024 * 1024,
				TaggerMaxConcurrentSync: 5,
			}

			err := cfg.Validate()
			assert.Error(t, err, "should fail validation for port %d", tt.port)
			assert.Contains(t, err.Error(), "invalid tagger_server_port")
		})
	}
}

func TestConfigValidate_ValidPortRange(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"port 0 (auto-assign)", 0},
		{"port 1", 1},
		{"port 8080", 8080},
		{"port 65535 (max)", 65535},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				TaggerServerPort:        tt.port,
				MetadataInterval:        300,
				TaggerMaxMessageSize:    4 * 1024 * 1024,
				TaggerMaxConcurrentSync: 5,
			}

			err := cfg.Validate()
			assert.NoError(t, err, "should pass validation for port %d", tt.port)
		})
	}
}

func TestConfigValidate_InvalidMetadataInterval(t *testing.T) {
	cfg := &Config{
		MetadataInterval:        -1,
		TaggerServerPort:        5000,
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	err := cfg.Validate()
	assert.Error(t, err, "should fail validation for negative metadata_interval")
	assert.Contains(t, err.Error(), "invalid metadata_interval")
}

func TestConfigValidate_ZeroMetadataInterval(t *testing.T) {
	cfg := &Config{
		MetadataInterval:        0,
		TaggerServerPort:        5000,
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	err := cfg.Validate()
	assert.NoError(t, err, "zero metadata_interval should be valid (disabled)")
}

func TestConfigValidate_MessageSizeDefaults(t *testing.T) {
	tests := []struct {
		name            string
		initialSize     int
		expectedSize    int
		shouldHaveError bool
	}{
		{"negative message size gets default", -1, 4 * 1024 * 1024, false},
		{"zero message size gets default", 0, 4 * 1024 * 1024, false},
		{"positive message size is preserved", 8 * 1024 * 1024, 8 * 1024 * 1024, false},
		{"small positive message size is preserved", 1024, 1024, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				MetadataInterval:        300,
				TaggerServerPort:        5000,
				TaggerMaxMessageSize:    tt.initialSize,
				TaggerMaxConcurrentSync: 5,
			}

			err := cfg.Validate()
			if tt.shouldHaveError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedSize, cfg.TaggerMaxMessageSize)
			}
		})
	}
}

func TestConfigValidate_ConcurrentSyncDefaults(t *testing.T) {
	tests := []struct {
		name         string
		initialSync  int
		expectedSync int
	}{
		{"negative concurrent sync gets default", -1, 5},
		{"zero concurrent sync gets default", 0, 5},
		{"positive concurrent sync is preserved", 10, 10},
		{"one concurrent sync is preserved", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				MetadataInterval:        300,
				TaggerServerPort:        5000,
				TaggerMaxMessageSize:    4 * 1024 * 1024,
				TaggerMaxConcurrentSync: tt.initialSync,
			}

			err := cfg.Validate()
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedSync, cfg.TaggerMaxConcurrentSync)
		})
	}
}

func TestConfigValidate_MultipleInvalidFields(t *testing.T) {
	cfg := &Config{
		MetadataInterval:        -100,
		TaggerServerPort:        70000,
		TaggerMaxMessageSize:    -1,
		TaggerMaxConcurrentSync: 0,
	}

	err := cfg.Validate()
	assert.Error(t, err, "should fail validation when multiple fields are invalid")
	// Should fail on first validation error (port)
	assert.Contains(t, err.Error(), "invalid tagger_server_port")
}

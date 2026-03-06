// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package state

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMergeRCConfigWithEmptyData(t *testing.T) {
	emptyUpdateStatus := func(_ string, _ ApplyStatus) {}

	content, err := MergeRCAgentConfig(emptyUpdateStatus, make(map[string]RawConfig))
	assert.NoError(t, err)
	assert.Equal(t, ConfigContent{}, content)
}

func TestParseConfigAgentConfig(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		metadata Metadata
		wantErr  bool
		wantName string
	}{
		{
			name: "valid config",
			data: []byte(`{"name": "test-config", "config": {"log_level": "debug"}}`),
			metadata: Metadata{
				ID:      "test-id",
				Version: 1,
			},
			wantErr:  false,
			wantName: "test-config",
		},
		{
			name:     "invalid json",
			data:     []byte(`{invalid json}`),
			metadata: Metadata{},
			wantErr:  true,
		},
		{
			name:     "empty config",
			data:     []byte(`{}`),
			metadata: Metadata{},
			wantErr:  false,
			wantName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseConfigAgentConfig(tt.data, tt.metadata)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantName, result.Config.Name)
				assert.Equal(t, tt.metadata, result.Metadata)
			}
		})
	}
}

func TestParseConfigAgentConfigOrder(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		metadata     Metadata
		wantErr      bool
		wantOrder    []string
		wantInternal []string
	}{
		{
			name: "valid order config",
			data: []byte(`{"order": ["layer1", "layer2"], "internal_order": ["internal1"]}`),
			metadata: Metadata{
				ID:      "order-id",
				Version: 1,
			},
			wantErr:      false,
			wantOrder:    []string{"layer1", "layer2"},
			wantInternal: []string{"internal1"},
		},
		{
			name:     "invalid json",
			data:     []byte(`{not valid}`),
			metadata: Metadata{},
			wantErr:  true,
		},
		{
			name:         "empty order",
			data:         []byte(`{}`),
			metadata:     Metadata{},
			wantErr:      false,
			wantOrder:    nil,
			wantInternal: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseConfigAgentConfigOrder(tt.data, tt.metadata)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantOrder, result.Config.Order)
				assert.Equal(t, tt.wantInternal, result.Config.InternalOrder)
				assert.Equal(t, tt.metadata, result.Metadata)
			}
		})
	}
}

func TestMergeRCAgentConfigWithValidData(t *testing.T) {
	appliedConfigs := make(map[string]ApplyStatus)
	updateStatus := func(cfgPath string, status ApplyStatus) {
		appliedConfigs[cfgPath] = status
	}

	orderConfig := `{"order": ["layer1"], "internal_order": []}`
	layerConfig := `{"name": "layer1", "config": {"log_level": "debug"}}`

	updates := map[string]RawConfig{
		"datadog/1/AGENT_CONFIG/configuration_order/configv1": {
			Config:   []byte(orderConfig),
			Metadata: Metadata{ID: "configuration_order"},
		},
		"datadog/1/AGENT_CONFIG/layer1/configv1": {
			Config:   []byte(layerConfig),
			Metadata: Metadata{ID: "layer1"},
		},
	}

	content, err := MergeRCAgentConfig(updateStatus, updates)
	assert.NoError(t, err)
	assert.Equal(t, "debug", content.LogLevel)
}

func TestMergeRCAgentConfigWithInvalidPath(t *testing.T) {
	appliedConfigs := make(map[string]ApplyStatus)
	updateStatus := func(cfgPath string, status ApplyStatus) {
		appliedConfigs[cfgPath] = status
	}

	updates := map[string]RawConfig{
		"invalid/path/format": {
			Config:   []byte(`{}`),
			Metadata: Metadata{},
		},
	}

	_, err := MergeRCAgentConfig(updateStatus, updates)
	assert.Error(t, err)
	assert.Equal(t, ApplyStateError, appliedConfigs["invalid/path/format"].State)
}

func TestMergeRCAgentConfigWithInvalidJSON(t *testing.T) {
	appliedConfigs := make(map[string]ApplyStatus)
	updateStatus := func(cfgPath string, status ApplyStatus) {
		appliedConfigs[cfgPath] = status
	}

	updates := map[string]RawConfig{
		"datadog/1/AGENT_CONFIG/configuration_order/configv1": {
			Config:   []byte(`{invalid json}`),
			Metadata: Metadata{},
		},
	}

	_, err := MergeRCAgentConfig(updateStatus, updates)
	assert.Error(t, err)
}

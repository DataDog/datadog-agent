// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"testing"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/stretchr/testify/assert"
)

func TestGetMetadata(t *testing.T) {
	tests := []struct {
		name             string
		configSource     string
		expectedSource   string
		expectedProvider string
	}{
		{
			name:             "config source with provider and source",
			configSource:     "file:/path/to/config.yaml",
			expectedSource:   "/path/to/config.yaml",
			expectedProvider: "file",
		},
		{
			name:             "config source with only provider",
			configSource:     "file",
			expectedSource:   "unknown",
			expectedProvider: "file",
		},
		{
			name:             "config source with multiple colons",
			configSource:     "file:/path/to:config.yaml",
			expectedSource:   "/path/to:config.yaml",
			expectedProvider: "file",
		},
		{
			name:             "empty config source",
			configSource:     "",
			expectedSource:   "unknown",
			expectedProvider: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockInfo := MockInfo{
				Name:         "test_check",
				CheckID:      checkid.ID("test_id"),
				Source:       tt.configSource,
				InitConf:     "init_config",
				InstanceConf: "instance_config",
			}

			metadata := GetMetadata(mockInfo, false)

			assert.Equal(t, "test_id", metadata["config.hash"])
			assert.Equal(t, tt.expectedProvider, metadata["config.provider"])
			assert.Equal(t, tt.expectedSource, metadata["config.source"])
		})
	}
}

func TestGetMetadataWithConfig(t *testing.T) {
	mockInfo := MockInfo{
		Name:         "test_check",
		CheckID:      checkid.ID("test_id"),
		Source:       "file:/path/to/config.yaml",
		InitConf:     "init_config",
		InstanceConf: "instance_config",
	}

	metadata := GetMetadata(mockInfo, true)

	assert.Equal(t, "test_id", metadata["config.hash"])
	assert.Equal(t, "file", metadata["config.provider"])
	assert.Equal(t, "/path/to/config.yaml", metadata["config.source"])
	assert.Equal(t, "init_config", metadata["init_config"])
	assert.Equal(t, "instance_config", metadata["instance_config"])
}

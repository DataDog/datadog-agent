// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package limits

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestMetadataConstants(t *testing.T) {
	// Verify metadata constants match intake server limits
	assert.Equal(t, 1024*1024, MetadataMaxUncompressed, "MetadataMaxUncompressed should be 1MB")
	assert.Equal(t, 800*1024, MetadataTargetBatch, "MetadataTargetBatch should be 800KB")
	assert.Less(t, MetadataTargetBatch, MetadataMaxUncompressed, "TargetBatch should be less than MaxUncompressed")
}

func TestGetMetadata(t *testing.T) {
	// Metadata limits are fixed and don't require config
	limits := Get(Metadata, nil)

	assert.Equal(t, MetadataMaxUncompressed, limits.MaxUncompressed)
	assert.Equal(t, MetadataTargetBatch, limits.TargetBatch)
	assert.Equal(t, 0, limits.MaxCompressed, "Metadata has no compressed limit")
	assert.Equal(t, 0, limits.MaxItems, "Metadata has no item limit")
}

func TestGetSeries(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("serializer_max_series_payload_size", 500000)
	cfg.SetWithoutSource("serializer_max_series_uncompressed_payload_size", 5000000)
	cfg.SetWithoutSource("serializer_max_series_points_per_payload", 10000)

	limits := Get(Series, cfg)

	assert.Equal(t, 500000, limits.MaxCompressed)
	assert.Equal(t, 5000000, limits.MaxUncompressed)
	assert.Equal(t, 10000, limits.MaxItems)
	assert.Equal(t, 0, limits.TargetBatch, "Series has no target batch")
}

func TestGetDefault(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("serializer_max_payload_size", 2500000)
	cfg.SetWithoutSource("serializer_max_uncompressed_payload_size", 4000000)

	limits := Get(Default, cfg)

	assert.Equal(t, 2500000, limits.MaxCompressed)
	assert.Equal(t, 4000000, limits.MaxUncompressed)
	assert.Equal(t, 0, limits.MaxItems, "Default has no item limit")
	assert.Equal(t, 0, limits.TargetBatch, "Default has no target batch")
}

func TestExceeds(t *testing.T) {
	tests := []struct {
		name         string
		limits       Limits
		compressed   int
		uncompressed int
		expected     bool
	}{
		{
			name:         "under both limits",
			limits:       Limits{MaxCompressed: 1000, MaxUncompressed: 2000},
			compressed:   500,
			uncompressed: 1000,
			expected:     false,
		},
		{
			name:         "exceeds compressed limit",
			limits:       Limits{MaxCompressed: 1000, MaxUncompressed: 2000},
			compressed:   1500,
			uncompressed: 1000,
			expected:     true,
		},
		{
			name:         "exceeds uncompressed limit",
			limits:       Limits{MaxCompressed: 1000, MaxUncompressed: 2000},
			compressed:   500,
			uncompressed: 2500,
			expected:     true,
		},
		{
			name:         "exceeds both limits",
			limits:       Limits{MaxCompressed: 1000, MaxUncompressed: 2000},
			compressed:   1500,
			uncompressed: 2500,
			expected:     true,
		},
		{
			name:         "exactly at compressed limit",
			limits:       Limits{MaxCompressed: 1000, MaxUncompressed: 2000},
			compressed:   1000,
			uncompressed: 1000,
			expected:     false,
		},
		{
			name:         "exactly at uncompressed limit",
			limits:       Limits{MaxCompressed: 1000, MaxUncompressed: 2000},
			compressed:   500,
			uncompressed: 2000,
			expected:     false,
		},
		{
			name:         "zero compressed limit means no limit",
			limits:       Limits{MaxCompressed: 0, MaxUncompressed: 2000},
			compressed:   999999,
			uncompressed: 1000,
			expected:     false,
		},
		{
			name:         "zero uncompressed limit means no limit",
			limits:       Limits{MaxCompressed: 1000, MaxUncompressed: 0},
			compressed:   500,
			uncompressed: 999999,
			expected:     false,
		},
		{
			name:         "both limits zero means no limits",
			limits:       Limits{MaxCompressed: 0, MaxUncompressed: 0},
			compressed:   999999,
			uncompressed: 999999,
			expected:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.limits.Exceeds(tc.compressed, tc.uncompressed)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestEndpointValues(t *testing.T) {
	// Verify endpoint constants are distinct
	assert.NotEqual(t, Default, Series)
	assert.NotEqual(t, Default, Metadata)
	assert.NotEqual(t, Series, Metadata)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestTagsFromBuildInfo(t *testing.T) {
	tests := []struct {
		name      string
		buildInfo component.BuildInfo
		expected  []string
	}{
		{
			name:      "empty build info",
			buildInfo: component.BuildInfo{},
			expected:  nil,
		},
		{
			name: "version only",
			buildInfo: component.BuildInfo{
				Version: "1.2.3",
			},
			expected: []string{"version:1.2.3"},
		},
		{
			name: "command only",
			buildInfo: component.BuildInfo{
				Command: "otel-agent",
			},
			expected: []string{"command:otel-agent"},
		},
		{
			name: "version and command",
			buildInfo: component.BuildInfo{
				Version: "1.2.3",
				Command: "otel-agent",
			},
			expected: []string{"version:1.2.3", "command:otel-agent"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := TagsFromBuildInfo(tt.buildInfo)
			assert.Equal(t, tt.expected, tags)
		})
	}
}

func TestCreateLivenessSerie(t *testing.T) {
	hostname := "test-host"
	// 1000 seconds in nanoseconds
	timestampNs := uint64(1000 * 1e9)
	tags := []string{"version:1.0.0", "command:otel-agent"}

	serie := CreateLivenessSerie(hostname, timestampNs, tags)

	require.NotNil(t, serie)
	assert.Equal(t, "otel.dogtel_extension.running", serie.Name)
	assert.Equal(t, hostname, serie.Host)
	assert.Equal(t, metrics.APIGaugeType, serie.MType)
	assert.Equal(t, "otel.dogtel_extension", serie.SourceTypeName)
	assert.Equal(t, metrics.MetricSourceOpenTelemetryCollectorUnknown, serie.Source)

	require.Len(t, serie.Points, 1)
	assert.Equal(t, float64(1000), serie.Points[0].Ts)
	assert.Equal(t, 1.0, serie.Points[0].Value)

	assert.Equal(t, tags, serie.Tags.UnsafeToReadOnlySliceString())
}

func TestCreateLivenessSerie_TimestampConversion(t *testing.T) {
	// Verify nanoseconds are correctly converted to seconds
	timestampNs := uint64(1704067200 * 1e9) // 2024-01-01 00:00:00 UTC in nanoseconds
	serie := CreateLivenessSerie("host", timestampNs, nil)

	require.NotNil(t, serie)
	require.Len(t, serie.Points, 1)
	assert.Equal(t, float64(1704067200), serie.Points[0].Ts)
}

func TestCreateLivenessSerie_EmptyTags(t *testing.T) {
	serie := CreateLivenessSerie("host", uint64(1000*1e9), nil)
	require.NotNil(t, serie)
	assert.Empty(t, serie.Tags.UnsafeToReadOnlySliceString())
}

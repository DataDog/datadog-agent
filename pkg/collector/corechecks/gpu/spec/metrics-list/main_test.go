// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
)

// The metrics list builds from the checked-in spec, AMD-only metrics included.
func TestBuildMetricRowsFromSpec(t *testing.T) {
	specs, err := gpuspec.LoadSpecs()
	require.NoError(t, err)
	rows, err := buildMetricRows(specs)
	require.NoError(t, err)

	byName := map[string]metricRow{}
	for _, row := range rows {
		byName[row.metricName] = row
	}
	amdOnly := 0
	for name, spec := range specs.Metrics.Metrics {
		row, ok := byName[gpuspec.PrefixedMetricName(specs, name)]
		if !ok {
			continue // metrics without aggregation metadata are not listed
		}
		if spec.Support.AMDOnly {
			amdOnly++
			assert.Equal(t, "amd", row.minSupportedArchitecture, name)
		} else {
			assert.NotEqual(t, "amd", row.minSupportedArchitecture, name)
		}
	}
	assert.Positive(t, amdOnly, "the spec has AMD-only metrics")
}

// A metric that no architecture supports and that is not marked AMD-only is a
// spec error.
func TestBuildMetricRowsRejectsUnsupportedMetrics(t *testing.T) {
	specs, err := gpuspec.LoadSpecs()
	require.NoError(t, err)
	spec := specs.Metrics.Metrics["sm_active"]
	spec.Support.AMDOnly = false
	spec.Support.UnsupportedArchitectures = slices.Collect(maps.Keys(specs.Architectures.Architectures))
	specs.Metrics.Metrics["sm_active"] = spec
	_, err = buildMetricRows(specs)
	assert.ErrorContains(t, err, "unsupported on all known architectures")
}

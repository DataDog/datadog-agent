// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

func TestFindMetricFamily(t *testing.T) {
	families := []prometheus.MetricFamily{
		{Name: "apiserver_storage_objects"},
		{Name: "etcd_object_counts"},
	}

	assert.Same(t, &families[0], FindMetricFamily(families, "apiserver_storage_objects", "etcd_object_counts"))
	assert.Same(t, &families[1], FindMetricFamily(families, "missing", "etcd_object_counts"))
	assert.Nil(t, FindMetricFamily(families, "missing"))
}

// TestFilterMetricLines streams a snapshot of a real /metrics response (testdata) through
// filterMetricLines and checks that it actually filters: the requested families' HELP/TYPE and
// sample lines survive, unrelated families are dropped, and the result is much smaller than the
// input, which is the whole point of streaming instead of buffering the raw response.
func TestFilterMetricLines(t *testing.T) {
	raw, err := os.ReadFile("testdata/apiserver_metrics_snapshot.txt")
	require.NoError(t, err)

	file, err := os.Open("testdata/apiserver_metrics_snapshot.txt")
	require.NoError(t, err)
	defer file.Close()

	filtered, err := filterMetricLines(file, []string{"apiserver_storage_objects", "etcd_object_counts", "apiserver_resource_size_estimate_bytes"})
	require.NoError(t, err)

	// Streaming must actually shrink the payload: only a handful of the snapshot's lines
	// belong to the requested families.
	assert.Less(t, len(filtered), len(raw), "filtered output should be much smaller than the raw input")

	families, err := prometheus.ParseMetrics(filtered)
	require.NoError(t, err)

	storageObjects := FindMetricFamily(families, "apiserver_storage_objects")
	require.NotNil(t, storageObjects)
	assert.Equal(t, "GAUGE", storageObjects.Type)
	assert.Len(t, storageObjects.Samples, 11)

	legacyObjectCounts := FindMetricFamily(families, "etcd_object_counts")
	require.NotNil(t, legacyObjectCounts)
	assert.Equal(t, "GAUGE", legacyObjectCounts.Type)
	assert.Len(t, legacyObjectCounts.Samples, 7)

	sizeEstimate := FindMetricFamily(families, "apiserver_resource_size_estimate_bytes")
	require.NotNil(t, sizeEstimate)
	assert.Equal(t, "GAUGE", sizeEstimate.Type)
	assert.Len(t, sizeEstimate.Samples, 4)

	// None of the unrelated families from the snapshot should have survived filtering.
	for _, unrelated := range []string{
		"apiserver_request_total",
		"apiserver_request_duration_seconds",
	} {
		assert.Nil(t, FindMetricFamily(families, unrelated), "unrelated family %q should have been filtered out", unrelated)
		assert.False(t, bytes.Contains(filtered, []byte(unrelated)), "filtered output should not contain %q", unrelated)
	}

	// apiserver_resource_objects shares the apiserver_resource_ prefix with the size estimate and
	// must not leak through. Only check the parsed families: its name legitimately appears in the
	// kept HELP text of apiserver_storage_objects.
	assert.Nil(t, FindMetricFamily(families, "apiserver_resource_objects"))
}

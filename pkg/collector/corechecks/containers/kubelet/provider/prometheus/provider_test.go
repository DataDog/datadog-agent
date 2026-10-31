// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubelet

package prometheus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

// TestIgnoreMetricByLabelWithYAMLDecodedList checks that ignore_metrics_by_labels values coming
// from a real YAML-decoded instance config are applied. IgnoreMetricsByLabels is declared as
// map[string]interface{}. A list value such as `["*"]` decodes into []interface{}, not []string,
// as of go.yaml.in/yaml/v2 and go.yaml.in/yaml/v3 alike, so ignoreMetricByLabel must match against
// that type.
func TestIgnoreMetricByLabelWithYAMLDecodedList(t *testing.T) {
	cfg := &common.KubeletConfig{}
	err := cfg.Parse([]byte("ignore_metrics_by_labels:\n  some_label:\n  - \"*\"\n"))
	require.NoError(t, err)

	p, err := NewProvider(cfg, nil, nil)
	require.NoError(t, err)

	filtered := &prometheus.Sample{
		Metric: prometheus.Metric{"some_label": "value1"},
	}
	assert.True(t, p.ignoreMetricByLabel(filtered, "some_metric"),
		"expected the metric to be ignored because label %q matches the configured wildcard filter",
		"some_label")

	unfiltered := &prometheus.Sample{
		Metric: prometheus.Metric{"other_label": "value1"},
	}
	assert.False(t, p.ignoreMetricByLabel(unfiltered, "some_metric"),
		"expected the metric not to be ignored because it has no %q label",
		"some_label")
}

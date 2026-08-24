// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package contexttop

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

func TestFromReaderSummarizesMetricTags(t *testing.T) {
	contexts := []aggregator.ContextDebugRepr{
		{Name: "requests", Host: "host-a", MetricTags: []string{"env:prod", "endpoint:/a"}, TaggerTags: []string{"pod_name:first"}},
		{Name: "requests", Host: "host-b", MetricTags: []string{"env:prod", "endpoint:/b"}, TaggerTags: []string{"pod_name:second"}},
		{Name: "errors", MetricTags: []string{"env:prod"}},
	}

	var dump bytes.Buffer
	enc := json.NewEncoder(&dump)
	for _, context := range contexts {
		require.NoError(t, enc.Encode(context))
	}

	result, err := FromReader(&dump, 10, 5)
	require.NoError(t, err)
	require.Equal(t, Result{
		Metrics: []Metric{
			{
				Name:     "requests",
				Contexts: 2,
				Tags: []Tag{
					{Key: "endpoint", UniqueValues: 2},
					{Key: "env", UniqueValues: 1},
				},
			},
			{
				Name:     "errors",
				Contexts: 1,
				Tags:     []Tag{{Key: "env", UniqueValues: 1}},
			},
		},
	}, result)
}

func TestFromReaderLimitsMetricsAndTags(t *testing.T) {
	contexts := []aggregator.ContextDebugRepr{
		{Name: "first", MetricTags: []string{"alpha:1", "beta:1", "gamma:1"}},
		{Name: "first", MetricTags: []string{"alpha:2", "beta:2", "gamma:1"}},
		{Name: "second"},
		{Name: "third"},
		{Name: "fourth"},
	}

	var dump bytes.Buffer
	enc := json.NewEncoder(&dump)
	for _, context := range contexts {
		require.NoError(t, enc.Encode(context))
	}

	result, err := FromReader(&dump, 2, 1)
	require.NoError(t, err)
	require.Equal(t, Result{
		Metrics: []Metric{
			{
				Name:           "first",
				Contexts:       2,
				Tags:           []Tag{{Key: "alpha", UniqueValues: 2}},
				OtherTags:      2,
				OtherTagValues: 3,
			},
			{Name: "fourth", Contexts: 1, Tags: []Tag{}},
		},
		OtherMetrics:  2,
		OtherContexts: 2,
	}, result)
}

func TestFromReaderIncludesSingleRemainder(t *testing.T) {
	var dump bytes.Buffer
	enc := json.NewEncoder(&dump)
	for _, name := range []string{"first", "second", "third"} {
		require.NoError(t, enc.Encode(aggregator.ContextDebugRepr{Name: name}))
	}

	result, err := FromReader(&dump, 2, 5)
	require.NoError(t, err)
	require.Len(t, result.Metrics, 3)
	require.Zero(t, result.OtherMetrics)
}

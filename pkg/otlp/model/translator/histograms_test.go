// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package translator

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDeltaHistogramOptions(t *testing.T) {
	tests := []struct {
		name     string
		otlpfile string
		ddogfile string
		options  []Option
	}{
		{
			name:     "simple histogram distributions",
			otlpfile: "testdata/otlpdata/histogram/simple-delta.json",
			ddogfile: "testdata/datadogdata/histogram/simple-delta_dist-nocs.json",
			options: []Option{
				WithHistogramMode(HistogramModeDistributions),
			},
		},
		{
			name:     "simple histogram distributions, with count sum metrics",
			otlpfile: "testdata/otlpdata/histogram/simple-delta.json",
			ddogfile: "testdata/datadogdata/histogram/simple-delta_dist-cs.json",
			options: []Option{
				WithHistogramMode(HistogramModeDistributions),
				WithCountSumMetrics(),
			},
		},
		{
			name:     "simple histogram buckets as counts, no count sum metrics",
			otlpfile: "testdata/otlpdata/histogram/simple-delta.json",
			ddogfile: "testdata/datadogdata/histogram/simple-delta_counters-nocs.json",
			options: []Option{
				WithHistogramMode(HistogramModeCounters),
			},
		},
		{
			name:     "simple histogram buckets as counts, with count sum metrics",
			otlpfile: "testdata/otlpdata/histogram/simple-delta.json",
			ddogfile: "testdata/datadogdata/histogram/simple-delta_counters-cs.json",
			options: []Option{
				WithHistogramMode(HistogramModeCounters),
				WithCountSumMetrics(),
			},
		},
		{
			name:     "simple histogram no buckets count sum metrics",
			otlpfile: "testdata/otlpdata/histogram/simple-delta.json",
			ddogfile: "testdata/datadogdata/histogram/simple-delta_nobuckets-cs.json",
			options: []Option{
				WithHistogramMode(HistogramModeNoBuckets),
				WithCountSumMetrics(),
			},
		},
	}

	for _, testinstance := range tests {
		t.Run(testinstance.name, func(t *testing.T) {
			translator, err := New(zap.NewNop(), testinstance.options...)
			require.NoError(t, err)
			AssertTranslatorMap(t, translator, testinstance.otlpfile, testinstance.ddogfile)
		})
	}
}

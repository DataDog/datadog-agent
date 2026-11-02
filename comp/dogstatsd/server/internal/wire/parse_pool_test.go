// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package wire

import (
	"testing"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/stretchr/testify/require"
)

func TestParseMetricSamplePoolErrors(t *testing.T) {
	for _, tc := range []struct {
		name           string
		message        string
		readTimestamps bool
		wantError      bool
	}{
		{"numeric", "metric:1:bad|g", true, true},
		{"empty_values", "metric::|g", true, true},
		{"sample_rate", "metric:1:2|g|@bad", true, true},
		{"timestamp_text", "metric:1:2|g|Tbad", true, true},
		{"timestamp_empty", "metric:1:2|g|T", true, true},
		{"timestamp_overflow", "metric:1:2|g|T9999999999999999999999", true, true},
		{"timestamp_zero", "metric:1:2|g|T0", true, true},
		{"timestamp_negative", "metric:1:2|g|T-1", true, true},
		{"timestamp_disabled", "metric:1:2|g|Tbad", false, false},
		{"success", "metric:1:2|g|T123", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tel := telemetryimpl.NewMock(t)
			pool := NewFloat64ListPool(true, tel)
			p := NewParser(Options{InternerSize: 4, ReadTimestamps: tc.readTimestamps}, pool, 1, NewInternerTelemetry(false, tel))
			sample, err := p.ParseMetric([]byte(tc.message))
			if tc.wantError {
				require.Error(t, err)
				require.Nil(t, sample.Values)
			} else {
				require.NoError(t, err)
				require.Equal(t, []float64{1, 2}, sample.Values)
				pool.Put(sample.Values)
			}
			for _, name := range []string{"float64_list_pool_get", "float64_list_pool_put"} {
				values, err := tel.GetCountMetric("dogstatsd", name)
				require.NoError(t, err)
				require.Len(t, values, 1)
				require.Equal(t, float64(1), values[0].Value())
			}
			values, err := tel.GetGaugeMetric("dogstatsd", "float64_list_pool")
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Zero(t, values[0].Value())
		})
	}
}

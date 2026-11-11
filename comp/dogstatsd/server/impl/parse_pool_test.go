// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !serverless

package serverimpl

import (
	"testing"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/stretchr/testify/require"
)

func TestParseMetricSamplePoolErrors(t *testing.T) {
	for _, tc := range []struct {
		name           string
		message        string
		readTimestamps bool
		wantError      bool
		wantBorrows    float64
	}{
		{"numeric", "metric:1:bad|g", true, true, 1},
		{"empty_values", "metric::|g", true, true, 1},
		{"sample_rate", "metric:1:2|g|@bad", true, true, 1},
		{"timestamp_text", "metric:1:2|g|Tbad", true, true, 1},
		{"timestamp_zero", "metric:1:2|g|T0", true, true, 1},
		{"timestamp_disabled", "metric:1:2|g|Tbad", false, false, 1},
		{"success", "metric:1:2|g|T123", true, false, 1},
		{"scalar_sample_rate", "metric:1|g|@bad", true, true, 0},
		{"scalar_timestamp", "metric:1|g|Tbad", true, true, 0},
		{"set_timestamp", "metric:one|s|T0", true, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tel := telemetryimpl.NewMock(t)
			cfg := configmock.New(t)
			cfg.SetInTest("telemetry.enabled", true)
			cfg.SetInTest("agent_telemetry.enabled", false)
			cfg.SetInTest("dogstatsd_no_aggregation_pipeline", tc.readTimestamps)
			pool := newFloat64ListPool(cfg, tel)
			gets := pool.tlmFloat64ListPoolGet.WithValues()
			puts := pool.tlmFloat64ListPoolPut.WithValues()
			usage := pool.tlmFloat64ListPool.WithValues()
			p := newParser(cfg, pool, 1, option.None[workloadmeta.Component](), newSiTelemetry(false, tel))

			sample, err := p.parseMetricSample([]byte(tc.message))
			if tc.wantError {
				require.Error(t, err)
				require.Nil(t, sample.values)
			} else {
				require.NoError(t, err)
				require.Equal(t, []float64{1, 2}, sample.values)
				require.Zero(t, puts.Get())
				require.Equal(t, tc.wantBorrows, usage.Get())
				pool.put(sample.values)
			}
			require.Equal(t, tc.wantBorrows, gets.Get())
			require.Equal(t, tc.wantBorrows, puts.Get())
			require.Zero(t, usage.Get())
		})
	}
}

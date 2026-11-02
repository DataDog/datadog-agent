// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package serverimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics/metricname"
	"github.com/stretchr/testify/require"
)

func TestServerMetricPoolOwnership(t *testing.T) {
	for _, tc := range []struct {
		name      string
		message   string
		filtered  bool
		wantError bool
	}{
		{"success", "metric:1:2|g", false, false},
		{"filtered", "metric:1:2|g", true, false},
		{"numeric_error", "metric:1:bad|g", false, true},
		{"sample_rate_error", "metric:1:2|g|@bad", false, true},
		{"timestamp_error", "metric:1:2|g|Tbad", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := noListenerConfig()
			cfg["telemetry.enabled"] = true
			cfg["agent_telemetry.enabled"] = false
			cfg["dogstatsd_no_aggregation_pipeline"] = true
			deps, s := fulfillDepsWithInactiveServer(t, cfg)
			p := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
			var filter *metricname.Matcher
			if tc.filtered {
				matcher := metricname.NewMatcher([]string{"metric"}, false)
				filter = &matcher
			}
			samples, err := s.parseMetricMessage(nil, p, []byte(tc.message), "", 0, "", false, filter)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tc.filtered {
					require.Empty(t, samples)
				} else {
					require.Len(t, samples, 2)
					require.Equal(t, 1.0, samples[0].Value)
					require.Equal(t, 2.0, samples[1].Value)
				}
			}
			tel := deps.Telemetry.(telemetry.Mock)
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

func TestParserPoolTelemetryConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name           string
		telemetry      bool
		agentTelemetry bool
		fips           bool
		wantTelemetry  bool
	}{
		{"disabled", false, false, false, false},
		{"telemetry", true, false, false, true},
		{"agent_telemetry", false, true, false, true},
		{"fips", false, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("telemetry.enabled", tc.telemetry)
			cfg.SetInTest("agent_telemetry.enabled", tc.agentTelemetry)
			cfg.SetInTest("fips.enabled", tc.fips)
			cfg.SetInTest("site", "datadoghq.com")
			tel := telemetryimpl.NewMock(t)
			pool := newFloat64ListPool(cfg, tel)
			before, err := tel.GetRegistry().Gather()
			require.NoError(t, err)
			pool.Put(pool.Get())
			if tc.wantTelemetry {
				values, err := tel.GetCountMetric("dogstatsd", "float64_list_pool_get")
				require.NoError(t, err)
				require.Len(t, values, 1)
				require.Equal(t, float64(1), values[0].Value())
			} else {
				after, err := tel.GetRegistry().Gather()
				require.NoError(t, err)
				require.Equal(t, before, after)
			}
		})
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"os"
	"path/filepath"
	"testing"

	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
	"github.com/stretchr/testify/require"
)

func TestLoadTestbenchLogTimeBuckets(t *testing.T) {
	for _, test := range []struct {
		name string
		json string
		want *observerimpl.LogCountBucketConfig
	}{
		{"omitted", `{}`, nil},
		{"null", `{"log_time_buckets":null}`, nil},
		{"enabled defaults", `{"log_time_buckets":{"enabled":true}}`, &observerimpl.LogCountBucketConfig{Enabled: true, BucketSeconds: 5, IdleTTLSeconds: 300, RetentionSeconds: 600}},
		{"explicit zero ttl", `{"log_time_buckets":{"enabled":true,"bucket_seconds":10,"idle_ttl_seconds":0,"retention_seconds":30}}`, &observerimpl.LogCountBucketConfig{Enabled: true, BucketSeconds: 10, IdleTTLSeconds: 0, RetentionSeconds: 30}},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			require.NoError(t, os.WriteFile(path, []byte(test.json), 0600))
			settings, err := LoadTestbenchParams(path)
			require.NoError(t, err)
			require.Equal(t, test.want, settings.LogCountBuckets)
		})
	}
}

func TestLoadTestbenchLogTimeBucketsWithMetricPipeline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	config := `{"use_metric_pipeline":true,"anomaly_cutoff":false,"log_time_buckets":{"enabled":true},"components":{}}`
	require.NoError(t, os.WriteFile(path, []byte(config), 0600))
	settings, err := LoadTestbenchParams(path)
	require.NoError(t, err)
	require.True(t, settings.UseMetricPipeline)
	require.NotNil(t, settings.MetricPipelineAnomalyCutoff)
	require.False(t, *settings.MetricPipelineAnomalyCutoff)
	require.Equal(t, &observerimpl.LogCountBucketConfig{
		Enabled: true, BucketSeconds: 5, IdleTTLSeconds: 300, RetentionSeconds: 600,
	}, settings.LogCountBuckets)
}

func TestLoadTestbenchStablePipeline(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		wantError   string
		wantName    string
		wantWarmup  int
		wantDefault bool
	}{
		{name: "omitted", config: `{"use_metric_pipeline":true,"components":{}}`, wantDefault: true},
		{name: "scanmw", config: `{"use_metric_pipeline":true,"stable":{"scanmw":{"min_points":42,"max_points":84}},"components":{}}`, wantName: "stable_pipeline:scanmw"},
		{name: "scanwelch", config: `{"use_metric_pipeline":true,"stable":{"scanwelch":{}},"components":{}}`, wantName: "stable_pipeline:scanwelch"},
		{name: "bocpd", config: `{"use_metric_pipeline":true,"stable":{"bocpd":{"warmup_points":42,"max_run_length":84}},"components":{}}`, wantName: "stable_pipeline:bocpd", wantWarmup: 42},
		{name: "multiple", config: `{"use_metric_pipeline":true,"stable":{"scanmw":{},"bocpd":{}},"components":{}}`, wantError: "stable pipeline accepts at most one detector block"},
		{name: "unknown field", config: `{"use_metric_pipeline":true,"stable":{"scanmw":{"enabled":true}},"components":{}}`, wantError: "parse stable pipeline config: json: unknown field \"enabled\""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			require.NoError(t, os.WriteFile(path, []byte(test.config), 0600))
			settings, err := LoadTestbenchParams(path)
			if test.wantError != "" {
				require.EqualError(t, err, test.wantError)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, settings.StablePipeline)
			if test.wantDefault {
				require.InDelta(t, 0.05, settings.StablePipeline.MinRelativeChange, 1e-12)
				require.Nil(t, settings.StablePipeline.ScanMW)
				require.Nil(t, settings.StablePipeline.ScanWelch)
				require.Nil(t, settings.StablePipeline.BOCPD)
				return
			}
			switch test.wantName {
			case "stable_pipeline:scanmw":
				require.NotNil(t, settings.StablePipeline.ScanMW)
			case "stable_pipeline:scanwelch":
				require.NotNil(t, settings.StablePipeline.ScanWelch)
			case "stable_pipeline:bocpd":
				require.NotNil(t, settings.StablePipeline.BOCPD)
				require.Equal(t, test.wantWarmup, settings.StablePipeline.BOCPD.WarmupPoints)
			}
		})
	}
}

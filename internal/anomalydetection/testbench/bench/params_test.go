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

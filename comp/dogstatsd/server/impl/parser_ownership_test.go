// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	"testing"

	noops "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/server/internal/wire"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/stretchr/testify/require"
)

func TestParserEnrichmentOwnership(t *testing.T) {
	for _, serverlessMode := range []bool{false, true} {
		cfg := configmock.New(t)
		cfg.SetInTest("dogstatsd_origin_detection_client", true)
		cfg.SetInTest("dogstatsd_no_aggregation_pipeline", true)
		telemetry := noops.GetCompatComponent()
		pool := newFloat64ListPool(cfg, telemetry)
		p := newParser(cfg, pool, 1, option.None[workloadmeta.Component](), wire.NewInternerTelemetry(false, telemetry))
		sample, err := p.ParseMetric([]byte("latency:3:5|ms|@0.5|#role:worker|c:cid-abc|T123"))
		require.NoError(t, err)
		out := enrichMetricSample(nil, sample, "socket-origin", 42, "listener", enrichConfig{defaultHostname: "host", serverlessMode: serverlessMode}, nil)
		require.Len(t, out, 2)
		wantHost := "host"
		if serverlessMode {
			wantHost = ""
		}
		for i, value := range []float64{3, 5} {
			require.Equal(t, "latency", out[i].Name)
			require.Equal(t, value, out[i].Value)
			require.Equal(t, metrics.HistogramType, out[i].Mtype)
			require.Equal(t, "millisecond", out[i].Unit)
			require.Equal(t, wantHost, out[i].Host)
			require.Equal(t, 123.0, out[i].Timestamp)
			require.Equal(t, 0.5, out[i].SampleRate)
			require.Equal(t, []string{"role:worker"}, out[i].Tags)
			require.Equal(t, "abc", out[i].OriginInfo.LocalData.ContainerID)
		}
		// Mutate before returning ownership; sync.Pool need not reuse a given slice.
		for i := range sample.Values {
			sample.Values[i] = 99
		}
		pool.Put(sample.Values)
		require.Equal(t, 3.0, out[0].Value)
		require.Equal(t, 5.0, out[1].Value)
	}
}

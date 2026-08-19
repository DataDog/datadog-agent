// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/stretchr/testify/require"
)

func TestPassthroughCorrelations(t *testing.T) {
	anomalies := []observerdef.Anomaly{
		{DetectorName: "rrcf", Timestamp: 30, Source: observerdef.SeriesDescriptor{Name: "late"}},
		{DetectorName: "bocpd", Timestamp: 20, Source: observerdef.SeriesDescriptor{Name: "first"}},
		{DetectorName: "rrcf", Timestamp: 10, Source: observerdef.SeriesDescriptor{Name: "early"}},
	}

	correlations := passthroughCorrelations(anomalies)
	require.Len(t, correlations, 3)
	require.Equal(t, "passthrough_bocpd_0", correlations[0].Pattern)
	require.Equal(t, int64(20), correlations[0].FirstSeen)
	require.Equal(t, "passthrough_rrcf_0", correlations[1].Pattern)
	require.Equal(t, int64(10), correlations[1].FirstSeen)
	require.Equal(t, "passthrough_rrcf_1", correlations[2].Pattern)
	require.Equal(t, int64(30), correlations[2].FirstSeen)
	for _, correlation := range correlations {
		require.Len(t, correlation.Anomalies, 1)
		require.Equal(t, correlation.FirstSeen, correlation.LastUpdated)
	}
}

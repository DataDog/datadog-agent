// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package wire

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOutstandingNumericValues(t *testing.T) {
	p := testParser(t, testOptions())
	first, err := p.ParseMetric([]byte("first:1:2|g"))
	require.NoError(t, err)
	defer p.float64List.Put(first.Values)
	second, err := p.ParseMetric([]byte("second:3:4|g"))
	require.NoError(t, err)
	defer p.float64List.Put(second.Values)
	require.Equal(t, []float64{1, 2}, first.Values)
	require.Equal(t, []float64{3, 4}, second.Values)
	second.Values[0] = 99
	require.Equal(t, []float64{1, 2}, first.Values)
}

func TestClientOriginOptions(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name, wantID := "disabled", ""
		if enabled {
			name, wantID = "enabled", "abc"
		}
		t.Run(name, func(t *testing.T) {
			options := testOptions()
			options.OriginDetection = enabled
			p := testParser(t, options)
			metric, err := p.ParseMetric([]byte("metric:1|g|c:cid-abc"))
			require.NoError(t, err)
			require.Equal(t, wantID, metric.LocalData.ContainerID)
			event, err := p.ParseEvent([]byte("_e{1,1}:a|b|c:cid-abc"))
			require.NoError(t, err)
			require.Equal(t, wantID, event.LocalData.ContainerID)
			check, err := p.ParseServiceCheck([]byte("_sc|check|0|c:cid-abc"))
			require.NoError(t, err)
			require.Equal(t, wantID, check.LocalData.ContainerID)
		})
	}
}

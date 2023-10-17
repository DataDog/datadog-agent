// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

//go:embed fixtures/metric_bytes_v1
var metricsDatav1 []byte

func TestV1MetricPayloads(t *testing.T) {
	t.Run("parseMetricSeries empty body should return error", func(t *testing.T) {
		metrics, err := ParseV1MetricSeries(api.Payload{
			Data:     []byte(""),
			Encoding: encodingDeflate,
		})
		assert.Error(t, err)
		assert.Empty(t, metrics)
	})
	t.Run("parseMetricSeries valid body should parse metrics", func(t *testing.T) {
		metrics, err := ParseV1MetricSeries(api.Payload{Data: metricsDatav1, Encoding: encodingDeflate})
		require.NoError(t, err)
		assert.Equal(t, metrics[0].Metric, "datadog.trace_agent.started")
		assert.Equal(t, metrics[0].Host, "COMP-WY4M717J6J")
		assert.Equal(t, metrics[0].Points[0][0].(float64), float64(1697177070))
	})
}

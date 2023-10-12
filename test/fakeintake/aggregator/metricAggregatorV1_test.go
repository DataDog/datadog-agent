// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
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
		assert.Nil(t, metrics)
	})
	/*t.Run("parseMetricSeries valid body should parse metrics", func(t *testing.T) {
		metrics, err := ParseV1MetricSeries(api.Payload{Data: metricsDatav1, Encoding: encodingDeflate})
		assert.NoError(t, err)
		assert.Equal(t, len(metrics), 2)

	})*/
}

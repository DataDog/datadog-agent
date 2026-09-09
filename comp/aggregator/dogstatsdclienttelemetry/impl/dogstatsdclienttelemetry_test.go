// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package dogstatsdclienttelemetryimpl

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestComponentObservesClientByteRateSeries(t *testing.T) {
	telemetry := telemetrymock.New(t)
	provides := NewComponent(Requires{Telemetry: telemetry})

	for _, test := range []struct {
		name     string
		value    float64
		expected float64
	}{
		{name: "datadog.dogstatsd.client.bytes_sent", value: 0.7, expected: 7},
		{name: "datadog.dogstatsd.client.bytes_dropped", value: 0.3, expected: 3},
		{name: "datadog.dogstatsd.client.bytes_dropped_queue", value: 0.5, expected: 5},
		{name: "datadog.dogstatsd.client.bytes_dropped_writer", value: 0.2, expected: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			provides.Observer.ObserveFinalDogStatsDSerie(&metrics.Serie{
				Name:     test.name,
				MType:    metrics.APIRateType,
				Interval: 10,
				Points:   []metrics.Point{{Value: test.value}},
				Tags:     tagset.CompositeTagsFromSlice([]string{"client:go", "client_transport:uds"}),
			})

			metrics, err := telemetry.GetCountMetric("dogstatsd_client", test.name[len("datadog.dogstatsd.client."):])
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			require.Equal(t, map[string]string{"client": "go", "client_transport": "uds"}, metrics[0].Tags())
			require.Equal(t, test.expected, metrics[0].Value())
		})
	}
}

func TestComponentSumsRatePointsInFinalSeries(t *testing.T) {
	telemetry := telemetrymock.New(t)
	provides := NewComponent(Requires{Telemetry: telemetry})

	provides.Observer.ObserveFinalDogStatsDSerie(&metrics.Serie{
		Name:     dogStatsDClientBytesSentMetric,
		MType:    metrics.APIRateType,
		Interval: 10,
		Points: []metrics.Point{
			{Value: 0.7},
			{Value: 0.3},
		},
	})

	metrics, err := telemetry.GetCountMetric("dogstatsd_client", "bytes_sent")
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Equal(t, 10.0, metrics[0].Value())
}

func TestComponentPreservesFractionalRecoveredByteTotal(t *testing.T) {
	telemetry := telemetrymock.New(t)
	provides := NewComponent(Requires{Telemetry: telemetry})

	provides.Observer.ObserveFinalDogStatsDSerie(&metrics.Serie{
		Name:     dogStatsDClientBytesSentMetric,
		MType:    metrics.APIRateType,
		Interval: 10,
		Points:   []metrics.Point{{Value: 0.75}},
	})

	metrics, err := telemetry.GetCountMetric("dogstatsd_client", "bytes_sent")
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Equal(t, 7.5, metrics[0].Value())
}

func TestComponentIgnoresUnsupportedOrInvalidSeries(t *testing.T) {
	telemetry := telemetrymock.New(t)
	provides := NewComponent(Requires{Telemetry: telemetry})

	provides.Observer.ObserveFinalDogStatsDSerie(&metrics.Serie{
		Name:     "datadog.dogstatsd.client.bytes_sent",
		MType:    metrics.APIRateType,
		Interval: 10,
		Points:   []metrics.Point{{Value: 1}},
	})

	for _, serie := range []*metrics.Serie{
		{
			Name:     "datadog.dogstatsd.client.metrics",
			MType:    metrics.APIRateType,
			Interval: 10,
			Points:   []metrics.Point{{Value: 7}},
		},
		{
			Name:     "datadog.dogstatsd.client.bytes_sent",
			MType:    metrics.APIGaugeType,
			Interval: 10,
			Points:   []metrics.Point{{Value: 7}},
		},
		{
			Name:     "datadog.dogstatsd.client.bytes_sent",
			MType:    metrics.APIRateType,
			Interval: 10,
			Points: []metrics.Point{
				{Value: -0.7},
				{Value: math.NaN()},
				{Value: math.Inf(1)},
				{Value: math.Ldexp(1, 64) / 10},
				{Value: 1e20},
			},
		},
	} {
		provides.Observer.ObserveFinalDogStatsDSerie(serie)
	}

	metrics, err := telemetry.GetCountMetric("dogstatsd_client", "bytes_sent")
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Equal(t, 10.0, metrics[0].Value())
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package metrics

import (
	"sort"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/require"
)

func TestFlushAndClearSingleContextMetrics(t *testing.T) {
	c := setupConfig(t)

	metrics1 := MakeContextMetrics()
	addMetricSample(metrics1, 100, 1, c)
	addMetricSample(metrics1, 200, 2, c)

	flusher := NewContextMetricsFlusher()
	flusher.Append(0, metrics1)

	require := require.New(t)
	seriesCollection := flushAndClear(require, flusher)

	require.Len(seriesCollection, 2)
	requireSerie(require, seriesCollection[0], 100, 1)
	requireSerie(require, seriesCollection[1], 200, 2)
}

func TestFlushAndClear(t *testing.T) {
	c := setupConfig(t)

	metrics1 := MakeContextMetrics()
	addMetricSample(metrics1, 100, 1, c)
	addMetricSample(metrics1, 200, 2, c)

	metrics2 := MakeContextMetrics()
	addMetricSample(metrics2, 300, 3, c)
	addMetricSample(metrics2, 200, 4, c)

	metrics3 := MakeContextMetrics()
	addMetricSample(metrics3, 300, 5, c)
	addMetricSample(metrics3, 200, 6, c)
	addMetricSample(metrics3, 400, 7, c)

	flusher := NewContextMetricsFlusher()
	flusher.Append(0, metrics1)
	flusher.Append(0, metrics2)
	flusher.Append(0, metrics3)

	require := require.New(t)
	seriesCollection := flushAndClear(require, flusher)
	require.Len(seriesCollection, 4)
	requireSerie(require, seriesCollection[0], 100, 1)
	requireSerie(require, seriesCollection[1], 200, 2, 4, 6)
	requireSerie(require, seriesCollection[2], 300, 3, 5)
	requireSerie(require, seriesCollection[3], 400, 7)
}

func TestFlushSerieRowFragmentsAndClear(t *testing.T) {
	c := setupConfig(t)

	metrics1 := MakeContextMetrics()
	addMetricSample(metrics1, 100, 1, c)
	addMetricSample(metrics1, 200, 2, c)

	metrics2 := MakeContextMetrics()
	addMetricSample(metrics2, 300, 3, c)
	addMetricSample(metrics2, 200, 4, c)

	metrics3 := MakeContextMetrics()
	addMetricSample(metrics3, 300, 5, c)
	addMetricSample(metrics3, 200, 6, c)
	addMetricSample(metrics3, 400, 7, c)

	flusher := NewContextMetricsFlusher()
	flusher.Append(0, metrics1)
	flusher.Append(0, metrics2)
	flusher.Append(0, metrics3)

	require := require.New(t)
	rowsCollection := flushSerieRowFragmentsAndClear(require, flusher)
	require.Len(rowsCollection, 4)
	requireSerieRowFragments(require, rowsCollection[0], 100, 1)
	requireSerieRowFragments(require, rowsCollection[1], 200, 2, 4, 6)
	requireSerieRowFragments(require, rowsCollection[2], 300, 3, 5)
	requireSerieRowFragments(require, rowsCollection[3], 400, 7)
}

func requireSerie(require *require.Assertions, series []*Serie, contextKey ckey.ContextKey, expectedValues ...float64) {
	require.Len(series, len(expectedValues))
	for i, serie := range series {
		require.Equal(contextKey, serie.ContextKey)
		require.Len(serie.Points, 1)
		require.Equal(expectedValues[i], serie.Points[0].Value)
	}
}

func addMetricSample(contextMetrics ContextMetrics, contextKey int, value float64, c model.Config) {
	mSample := MetricSample{
		Value: value,
		Mtype: GaugeType,
	}
	contextMetrics.AddSample(ckey.ContextKey(contextKey), &mSample, 1, 10, nil, c)
}

func flushAndClear(require *require.Assertions, flusher *ContextMetricsFlusher) [][]*Serie {
	var seriesCollection [][]*Serie
	flusher.FlushAndClear(func(s []*Serie) {
		// Clone `s` as it is reused at each call
		series := make([]*Serie, len(s))
		copy(series, s)
		seriesCollection = append(seriesCollection, series)
	})

	// Sort as the order depensd on the map order which is undefined
	sort.Slice(seriesCollection, func(i, j int) bool {
		require.Greater(len(seriesCollection[i]), 0)
		require.Greater(len(seriesCollection[j]), 0)
		return seriesCollection[i][0].ContextKey < seriesCollection[j][0].ContextKey
	})
	return seriesCollection
}

type keyedSerieRowFragments struct {
	contextKey ckey.ContextKey
	rows       []SerieRowFragment
}

func requireSerieRowFragments(require *require.Assertions, keyed keyedSerieRowFragments, contextKey ckey.ContextKey, expectedValues ...float64) {
	require.Equal(contextKey, keyed.contextKey)
	require.Len(keyed.rows, len(expectedValues))
	for i, row := range keyed.rows {
		require.Len(row.Points, 1)
		require.Equal(expectedValues[i], row.Points[0].Value)
		require.Equal(APIGaugeType, row.MType)
	}
}

func flushSerieRowFragmentsAndClear(require *require.Assertions, flusher *ContextMetricsFlusher) []keyedSerieRowFragments {
	var rowsCollection []keyedSerieRowFragments
	flusher.FlushSerieRowFragmentsAndClear(func(contextKey ckey.ContextKey, rows []SerieRowFragment) {
		// Clone rows and their point slices as both are reused at each call.
		clonedRows := make([]SerieRowFragment, len(rows))
		for i, row := range rows {
			clonedRows[i] = row
			clonedRows[i].Points = append([]Point(nil), row.Points...)
		}
		rowsCollection = append(rowsCollection, keyedSerieRowFragments{contextKey: contextKey, rows: clonedRows})
	})

	// Sort as the order depensd on the map order which is undefined
	sort.Slice(rowsCollection, func(i, j int) bool {
		require.Greater(len(rowsCollection[i].rows), 0)
		require.Greater(len(rowsCollection[j].rows), 0)
		return rowsCollection[i].contextKey < rowsCollection[j].contextKey
	})
	return rowsCollection
}

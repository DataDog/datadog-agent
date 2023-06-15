// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package externalmetrics

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"

	"github.com/stretchr/testify/assert"
)

// NewDatadogMetricForTests creates a new internal metric for tests.
func NewDatadogMetricForTests(id, query string, maxAge, timeWindow time.Duration) model.DatadogMetricInternal {
	metric := model.NewDatadogMetricInternalFromExternalMetric(id, query, id, "")
	metric.MaxAge = maxAge
	metric.TimeWindow = timeWindow
	return metric
}

type mockedProcessor struct {
	points          map[string]autoscalers.Point
	err             []error
	errIndex        int
	extQueryCounter int64
}

func (p *mockedProcessor) UpdateExternalMetrics(emList map[string]custommetrics.ExternalMetricValue) map[string]custommetrics.ExternalMetricValue {
	return nil
}

func (p *mockedProcessor) QueryExternalMetric(queries []string, timeWindow time.Duration) (map[string]autoscalers.Point, error) {
	p.extQueryCounter++

	if p.errIndex == len(p.err)-1 {
		return p.points, p.err[p.errIndex]
	} else {
		p.errIndex++
		return p.points, p.err[p.errIndex]
	}
}

func (p *mockedProcessor) ProcessEMList(emList []custommetrics.ExternalMetricValue) map[string]custommetrics.ExternalMetricValue {
	return nil
}

type ddmWithQuery struct {
	ddm   model.DatadogMetricInternal
	query string
}

type metricsFixture struct {
	desc          string
	maxAge        int64
	storeContent  []ddmWithQuery
	queryResults  map[string]autoscalers.Point
	queryError    []error
	expected      []ddmWithQuery
	extQueryCount int64
}

func (f *metricsFixture) run(t *testing.T, testTime time.Time) {
	t.Helper()

	// Create and fill store
	store := NewDatadogMetricsInternalStore()
	for _, datadogMetric := range f.storeContent {
		datadogMetric.ddm.SetQueries(datadogMetric.query)
		store.Set(datadogMetric.ddm.ID, datadogMetric.ddm, "utest")
	}

	// Create MetricsRetriever
	mockedProcessor := mockedProcessor{
		points:          f.queryResults,
		err:             f.queryError,
		errIndex:        0,
		extQueryCounter: 0,
	}
	metricsRetriever, err := NewMetricsRetriever(0, f.maxAge, &mockedProcessor, getIsLeaderFunction(true), &store)
	assert.Nil(t, err)
	metricsRetriever.retrieveMetricsValues()

	for _, expectedDatadogMetric := range f.expected {
		expectedDatadogMetric.ddm.SetQueries(expectedDatadogMetric.query)
		datadogMetric := store.Get(expectedDatadogMetric.ddm.ID)

		// Update time will be set to a value (as metricsRetriever uses time.Now()) that should be > testTime
		// Thus, aligning updateTime to have a working comparison
		if datadogMetric != nil && datadogMetric.Active {
			assert.Condition(t, func() bool { return datadogMetric.UpdateTime.After(expectedDatadogMetric.ddm.UpdateTime) })

			alignedTime := time.Now().UTC()
			expectedDatadogMetric.ddm.UpdateTime = alignedTime
			datadogMetric.UpdateTime = alignedTime
		}

		assert.Equal(t, &expectedDatadogMetric.ddm, datadogMetric)
		assert.Equal(t, f.extQueryCount, mockedProcessor.extQueryCounter)
	}
}

func TestRetrieveMetricsBasic(t *testing.T) {
	// At the end we'll check that update time has been updated, giving 10s to run the tests
	// We truncate down to the second as that's the granularity we have from backend
	defaultTestTime := time.Now().Add(time.Duration(-1) * time.Second).UTC().Truncate(time.Second)
	defaultPreviousUpdateTime := time.Now().Add(time.Duration(-11) * time.Second).UTC().Truncate(time.Second)

	fixtures := []metricsFixture{
		{
			maxAge:        30,
			desc:          "Test nominal case - no errors while retrieving metric values",
			extQueryCount: 1,
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    nil,
					},
					query: "query-metric1",
				},
			},
			queryResults: map[string]autoscalers.Point{
				"query-metric0": {
					Value:     10.0,
					Timestamp: defaultTestTime.Unix(),
					Valid:     true,
				},
				"query-metric1": {
					Value:     11.0,
					Timestamp: defaultTestTime.Unix(),
					Valid:     true,
				},
			},
			queryError: []error{nil},
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    10.0,
						DataTime: defaultTestTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						Value:    11.0,
						DataTime: defaultTestTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric1",
				},
			},
		},
	}

	for i, fixture := range fixtures {
		t.Run(fmt.Sprintf("#%d %s", i, fixture.desc), func(t *testing.T) {
			fixture.run(t, defaultTestTime)
		})
	}
}

func TestRetrieveMetricsErrorCases(t *testing.T) {
	// At the end we'll check that update time has been updated, giving 10s to run the tests
	// We truncate down to the second as that's the granularity we have from backend
	defaultTestTime := time.Now().Add(time.Duration(-1) * time.Second).UTC().Truncate(time.Second)
	defaultPreviousUpdateTime := time.Now().Add(time.Duration(-11) * time.Second).UTC().Truncate(time.Second)

	fixtures := []metricsFixture{
		{
			maxAge:        5,
			desc:          "Test expired data from backend",
			extQueryCount: 1,
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    nil,
					},
					query: "query-metric1",
				},
			},
			queryResults: map[string]autoscalers.Point{
				"query-metric0": {
					Value:     10.0,
					Timestamp: defaultTestTime.Unix(),
					Valid:     true,
				},
				"query-metric1": {
					Value:     11.0,
					Timestamp: defaultPreviousUpdateTime.Unix(),
					Valid:     true,
				},
			},
			queryError: []error{nil},
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    10.0,
						DataTime: defaultTestTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						Value:    11.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    fmt.Errorf(invalidMetricOutdatedErrorMessage, "query-metric1"),
					},
					query: "query-metric1",
				},
			},
		},
		{
			maxAge:        15,
			desc:          "Test expired data from backend defining per-metric maxAge (overrides global maxAge)",
			extQueryCount: 1,
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
						MaxAge:   20 * time.Second,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    nil,
						MaxAge:   5 * time.Second,
					},
					query: "query-metric1",
				},
			},
			queryResults: map[string]autoscalers.Point{
				"query-metric0": {
					Value:     10.0,
					Timestamp: defaultTestTime.Unix(),
					Valid:     true,
				},
				"query-metric1": {
					Value:     11.0,
					Timestamp: defaultPreviousUpdateTime.Unix(),
					Valid:     true,
				},
			},
			queryError: []error{nil},
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    10.0,
						DataTime: defaultTestTime,
						Valid:    true,
						Error:    nil,
						MaxAge:   20 * time.Second,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						Value:    11.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    fmt.Errorf(invalidMetricOutdatedErrorMessage, "query-metric1"),
						MaxAge:   5 * time.Second,
					},
					query: "query-metric1",
				},
			},
		},
		{
			maxAge:        30,
			desc:          "Test backend error (single metric)",
			extQueryCount: 1,
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    8.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						Value:    11.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric1",
				},
			},
			queryResults: map[string]autoscalers.Point{
				"query-metric0": {
					Value:     10.0,
					Timestamp: defaultTestTime.Unix(),
					Valid:     true,
				},
				"query-metric1": {
					Value:     0,
					Timestamp: defaultPreviousUpdateTime.Unix(),
					Valid:     false,
					Error:     errors.New("some err"),
				},
			},
			queryError: []error{nil},
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    10.0,
						DataTime: defaultTestTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						Value:    11.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    fmt.Errorf(invalidMetricErrorMessage, errors.New("some err"), "query-metric1"),
					},
					query: "query-metric1",
				},
			},
		},
		{
			maxAge:        30,
			desc:          "Test global error from backend",
			extQueryCount: 1,
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    1.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						Value:    2.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    nil,
					},
					query: "query-metric1",
				},
			},
			queryResults: map[string]autoscalers.Point{},
			queryError:   []error{fmt.Errorf("Backend error 500")},
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    1.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    fmt.Errorf(invalidMetricGlobalErrorMessage),
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						Value:    2.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    fmt.Errorf(invalidMetricGlobalErrorMessage),
					},
					query: "query-metric1",
				},
			},
		},
		{
			maxAge:        30,
			desc:          "Test missing query response from backend",
			extQueryCount: 1,
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    1.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						Value:    2.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    nil,
					},
					query: "query-metric1",
				},
			},
			queryResults: map[string]autoscalers.Point{
				"query-metric0": {
					Value:     10.0,
					Timestamp: defaultTestTime.Unix(),
					Valid:     true,
				},
			},
			queryError: []error{fmt.Errorf("Backend error 500")},
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    10.0,
						DataTime: defaultTestTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						Value:    2.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    fmt.Errorf(invalidMetricNotFoundErrorMessage, "query-metric1"),
					},
					query: "query-metric1",
				},
			},
		},
	}

	for i, fixture := range fixtures {
		t.Run(fmt.Sprintf("#%d %s", i, fixture.desc), func(t *testing.T) {
			fixture.run(t, defaultTestTime)
		})
	}
}

func TestRetrieveMetricsNotActive(t *testing.T) {
	// At the end we'll check that update time has been updated, giving 10s to run the tests
	// We truncate down to the second as that's the granularity we have from backend
	defaultTestTime := time.Now().Add(time.Duration(-1) * time.Second).UTC().Truncate(time.Second)
	defaultPreviousUpdateTime := time.Now().Add(time.Duration(-11) * time.Second).UTC().Truncate(time.Second)

	fixtures := []metricsFixture{
		{
			maxAge:        30,
			desc:          "Test some metrics are not active",
			extQueryCount: 1,
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   false,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    nil,
					},
					query: "query-metric1",
				},
			},
			queryResults: map[string]autoscalers.Point{
				"query-metric0": {
					Value:     10.0,
					Timestamp: defaultTestTime.Unix(),
					Valid:     true,
				},
				"query-metric1": {
					Value:     11.0,
					Timestamp: defaultTestTime.Unix(),
					Valid:     true,
				},
			},
			queryError: []error{nil},
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    10.0,
						DataTime: defaultTestTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   false,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    nil,
					},
					query: "query-metric1",
				},
			},
		},
		{
			maxAge:        30,
			desc:          "Test no active metrics",
			extQueryCount: 0,
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   false,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   false,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    nil,
					},
					query: "query-metric1",
				},
			},
			queryResults: map[string]autoscalers.Point{
				"query-metric0": {
					Value:     10.0,
					Timestamp: defaultTestTime.Unix(),
					Valid:     true,
				},
				"query-metric1": {
					Value:     11.0,
					Timestamp: defaultTestTime.Unix(),
					Valid:     true,
				},
			},
			queryError: []error{nil},
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   false,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   false,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    nil,
					},
					query: "query-metric1",
				},
			},
		},
	}

	for i, fixture := range fixtures {
		t.Run(fmt.Sprintf("#%d %s", i, fixture.desc), func(t *testing.T) {
			fixture.run(t, defaultTestTime)
		})
	}
}

func TestGetUniqueQueriesByTimeWindow(t *testing.T) {
	metrics := []model.DatadogMetricInternal{
		NewDatadogMetricForTests("1", "system.cpu", time.Minute*1, time.Hour*2),
		NewDatadogMetricForTests("2", "system.cpu", time.Minute*1, time.Hour*2),
		NewDatadogMetricForTests("3", "system.mem", time.Minute*1, time.Hour*2),
		NewDatadogMetricForTests("4", "system.mem", time.Minute*1, time.Minute*2),
		NewDatadogMetricForTests("5", "system.mem", time.Minute*1, time.Minute*1),
		NewDatadogMetricForTests("6", "system.network", time.Minute*1, time.Minute*1),
		NewDatadogMetricForTests("7", "system.disk", time.Minute*1, 0),
	}
	metricsByTimeWindow := getBatchedQueriesByTimeWindow(metrics)
	expected := map[time.Duration][]string{
		// These have a longer than default time window
		time.Hour * 2: {"system.cpu", "system.mem"},
		// These do not.
		autoscalers.GetDefaultTimeWindow(): {"system.mem", "system.network", "system.disk"},
	}

	assert.Equal(t, expected, metricsByTimeWindow)
}

func TestRetrieveMetricsBatchErrorCases(t *testing.T) {
	// At the end we'll check that update time has been updated, giving 10s to run the tests
	// We truncate down to the second as that's the granularity we have from backend
	defaultTestTime := time.Now().Add(time.Duration(-1) * time.Second).UTC().Truncate(time.Second)
	defaultPreviousUpdateTime := time.Now().Add(time.Duration(-11) * time.Second).UTC().Truncate(time.Second)

	fixtures := []metricsFixture{
		{
			maxAge:        30,
			desc:          "Test split batch, error recovers",
			extQueryCount: 2,
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    1.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						Value:    2.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    fmt.Errorf("Backend error 400"),
					},
					query: "query-metric1",
				},
			},
			queryResults: map[string]autoscalers.Point{
				"query-metric0": {
					Value:     10.0,
					Timestamp: defaultTestTime.Unix(),
					Valid:     true,
				},
				"query-metric1": {
					Value:     20.0,
					Timestamp: defaultTestTime.Unix(),
					Valid:     true,
					Error:     nil,
				},
			},
			queryError: []error{nil},
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    10.0,
						DataTime: defaultTestTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						Value:    20.0,
						DataTime: defaultTestTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric1",
				},
			},
		},
		{
			maxAge:        30,
			desc:          "Test split batch, error persists",
			extQueryCount: 2,
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    1.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						Value:    2.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    fmt.Errorf("Backend error 400"),
					},
					query: "query-metric1",
				},
			},
			queryResults: map[string]autoscalers.Point{
				"query-metric0": {
					Value:     10.0,
					Timestamp: defaultTestTime.Unix(),
					Valid:     true,
				},
				"query-metric1": {
					Value:     20.0,
					Timestamp: defaultPreviousUpdateTime.Unix(),
					Valid:     false,
					Error:     errors.New("some err"),
				},
			},
			queryError: []error{nil},
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    10.0,
						DataTime: defaultTestTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						Value:    2.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    errors.New("some err, query was: query-metric1"),
					},
					query: "query-metric1",
				},
			},
		},
		{
			maxAge:        30,
			desc:          "Test 3 batches one with good, two for error metrics",
			extQueryCount: 3,
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    1.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						Value:    2.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    fmt.Errorf("Backend error 500"),
					},
					query: "query-metric1",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric2",
						Active:   true,
						Value:    3.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    fmt.Errorf("Backend error 500"),
					},
					query: "query-metric2",
				},
			},
			queryResults: map[string]autoscalers.Point{
				"query-metric0": {
					Value:     10.0,
					Timestamp: defaultTestTime.Unix(),
					Valid:     true,
				},
				"query-metric1": {
					Value:     20.0,
					Timestamp: defaultPreviousUpdateTime.Unix(),
					Valid:     false,
					Error:     errors.New("some err"),
				},
				"query-metric2": {
					Value:     30.0,
					Timestamp: defaultPreviousUpdateTime.Unix(),
					Valid:     false,
					Error:     errors.New("some other err"),
				},
			},
			queryError: []error{nil, fmt.Errorf("Backend error 500")},
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    10.0,
						DataTime: defaultTestTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						Value:    2.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    errors.New("some err, query was: query-metric1"),
					},
					query: "query-metric1",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric2",
						Active:   true,
						Value:    3.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    errors.New("some other err, query was: query-metric2"),
					},
					query: "query-metric2",
				},
			},
		},
		{
			maxAge:        30,
			desc:          "Test 2 batches, one with error, other with two good metrics",
			extQueryCount: 3,
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    1.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						Value:    2.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    fmt.Errorf("Backend error 500"),
					},
					query: "query-metric1",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric2",
						Active:   true,
						Value:    3.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric2",
				},
			},
			queryResults: map[string]autoscalers.Point{
				"query-metric0": {
					Value:     10.0,
					Timestamp: defaultTestTime.Unix(),
					Valid:     true,
				},
				"query-metric1": {
					Value:     20.0,
					Timestamp: defaultPreviousUpdateTime.Unix(),
					Valid:     false,
					Error:     errors.New("some err"),
				},
				"query-metric2": {
					Value:     30.0,
					Timestamp: defaultTestTime.Unix(),
					Valid:     true,
					Error:     nil,
				},
			},
			queryError: []error{fmt.Errorf("Backend error 500")},
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    10.0,
						DataTime: defaultTestTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric1",
						Active:   true,
						Value:    2.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    false,
						Error:    errors.New("some err, query was: query-metric1"),
					},
					query: "query-metric1",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric2",
						Active:   true,
						Value:    30.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
					},
					query: "query-metric2",
				},
			},
		},
	}

	for i, fixture := range fixtures {
		t.Run(fmt.Sprintf("#%d %s", i, fixture.desc), func(t *testing.T) {
			if fixture.desc == "Test split batch, global error in 2nd batch" {
				fixture.run(t, defaultTestTime)
			}
		})
	}
}

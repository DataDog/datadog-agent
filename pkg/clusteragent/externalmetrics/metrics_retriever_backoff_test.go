// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package externalmetrics

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"

	"github.com/stretchr/testify/assert"
)

type mockedProcessorWithBackoff struct {
	points          map[string]autoscalers.Point
	err             []error
	errIndex        int
	extQueryCounter int64
	queryCapture    [][]string
}

func (p *mockedProcessorWithBackoff) UpdateExternalMetrics(emList map[string]custommetrics.ExternalMetricValue) map[string]custommetrics.ExternalMetricValue {
	return nil
}

func (p *mockedProcessorWithBackoff) QueryExternalMetric(queries []string, timeWindow time.Duration) (map[string]autoscalers.Point, error) {
	p.extQueryCounter++
	// Sort for slice comparison
	sort.Strings(queries)
	p.queryCapture = append(p.queryCapture, queries)
	// Sort slices by first element, slices should be disjoint
	sort.Slice(p.queryCapture, func(i, j int) bool {
		return p.queryCapture[i][0] < p.queryCapture[j][0]
	})

	if p.errIndex == len(p.err)-1 {
		return p.points, p.err[p.errIndex]
	}

	p.errIndex++
	return p.points, p.err[p.errIndex]
}

func (p *mockedProcessorWithBackoff) ProcessEMList(emList []custommetrics.ExternalMetricValue) map[string]custommetrics.ExternalMetricValue {
	return nil
}

type metricsFixtureWithBackoff struct {
	desc            string
	maxAge          int64
	storeContent    []ddmWithQuery
	queryResults    map[string]autoscalers.Point
	queryError      []error
	expected        []ddmWithQuery
	extQueryCount   int64
	extQueryBatches [][]string
}

func (f *metricsFixtureWithBackoff) runWithBackoff(t *testing.T, testTime time.Time) {
	t.Helper()

	// Create and fill store
	store := NewDatadogMetricsInternalStore()
	for _, datadogMetric := range f.storeContent {
		datadogMetric.ddm.SetQueries(datadogMetric.query)
		store.Set(datadogMetric.ddm.ID, datadogMetric.ddm, "utest")
	}

	// Create MetricsRetriever
	mockedProcessor := mockedProcessorWithBackoff{
		points:          f.queryResults,
		err:             f.queryError,
		errIndex:        0,
		extQueryCounter: 0,
	}
	metricsRetriever, err := NewMetricsRetriever(0, f.maxAge, &mockedProcessor, getIsLeaderFunction(true), &store, true)
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

			// These will contain random element if Retries > 0
			if expectedDatadogMetric.ddm.Retries > 0 {
				expectedDatadogMetric.ddm.RetryAfter = datadogMetric.RetryAfter
				// Align errors and verify prefix is expected
				expectedDatadogMetric.ddm.Error = datadogMetric.Error
				assert.True(t, strings.HasPrefix(datadogMetric.Error.Error(), expectedDatadogMetric.ddm.Error.Error()))
			}
		}

		assert.Equal(t, &expectedDatadogMetric.ddm, datadogMetric)
		assert.Equal(t, f.extQueryCount, mockedProcessor.extQueryCounter)

		// Skip this assert, when not set, i.e. test doesn't verify actual queries
		if len(f.extQueryBatches) > 0 {
			assert.Equal(t, f.extQueryBatches, mockedProcessor.queryCapture)
		}
	}
}

func (f *metricsFixtureWithBackoff) runQueryOnly(t *testing.T) {
	t.Helper()

	// Create and fill store
	store := NewDatadogMetricsInternalStore()
	for _, datadogMetric := range f.storeContent {
		datadogMetric.ddm.SetQueries(datadogMetric.query)
		store.Set(datadogMetric.ddm.ID, datadogMetric.ddm, "utest")
	}

	// Create MetricsRetriever
	mockedProcessor := mockedProcessorWithBackoff{
		points:          f.queryResults,
		err:             f.queryError,
		errIndex:        0,
		extQueryCounter: 0,
	}
	metricsRetriever, err := NewMetricsRetriever(0, f.maxAge, &mockedProcessor, getIsLeaderFunction(true), &store, true)
	assert.Nil(t, err)
	metricsRetriever.retrieveMetricsValues()
	assert.Equal(t, f.extQueryCount, mockedProcessor.extQueryCounter)
	assert.Equal(t, f.extQueryBatches, mockedProcessor.queryCapture)
}

func TestRetrieveMetricsBasicWithBackoff(t *testing.T) {
	// At the end we'll check that update time has been updated, giving 10s to run the tests
	// We truncate down to the second as that's the granularity we have from backend
	defaultTestTime := time.Now().Add(time.Duration(-1) * time.Second).UTC().Truncate(time.Second)
	defaultPreviousUpdateTime := time.Now().Add(time.Duration(-11) * time.Second).UTC().Truncate(time.Second)

	fixtures := []metricsFixtureWithBackoff{
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
			fixture.runWithBackoff(t, defaultTestTime)
		})
	}
}

func TestRetrieveMetricsErrorCasesWithBackoff(t *testing.T) {
	// At the end we'll check that update time has been updated, giving 10s to run the tests
	// We truncate down to the second as that's the granularity we have from backend
	defaultTestTime := time.Now().Add(time.Duration(-1) * time.Second).UTC().Truncate(time.Second)
	defaultPreviousUpdateTime := time.Now().Add(time.Duration(-11) * time.Second).UTC().Truncate(time.Second)

	fixtures := []metricsFixtureWithBackoff{
		{
			maxAge:        5,
			desc:          "Test expired data from backend, don't set Retries",
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
						Retries:  0,
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
						Retries:  0,
					},
					query: "query-metric1",
				},
			},
		},
		{
			maxAge:        15,
			desc:          "Test expired data from backend defining per-metric maxAge (overrides global maxAge), don't set Retries",
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
						Retries:  0,
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
						Retries:  0,
					},
					query: "query-metric1",
				},
			},
		},
		{
			maxAge:        30,
			desc:          "Test backend error (single metric), set Retries (single metrics)",
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
						Retries:  0,
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
						Error:    fmt.Errorf(invalidMetricErrorWithRetriesMessage, errors.New("some err"), "query-metric1", ""),
						Retries:  1,
					},
					query: "query-metric1",
				},
			},
		},
		{
			maxAge:        30,
			desc:          "Test global error from backend, set Retries (all)",
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
						Error:    fmt.Errorf(invalidMetricGlobalErrorWithRetriesMessage, 2, ""),
						Retries:  1,
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
						Error:    fmt.Errorf(invalidMetricGlobalErrorWithRetriesMessage, 2, ""),
						Retries:  1,
					},
					query: "query-metric1",
				},
			},
		},
		{
			maxAge:        30,
			desc:          "Test missing query response from backend, don't set Retries",
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
						Retries:  0,
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
						Retries:  0,
					},
					query: "query-metric1",
				},
			},
		},
	}

	for i, fixture := range fixtures {
		t.Run(fmt.Sprintf("#%d %s", i, fixture.desc), func(t *testing.T) {
			// if fixture.desc == "Test global error from backend, set Retries (all)" {
			fixture.runWithBackoff(t, defaultTestTime)
			// }
		})
	}
}

func TestRetrieveMetricsNotActiveWithBackoff(t *testing.T) {
	// At the end we'll check that update time has been updated, giving 10s to run the tests
	// We truncate down to the second as that's the granularity we have from backend
	defaultTestTime := time.Now().Add(time.Duration(-1) * time.Second).UTC().Truncate(time.Second)
	defaultPreviousUpdateTime := time.Now().Add(time.Duration(-11) * time.Second).UTC().Truncate(time.Second)

	fixtures := []metricsFixtureWithBackoff{
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
			fixture.runWithBackoff(t, defaultTestTime)
		})
	}
}

func TestGetUniqueQueriesByTimeWindowWithBackoff(t *testing.T) {
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

func TestRetrieveMetricsBatchErrorCasesWithBackoff(t *testing.T) {
	// At the end we'll check that update time has been updated, giving 10s to run the tests
	// We truncate down to the second as that's the granularity we have from backend
	defaultTestTime := time.Now().Add(time.Duration(-1) * time.Second).UTC().Truncate(time.Second)
	defaultPreviousUpdateTime := time.Now().Add(time.Duration(-11) * time.Second).UTC().Truncate(time.Second)

	fixtures := []metricsFixtureWithBackoff{
		{
			maxAge:        30,
			desc:          "Test split batch, error recovers; reset Retries",
			extQueryCount: 2,
			extQueryBatches: [][]string{
				{"query-metric0"},
				{"query-metric1"},
			},
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    1.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
						Retries:  0,
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
						Retries:  1,
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
						Retries:  0,
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
						Retries:  0,
					},
					query: "query-metric1",
				},
			},
		},
		{
			maxAge:        30,
			desc:          "Test split batch, error persists; increase Retries",
			extQueryCount: 2,
			extQueryBatches: [][]string{
				{"query-metric0"},
				{"query-metric1"},
			},

			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    1.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
						Retries:  0,
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
						Retries:  1,
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
						Retries:  0,
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
						Retries:  2,
					},
					query: "query-metric1",
				},
			},
		},
		{
			maxAge:        30,
			desc:          "Test 3 batches one with good, two for error metrics; increase Retries",
			extQueryCount: 3,
			extQueryBatches: [][]string{
				{"query-metric0"},
				{"query-metric1"},
				{"query-metric2"},
			},
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    1.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
						Retries:  0,
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
						Retries:  1,
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
						Retries:  1,
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
						Retries:  0,
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
						Retries:  2,
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
						Retries:  2,
					},
					query: "query-metric2",
				},
			},
		},
		{
			maxAge:        30,
			desc:          "Test 2 batches, one with error, other with two good metrics; increase Retries",
			extQueryCount: 2,
			extQueryBatches: [][]string{
				{"query-metric0", "query-metric2"},
				{"query-metric1"},
			},
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric0",
						Active:   true,
						Value:    1.0,
						DataTime: defaultPreviousUpdateTime,
						Valid:    true,
						Error:    nil,
						Retries:  0,
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
						Retries:  1,
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
						Retries:  0,
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
						Retries:  0,
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
						Retries:  2,
					},
					query: "query-metric1",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:       "metric2",
						Active:   true,
						Value:    30.0,
						DataTime: defaultTestTime,
						Valid:    true,
						Error:    nil,
						Retries:  0,
					},
					query: "query-metric2",
				},
			},
		},
	}

	for i, fixture := range fixtures {
		t.Run(fmt.Sprintf("#%d %s", i, fixture.desc), func(t *testing.T) {
			fixture.runWithBackoff(t, defaultTestTime)
		})
	}
}

func TestRetryIncTimingWithBackoff(t *testing.T) {
	// Current backoff policy in metrics retriever: backoff.NewPolicy(2, 30, 1800, 2, false)
	// when retries > 5,  backoff capped at 1800sec
	// when retries <= 5, backoff random(2^(retries-1) * 30, 2^retries * 30)

	tests := []struct {
		Name                 string
		CurrentRetries       int
		NewRetries           int
		RetryAfterMinFromNow int
		RetryAfterMaxFromNow int
	}{
		{
			Name:                 "0->1",
			CurrentRetries:       0,
			NewRetries:           1,
			RetryAfterMinFromNow: 30,
			RetryAfterMaxFromNow: 60,
		},
		{
			Name:                 "1->2",
			CurrentRetries:       1,
			NewRetries:           2,
			RetryAfterMinFromNow: 60,
			RetryAfterMaxFromNow: 120,
		},
		{
			Name:                 "5->6",
			CurrentRetries:       5,
			NewRetries:           6,
			RetryAfterMinFromNow: 1799,
			RetryAfterMaxFromNow: 1801,
		},
		{
			Name:                 "10->11",
			CurrentRetries:       10,
			NewRetries:           11,
			RetryAfterMinFromNow: 1799,
			RetryAfterMaxFromNow: 1801,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			ddMetricsInternal := model.DatadogMetricInternal{
				Retries: tt.CurrentRetries,
			}
			retryTimeMin := time.Now().Add(time.Duration(tt.RetryAfterMinFromNow) * time.Second)
			retryTimeMax := time.Now().Add(time.Duration(tt.RetryAfterMaxFromNow) * time.Second)
			incrementRetries(&ddMetricsInternal)
			assert.Equal(t, tt.NewRetries, ddMetricsInternal.Retries)
			assert.True(t, ddMetricsInternal.RetryAfter.After(retryTimeMin))
			assert.True(t, ddMetricsInternal.RetryAfter.Before(retryTimeMax))
		})
	}
}

func TestBatchSplittingWithBackoff(t *testing.T) {

	// In this case we only care about how many queries batch queries are made
	// to verify the backoff logic. Backoff timing is tested in the previous test

	fixtures := []metricsFixtureWithBackoff{
		{
			desc:          "Test mixed queries with backoffs, query one with expired backoff; backoff one; query valid",
			extQueryCount: 3,
			extQueryBatches: [][]string{
				{"query-metric0"},
				{"query-metric2"},
				{"query-metric3"},
			},
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:      "metric0",
						Active:  true,
						Error:   nil,
						Retries: 0, // no error, no backoff: +1 query
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric1",
						Active:     true,
						Error:      errors.New("some err"),
						RetryAfter: time.Now().Add(time.Duration(5) * time.Second),
						Retries:    1, // backoff not expired: no change
					},
					query: "query-metric1",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric2",
						Active:     true,
						Error:      errors.New("some err"),
						RetryAfter: time.Now().Add(time.Duration(-5) * time.Second),
						Retries:    1, // backoff expired: +1 query
					},
					query: "query-metric2",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric3",
						Active:     true,
						Error:      errors.New("some err"),
						RetryAfter: time.Now().Add(time.Duration(-5) * time.Second),
						Retries:    2, // backoff expired: +1 query
					},
					query: "query-metric3",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric4",
						Active:     true,
						Error:      errors.New("some err"),
						RetryAfter: time.Now().Add(time.Duration(5) * time.Second),
						Retries:    2, // backoff not expired: no change
					},
					query: "query-metric4",
				},
			},
			queryResults: map[string]autoscalers.Point{
				"query-metric0": {},
				"query-metric1": {},
				"query-metric2": {},
				"query-metric3": {},
				"query-metric4": {},
			},
			queryError: []error{nil},
		},
		{
			desc:          "Test mix with multiple valid metrics, invalid with and without backoff",
			extQueryCount: 2,
			extQueryBatches: [][]string{
				{"query-metric0", "query-metric1", "query-metric2"},
				{"query-metric3"},
			},
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:      "metric0",
						Active:  true,
						Error:   nil,
						Retries: 0, // no error, no backoff: +1 query for valid queries
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:      "metric1",
						Active:  true,
						Error:   nil,
						Retries: 0, // no error, no backoff, same query as metric0
					},
					query: "query-metric1",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:      "metric2",
						Active:  true,
						Error:   nil,
						Retries: 0, // no error, no backoff, same query as metric0
					},
					query: "query-metric2",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric3",
						Active:     true,
						Error:      errors.New("some err"),
						RetryAfter: time.Now().Add(time.Duration(-5) * time.Second),
						Retries:    2, // backoff expired: +1 query
					},
					query: "query-metric3",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric4",
						Active:     true,
						Error:      errors.New("some err"),
						RetryAfter: time.Now().Add(time.Duration(5) * time.Second),
						Retries:    2, // backoff not expired: no change
					},
					query: "query-metric4",
				},
			},
			queryResults: map[string]autoscalers.Point{
				"query-metric0": {},
				"query-metric1": {},
				"query-metric2": {},
				"query-metric3": {},
				"query-metric4": {},
			},
			queryError: []error{nil},
		},
	}

	for i, fixture := range fixtures {
		t.Run(fmt.Sprintf("#%d %s", i, fixture.desc), func(t *testing.T) {
			fixture.runQueryOnly(t)
		})
	}
}

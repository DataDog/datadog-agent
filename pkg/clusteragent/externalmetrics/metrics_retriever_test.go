// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver

package externalmetrics

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"

	"github.com/stretchr/testify/assert"
)

type mockedProcessor struct {
	points map[string]autoscalers.Point
	err    error
}

func (p *mockedProcessor) UpdateExternalMetrics(emList map[string]custommetrics.ExternalMetricValue) map[string]custommetrics.ExternalMetricValue {
	return nil
}

func (p *mockedProcessor) QueryExternalMetric(queries []string) (map[string]autoscalers.Point, error) {
	return p.points, p.err
}

func (p *mockedProcessor) ProcessEMList(emList []custommetrics.ExternalMetricValue) map[string]custommetrics.ExternalMetricValue {
	return nil
}

type ddmWithQuery struct {
	ddm   model.DatadogMetricInternal
	query string
}

type metricsFixture struct {
	desc         string
	maxAge       int64
	storeContent []ddmWithQuery
	queryResults map[string]autoscalers.Point
	queryError   error
	expected     []ddmWithQuery
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
		points: f.queryResults,
		err:    f.queryError,
	}
	metricsRetriever, err := NewMetricsRetriever(0, f.maxAge, &mockedProcessor, getIsLeaderFunction(true), &store)
	assert.Nil(t, err)
	metricsRetriever.retrieveMetricsValues()

	for _, expectedDatadogMetric := range f.expected {
		expectedDatadogMetric.ddm.SetQueries(expectedDatadogMetric.query)
		datadogMetric := store.Get(expectedDatadogMetric.ddm.ID)

		// Update time will be set to a value (as metricsRetriever uses time.Now()) that should be > testTime
		// Thus, aligning updateTime to have a working comparison
		if !expectedDatadogMetric.ddm.Valid && datadogMetric != nil && datadogMetric.Active {
			assert.Condition(t, func() bool { return datadogMetric.UpdateTime.After(expectedDatadogMetric.ddm.UpdateTime) })

			alignedTime := time.Now().UTC()
			expectedDatadogMetric.ddm.UpdateTime = alignedTime
			datadogMetric.UpdateTime = alignedTime
		}

		assert.Equal(t, &expectedDatadogMetric.ddm, datadogMetric)
	}
}

func TestRetrieveMetricsBasic(t *testing.T) {
	// At the end we'll check that update time has been updated, giving 10s to run the tests
	// We truncate down to the second as that's the granularity we have from backend
	defaultTestTime := time.Now().Add(time.Duration(-1) * time.Second).UTC().Truncate(time.Second)
	defaultPreviousUpdateTime := time.Now().Add(time.Duration(-11) * time.Second).UTC().Truncate(time.Second)

	fixtures := []metricsFixture{
		{
			maxAge: 30,
			desc:   "Test nominal case - no errors while retrieving metric values",
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric0",
						Active:     true,
						UpdateTime: defaultPreviousUpdateTime,
						Valid:      true,
						Error:      nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric1",
						Active:     true,
						UpdateTime: defaultPreviousUpdateTime,
						Valid:      false,
						Error:      nil,
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
			queryError: nil,
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric0",
						Active:     true,
						Value:      10.0,
						UpdateTime: defaultTestTime,
						Valid:      true,
						Error:      nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric1",
						Active:     true,
						Value:      11.0,
						UpdateTime: defaultTestTime,
						Valid:      true,
						Error:      nil,
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
			maxAge: 5,
			desc:   "Test expired data from backend",
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric0",
						Active:     true,
						UpdateTime: defaultPreviousUpdateTime,
						Valid:      true,
						Error:      nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric1",
						Active:     true,
						UpdateTime: defaultPreviousUpdateTime,
						Valid:      false,
						Error:      nil,
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
			queryError: nil,
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric0",
						Active:     true,
						Value:      10.0,
						UpdateTime: defaultTestTime,
						Valid:      true,
						Error:      nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:     "metric1",
						Active: true,
						Value:  11.0,
						Valid:  false,
						Error:  fmt.Errorf(invalidMetricOutdatedErrorMessage, "query-metric1"),
						// UpdateTime not set as it will not be compared directly
					},
					query: "query-metric1",
				},
			},
		},
		{
			maxAge: 30,
			desc:   "Test backend error (single metric)",
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric0",
						Active:     true,
						Value:      8.0,
						UpdateTime: defaultPreviousUpdateTime,
						Valid:      true,
						Error:      nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric1",
						Active:     true,
						Value:      11.0,
						UpdateTime: defaultPreviousUpdateTime,
						Valid:      true,
						Error:      nil,
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
				},
			},
			queryError: nil,
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric0",
						Active:     true,
						Value:      10.0,
						UpdateTime: defaultTestTime,
						Valid:      true,
						Error:      nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:     "metric1",
						Active: true,
						Value:  11.0,
						Valid:  false,
						Error:  fmt.Errorf(invalidMetricBackendErrorMessage, "query-metric1"),
						// UpdateTime not set as it will not be compared directly
					},
					query: "query-metric1",
				},
			},
		},
		{
			maxAge: 30,
			desc:   "Test global error from backend",
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric0",
						Active:     true,
						Value:      1.0,
						UpdateTime: defaultPreviousUpdateTime,
						Valid:      true,
						Error:      nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric1",
						Active:     true,
						Value:      2.0,
						UpdateTime: defaultPreviousUpdateTime,
						Valid:      false,
						Error:      nil,
					},
					query: "query-metric1",
				},
			},
			queryResults: map[string]autoscalers.Point{},
			queryError:   fmt.Errorf("Backend error 500"),
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:     "metric0",
						Active: true,
						Value:  1.0,
						Valid:  false,
						Error:  fmt.Errorf(invalidMetricGlobalErrorMessage),
						// UpdateTime not set as it will not be compared directly
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:     "metric1",
						Active: true,
						Value:  2.0,
						Valid:  false,
						Error:  fmt.Errorf(invalidMetricGlobalErrorMessage),
						// UpdateTime not set as it will not be compared directly
					},
					query: "query-metric1",
				},
			},
		},
		{
			maxAge: 30,
			desc:   "Test missing query response from backend",
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric0",
						Active:     true,
						Value:      1.0,
						UpdateTime: defaultPreviousUpdateTime,
						Valid:      true,
						Error:      nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric1",
						Active:     true,
						Value:      2.0,
						UpdateTime: defaultPreviousUpdateTime,
						Valid:      false,
						Error:      nil,
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
			queryError: fmt.Errorf("Backend error 500"),
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric0",
						Active:     true,
						Value:      10.0,
						UpdateTime: defaultTestTime,
						Valid:      true,
						Error:      nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:     "metric1",
						Active: true,
						Value:  2.0,
						Valid:  false,
						Error:  fmt.Errorf(invalidMetricNoDataErrorMessage, "query-metric1"),
						// UpdateTime not set as it will not be compared directly
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
			maxAge: 30,
			desc:   "Test some metrics are not active",
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric0",
						Active:     true,
						UpdateTime: defaultPreviousUpdateTime,
						Valid:      true,
						Error:      nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric1",
						Active:     false,
						UpdateTime: defaultPreviousUpdateTime,
						Valid:      false,
						Error:      nil,
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
			queryError: nil,
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric0",
						Active:     true,
						Value:      10.0,
						UpdateTime: defaultTestTime,
						Valid:      true,
						Error:      nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric1",
						Active:     false,
						UpdateTime: defaultPreviousUpdateTime,
						Valid:      false,
						Error:      nil,
					},
					query: "query-metric1",
				},
			},
		},
		{
			maxAge: 30,
			desc:   "Test no active metrics",
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric0",
						Active:     false,
						UpdateTime: defaultPreviousUpdateTime,
						Valid:      true,
						Error:      nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric1",
						Active:     false,
						UpdateTime: defaultPreviousUpdateTime,
						Valid:      false,
						Error:      nil,
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
			queryError: nil,
			expected: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric0",
						Active:     false,
						UpdateTime: defaultPreviousUpdateTime,
						Valid:      true,
						Error:      nil,
					},
					query: "query-metric0",
				},
				{
					ddm: model.DatadogMetricInternal{
						ID:         "metric1",
						Active:     false,
						UpdateTime: defaultPreviousUpdateTime,
						Valid:      false,
						Error:      nil,
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

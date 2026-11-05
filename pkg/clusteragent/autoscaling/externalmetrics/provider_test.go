// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package externalmetrics

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/externalmetrics/model"

	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/metrics/pkg/apis/external_metrics"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type providerFixture struct {
	desc                       string
	storeContent               []ddmWithQuery
	queryNamespace             string
	queryMetricName            string
	querySelector              map[string]string
	expectedExternalMetrics    []external_metrics.ExternalMetricValue
	expectedExternalMetricInfo []provider.ExternalMetricInfo
	expectedError              error
}

func (f *providerFixture) runGetExternalMetric(t *testing.T, testTime time.Time) {
	t.Helper()

	// Create provider and fill store
	datadogMetricProvider := datadogMetricProvider{
		store:            NewDatadogMetricsInternalStore(),
		autogenNamespace: "default",
	}
	for _, datadogMetric := range f.storeContent {
		datadogMetric.ddm.SetQueries(datadogMetric.query)
		datadogMetricProvider.store.Set(datadogMetric.ddm.ID, datadogMetric.ddm, "utest")
	}

	externalMetrics, err := datadogMetricProvider.getExternalMetric(f.queryNamespace, labels.Set(f.querySelector).AsSelector(), provider.ExternalMetricInfo{Metric: f.queryMetricName}, testTime)
	if err != nil {
		assert.Equal(t, f.expectedError, err)
		assert.Nil(t, externalMetrics)
		return
	}

	require.NotNil(t, externalMetrics)
	assert.ElementsMatch(t, f.expectedExternalMetrics, externalMetrics.Items)
}

func (f *providerFixture) runListAllExternalMetrics(t *testing.T) {
	t.Helper()

	// Create provider and fill store
	datadogMetricProvider := datadogMetricProvider{
		store: NewDatadogMetricsInternalStore(),
	}
	for _, datadogMetric := range f.storeContent {
		datadogMetric.ddm.SetQueries(datadogMetric.query)
		datadogMetricProvider.store.Set(datadogMetric.ddm.ID, datadogMetric.ddm, "utest")
	}

	expectedExternalMetricInfo := datadogMetricProvider.ListAllExternalMetrics()
	assert.ElementsMatch(t, f.expectedExternalMetricInfo, expectedExternalMetricInfo)
}

func TestGetExternalMetrics(t *testing.T) {
	defaultUpdateTime := time.Now().UTC()
	defaultMetaUpdateTime := metav1.NewTime(defaultUpdateTime)

	fixtures := []providerFixture{
		{
			desc: "Test nominal case - DatadogMetric exists and is valid",
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "ns/metric0",
						DataTime: defaultUpdateTime,
						Valid:    true,
						Error:    nil,
						Value:    42.0,
					},
					query: "query-metric0",
				},
			},
			queryMetricName: "datadogmetric@ns:metric0",
			queryNamespace:  "ns",
			expectedExternalMetrics: []external_metrics.ExternalMetricValue{
				{
					MetricName:   "datadogmetric@ns:metric0",
					MetricLabels: nil,
					Timestamp:    defaultMetaUpdateTime,
					Value:        resource.MustParse(fmt.Sprintf("%v", 42.0)),
				},
			},
		},
		{
			desc: "Test nominal case - DatadogMetric reference without namespace",
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "ns/metric0",
						DataTime: defaultUpdateTime,
						Valid:    true,
						Error:    nil,
						Value:    42.0,
					},
					query: "query-metric0",
				},
			},
			queryMetricName: "datadogmetric@metric0",
			queryNamespace:  "ns",
			expectedExternalMetrics: []external_metrics.ExternalMetricValue{
				{
					MetricName:   "datadogmetric@metric0",
					MetricLabels: nil,
					Timestamp:    defaultMetaUpdateTime,
					Value:        resource.MustParse(fmt.Sprintf("%v", 42.0)),
				},
			},
		},
		{
			desc: "Test DatadogMetric is valid but old returns error",
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "ns/metric0",
						DataTime: defaultUpdateTime.Add(-time.Hour),
						Valid:    true,
						Error:    nil,
						Value:    42.0,
					},
					query: "query-metric0",
				},
			},
			queryMetricName:         "datadogmetric@ns:metric0",
			queryNamespace:          "ns",
			expectedExternalMetrics: nil,
			expectedError:           fmt.Errorf("DatadogMetric is stale, last updated: %v. Check datadog-cluster-agent logs for errors", defaultUpdateTime.Add(-time.Hour)),
		},
		{
			desc: "Test DatadogMetric is invalid",
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "ns/metric0",
						DataTime: defaultUpdateTime,
						Valid:    false,
						Error:    errors.New("Some error"),
						Value:    42.0,
					},
					query: "query-metric0",
				},
			},
			queryMetricName:         "datadogmetric@ns:metric0",
			queryNamespace:          "ns",
			expectedExternalMetrics: nil,
			expectedError:           errors.New("Some error"),
		},
		{
			desc: "Test DatadogMetric is invalid, no error",
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "ns/metric0",
						DataTime: defaultUpdateTime,
						Valid:    false,
						Value:    42.0,
					},
					query: "query-metric0",
				},
			},
			queryMetricName:         "datadogmetric@ns:metric0",
			queryNamespace:          "ns",
			expectedExternalMetrics: nil,
			expectedError:           errors.New("DatadogMetric is invalid, missing error details"),
		},
		{
			desc: "Test DatadogMetric not found",
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "ns/metric0",
						DataTime: defaultUpdateTime,
						Valid:    true,
						Error:    nil,
						Value:    42.0,
					},
					query: "query-metric0",
				},
			},
			queryMetricName:         "datadogmetric@ns:metric1",
			queryNamespace:          "ns",
			expectedExternalMetrics: nil,
			expectedError:           errors.New("DatadogMetric not found for metric name: datadogmetric@ns:metric1, datadogmetricid: ns/metric1"),
		},
		{
			desc: "Test DatadogMetric not found in request namespace",
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "ns/metric0",
						DataTime: defaultUpdateTime,
						Valid:    true,
						Error:    nil,
						Value:    42.0,
					},
					query: "query-metric0",
				},
			},
			queryMetricName:         "datadogmetric@ns:metric1",
			queryNamespace:          "ns",
			expectedExternalMetrics: nil,
			expectedError:           errors.New("DatadogMetric not found for metric name: datadogmetric@ns:metric1, datadogmetricid: ns/metric1"),
		},
		{
			desc: "Test DatadogMetric reference namespace is ignored, DatadogMetric is not readable from another namespace",
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "ns/metric0",
						DataTime: defaultUpdateTime,
						Valid:    true,
						Error:    nil,
						Value:    42.0,
					},
					query: "query-metric0",
				},
			},
			queryMetricName:         "datadogmetric@ns:metric0",
			queryNamespace:          "tenant-b",
			expectedExternalMetrics: nil,
			expectedError:           errors.New("DatadogMetric not found for metric name: datadogmetric@ns:metric0, datadogmetricid: tenant-b/metric0"),
		},
		{
			desc: "Test ExternalMetric use wrong DatadogMetric format",
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "ns/metric0",
						DataTime: defaultUpdateTime,
						Valid:    true,
						Error:    nil,
						Value:    42.0,
					},
					query: "query-metric0",
				},
			},
			queryMetricName:         "datadogmetric@metric_1",
			expectedExternalMetrics: nil,
			expectedError:           errors.New("ExternalMetric does not follow DatadogMetric format: datadogmetric@metric_1"),
		},
		{
			desc: "Test ExternalMetric does not use DatadogMetric format",
			storeContent: []ddmWithQuery{
				{
					ddm: model.DatadogMetricInternal{
						ID:       "ns/metric0",
						DataTime: defaultUpdateTime,
						Valid:    true,
						Error:    nil,
						Value:    42.0,
					},
					query: "query-metric0",
				},
			},
			queryMetricName:         "nginx.net.request_per_s",
			expectedExternalMetrics: nil,
			expectedError:           errors.New("DatadogMetric not found for metric name: nginx.net.request_per_s, datadogmetricid: default/dcaautogen-32402d8dfc05cf540928a606d78ed68c0607f7"),
		},
	}

	for i, fixture := range fixtures {
		t.Run(fmt.Sprintf("#%d %s", i, fixture.desc), func(t *testing.T) {
			fixture.runGetExternalMetric(t, defaultUpdateTime)
		})
	}
}

func TestGetExternalMetricErrorStatus(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int32
	}{
		{
			name:       "no series",
			err:        newQueryError("query-metric0", noSeriesFoundErrorFragment+", check Cluster Agent logs for QueryIndex errors", time.Time{}),
			statusCode: http.StatusUnprocessableEntity,
		},
		{
			name:       "only null values",
			err:        newQueryError("query-metric0", onlyNullValuesErrorFragment+" (4 points), check data is available in the last 300 seconds", time.Time{}),
			statusCode: http.StatusUnprocessableEntity,
		},
		{
			name:       "persisted no-data error",
			err:        errors.New(newQueryError("query-metric0", noSeriesFoundErrorFragment, time.Time{}).Error()),
			statusCode: http.StatusUnprocessableEntity,
		},
		{
			name:       "other processing error",
			err:        newQueryError("query-metric0", "multiple series found", time.Time{}),
			statusCode: http.StatusInternalServerError,
		},
		{
			name:       "backend error",
			err:        newBatchError(errors.New("backend unavailable"), time.Time{}),
			statusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metricProvider := datadogMetricProvider{
				store:            NewDatadogMetricsInternalStore(),
				autogenNamespace: "default",
			}
			metricProvider.store.Set("ns/metric0", model.DatadogMetricInternal{
				ID:    "ns/metric0",
				Valid: false,
				Error: tt.err,
			}, "utest")

			res, err := metricProvider.GetExternalMetric(
				context.Background(),
				"ns",
				labels.Everything(),
				provider.ExternalMetricInfo{Metric: "datadogmetric@ns:metric0"},
			)

			require.Nil(t, res)
			var statusErr *apierr.StatusError
			require.ErrorAs(t, err, &statusErr)
			assert.Equal(t, tt.statusCode, statusErr.ErrStatus.Code)
		})
	}
}

func TestListAllExternalMetrics(t *testing.T) {
	fixtures := []providerFixture{
		{
			desc:                       "Test no metrics in store (send fake metric back)",
			storeContent:               []ddmWithQuery{},
			expectedExternalMetricInfo: []provider.ExternalMetricInfo{{Metric: "externalmetrics"}},
		},
	}

	for i, fixture := range fixtures {
		t.Run(fmt.Sprintf("#%d %s", i, fixture.desc), func(t *testing.T) {
			fixture.runListAllExternalMetrics(t)
		})
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package externalmetrics

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics/model"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/metrics/pkg/apis/external_metrics"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type providerFixture struct {
	desc                       string
	storeContent               []model.DatadogMetricInternal
	queryNamespace             string
	queryMetricName            string
	querySelector              map[string]string
	expectedExternalMetrics    []external_metrics.ExternalMetricValue
	expectedExternalMetricInfo []provider.ExternalMetricInfo
	expectedError              error
}

func (f *providerFixture) runGetExternalMetric(t *testing.T) {
	t.Helper()

	// Create provider and fill store
	datadogMetricProvider := datadogMetricProvider{
		store:            NewDatadogMetricsInternalStore(),
		autogenNamespace: "default",
	}
	for _, datadogMetric := range f.storeContent {
		datadogMetricProvider.store.Set(datadogMetric.ID, datadogMetric, "utest")
	}

	externalMetrics, err := datadogMetricProvider.GetExternalMetric(f.queryNamespace, labels.Set(f.querySelector).AsSelector(), provider.ExternalMetricInfo{Metric: f.queryMetricName})
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
		datadogMetricProvider.store.Set(datadogMetric.ID, datadogMetric, "utest")
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
			storeContent: []model.DatadogMetricInternal{
				{
					ID:         "ns/metric0",
					Query:      "query-metric0",
					UpdateTime: defaultUpdateTime,
					Valid:      true,
					Error:      nil,
					Value:      42.0,
				},
			},
			queryMetricName: "datadogmetric@ns:metric0",
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
			desc: "Test DatadogMetric is invalid",
			storeContent: []model.DatadogMetricInternal{
				{
					ID:         "ns/metric0",
					Query:      "query-metric0",
					UpdateTime: defaultUpdateTime,
					Valid:      false,
					Error:      fmt.Errorf("Some error"),
					Value:      42.0,
				},
			},
			queryMetricName:         "datadogmetric@ns:metric0",
			expectedExternalMetrics: nil,
			expectedError:           fmt.Errorf("DatadogMetric is invalid, err: %v", fmt.Errorf("Some error")),
		},
		{
			desc: "Test DatadogMetric not found",
			storeContent: []model.DatadogMetricInternal{
				{
					ID:         "ns/metric0",
					Query:      "query-metric0",
					UpdateTime: defaultUpdateTime,
					Valid:      true,
					Error:      nil,
					Value:      42.0,
				},
			},
			queryMetricName:         "datadogmetric@ns:metric1",
			expectedExternalMetrics: nil,
			expectedError:           fmt.Errorf("DatadogMetric not found for metric name: datadogmetric@ns:metric1, datadogmetricid: ns/metric1"),
		},
		{
			desc: "Test DatadogMetric not found",
			storeContent: []model.DatadogMetricInternal{
				{
					ID:         "ns/metric0",
					Query:      "query-metric0",
					UpdateTime: defaultUpdateTime,
					Valid:      true,
					Error:      nil,
					Value:      42.0,
				},
			},
			queryMetricName:         "datadogmetric@ns:metric1",
			expectedExternalMetrics: nil,
			expectedError:           fmt.Errorf("DatadogMetric not found for metric name: datadogmetric@ns:metric1, datadogmetricid: ns/metric1"),
		},
		{
			desc: "Test ExternalMetric use wrong DatadogMetric format",
			storeContent: []model.DatadogMetricInternal{
				{
					ID:         "ns/metric0",
					Query:      "query-metric0",
					UpdateTime: defaultUpdateTime,
					Valid:      true,
					Error:      nil,
					Value:      42.0,
				},
			},
			queryMetricName:         "datadogmetric@metric1",
			expectedExternalMetrics: nil,
			expectedError:           fmt.Errorf("ExternalMetric does not follow DatadogMetric format"),
		},
		{
			desc: "Test ExternalMetric does not use DatadogMetric format",
			storeContent: []model.DatadogMetricInternal{
				{
					ID:         "ns/metric0",
					Query:      "query-metric0",
					UpdateTime: defaultUpdateTime,
					Valid:      true,
					Error:      nil,
					Value:      42.0,
				},
			},
			queryMetricName:         "nginx.net.request_per_s",
			expectedExternalMetrics: nil,
			expectedError:           fmt.Errorf("DatadogMetric not found for metric name: nginx.net.request_per_s, datadogmetricid: default/dcaautogen-32402d8dfc05cf540928a606d78ed68c0607f758"),
		},
	}

	for i, fixture := range fixtures {
		t.Run(fmt.Sprintf("#%d %s", i, fixture.desc), func(t *testing.T) {
			fixture.runGetExternalMetric(t)
		})
	}
}

func TestListAllExternalMetrics(t *testing.T) {
	defaultUpdateTime := time.Now().UTC()

	fixtures := []providerFixture{
		{
			desc:                       "Test no metrics in store",
			storeContent:               []model.DatadogMetricInternal{},
			expectedExternalMetricInfo: []provider.ExternalMetricInfo{},
		},
		{
			desc: "Test with metrics in store",
			storeContent: []model.DatadogMetricInternal{
				{
					ID:         "ns/metric0",
					Query:      "query-metric0",
					UpdateTime: defaultUpdateTime,
					Valid:      true,
					Error:      nil,
					Value:      42.0,
				},
				{
					ID:         "ns/metric1",
					Query:      "query-metric1",
					UpdateTime: defaultUpdateTime,
					Valid:      false,
					Error:      nil,
					Value:      42.0,
				},
				{
					ID:                 "autogen-foo",
					Query:              "query-metric2",
					UpdateTime:         defaultUpdateTime,
					ExternalMetricName: "metric2",
					Autogen:            true,
					Valid:              false,
					Error:              nil,
					Value:              42.0,
				},
				{
					ID:                 "autogen-bar",
					Query:              "query-metric3",
					UpdateTime:         defaultUpdateTime,
					ExternalMetricName: "metric2",
					Autogen:            true,
					Valid:              false,
					Error:              nil,
					Value:              42.0,
				},
			},
			expectedExternalMetricInfo: []provider.ExternalMetricInfo{
				{Metric: "datadogmetric@ns:metric0"},
				{Metric: "datadogmetric@ns:metric1"},
				{Metric: "metric2"},
			},
		},
	}

	for i, fixture := range fixtures {
		t.Run(fmt.Sprintf("#%d %s", i, fixture.desc), func(t *testing.T) {
			fixture.runListAllExternalMetrics(t)
		})
	}
}
